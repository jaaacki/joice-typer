//go:build windows

package windows

import (
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"sync"
	"unsafe"

	bridgepkg "voicetype/internal/core/bridge"

	"golang.org/x/sys/windows"
)

type hotkeyCaptureListener struct {
	logger *slog.Logger

	mu       sync.Mutex
	thread   uint32
	hook     windows.Handle
	pressed  map[uint32]bool
	captured []string
}

var (
	webHotkeyCaptureMu    sync.Mutex
	webHotkeyCaptureState bridgepkg.HotkeyCaptureSnapshot

	windowsHotkeyCaptureMu       sync.Mutex
	windowsHotkeyCaptureListener *hotkeyCaptureListener
	windowsHotkeyCaptureCallback = windows.NewCallback(windowsLowLevelHotkeyCaptureProc)
)

func hotkeyCaptureSnapshot(keys []string, recording bool) bridgepkg.HotkeyCaptureSnapshot {
	display := "Press keys..."
	if len(keys) > 0 {
		display = FormatHotkeyDisplay(keys)
	} else if !recording {
		display = ""
	}
	return bridgepkg.HotkeyCaptureSnapshot{
		TriggerKey: append([]string(nil), keys...),
		Display:    display,
		Recording:  recording,
		CanConfirm: len(keys) > 0,
	}
}

func setWebHotkeyCaptureState(snapshot bridgepkg.HotkeyCaptureSnapshot) {
	webHotkeyCaptureMu.Lock()
	webHotkeyCaptureState = snapshot
	webHotkeyCaptureMu.Unlock()
}

func currentWebHotkeyCaptureState() bridgepkg.HotkeyCaptureSnapshot {
	webHotkeyCaptureMu.Lock()
	defer webHotkeyCaptureMu.Unlock()
	return webHotkeyCaptureState
}

func startWebSettingsHotkeyCapture() (bridgepkg.HotkeyCaptureSnapshot, error) {
	current := currentWebHotkeyCaptureState()
	if current.Recording {
		return current, nil
	}

	listener := &hotkeyCaptureListener{
		logger:  currentSettingsLogger().With("component", "hotkey_capture"),
		pressed: make(map[uint32]bool),
	}
	ready := make(chan error, 1)
	go listener.run(ready)
	if err := <-ready; err != nil {
		return bridgepkg.HotkeyCaptureSnapshot{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeHotkeyCaptureStartFailed,
			"Failed to start hotkey capture",
			false,
			nil,
			err,
		)
	}

	snapshot := hotkeyCaptureSnapshot(nil, true)
	setWebHotkeyCaptureState(snapshot)
	publishHotkeyCaptureChanged(snapshot)
	return snapshot, nil
}

func cancelWebSettingsHotkeyCapture() error {
	if err := stopWindowsHotkeyCaptureListener(); err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeHotkeyCaptureCancelFailed,
			"Failed to cancel hotkey capture",
			false,
			nil,
			err,
		)
	}
	snapshot := bridgepkg.HotkeyCaptureSnapshot{}
	setWebHotkeyCaptureState(snapshot)
	publishHotkeyCaptureChanged(snapshot)
	return nil
}

func confirmWebSettingsHotkeyCapture() (bridgepkg.HotkeyCaptureSnapshot, error) {
	snapshot := currentWebHotkeyCaptureState()
	if len(snapshot.TriggerKey) == 0 {
		return bridgepkg.HotkeyCaptureSnapshot{}, bridgepkg.NewContractError(
			bridgepkg.ErrorCodeHotkeyCaptureConfirmFailed,
			"No hotkey was captured",
			false,
			nil,
		)
	}
	if err := stopWindowsHotkeyCaptureListener(); err != nil {
		return bridgepkg.HotkeyCaptureSnapshot{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeHotkeyCaptureConfirmFailed,
			"Failed to finish hotkey capture",
			false,
			nil,
			err,
		)
	}
	final := hotkeyCaptureSnapshot(snapshot.TriggerKey, false)
	setWebHotkeyCaptureState(final)
	publishHotkeyCaptureChanged(final)
	return final, nil
}

func resetWebSettingsHotkeyCapture() {
	if err := stopWindowsHotkeyCaptureListener(); err != nil {
		currentSettingsLogger().Warn("failed to stop windows hotkey capture", "operation", "resetWebSettingsHotkeyCapture", "error", err)
	}
	setWebHotkeyCaptureState(bridgepkg.HotkeyCaptureSnapshot{})
}

func (h *hotkeyCaptureListener) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	threadID, _, _ := procGetCurrentThreadID.Call()
	h.mu.Lock()
	h.thread = uint32(threadID)
	h.mu.Unlock()

	windowsHotkeyCaptureMu.Lock()
	windowsHotkeyCaptureListener = h
	windowsHotkeyCaptureMu.Unlock()

	hook, _, callErr := procSetWindowsHookExW.Call(
		whKeyboardLL,
		windowsHotkeyCaptureCallback,
		0,
		0,
	)
	if hook == 0 {
		windowsHotkeyCaptureMu.Lock()
		windowsHotkeyCaptureListener = nil
		windowsHotkeyCaptureMu.Unlock()
		ready <- fmt.Errorf("set windows hook: %w", callErr)
		return
	}
	h.mu.Lock()
	h.hook = windows.Handle(hook)
	h.mu.Unlock()
	ready <- nil

	var msg windowsMsg
	for {
		ret, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			currentSettingsLogger().Warn("windows hotkey capture message loop failed", "operation", "hotkeyCapture.run", "error", callErr)
			goto shutdown
		case 0:
			goto shutdown
		default:
			if msg.Message == wmQuit {
				goto shutdown
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}

shutdown:
	procUnhookWindowsHookEx.Call(hook)
	windowsHotkeyCaptureMu.Lock()
	if windowsHotkeyCaptureListener == h {
		windowsHotkeyCaptureListener = nil
	}
	windowsHotkeyCaptureMu.Unlock()
	h.mu.Lock()
	h.thread = 0
	h.hook = 0
	clear(h.pressed)
	h.mu.Unlock()
}

func stopWindowsHotkeyCaptureListener() error {
	windowsHotkeyCaptureMu.Lock()
	listener := windowsHotkeyCaptureListener
	windowsHotkeyCaptureMu.Unlock()
	if listener == nil {
		return nil
	}
	listener.mu.Lock()
	thread := listener.thread
	listener.mu.Unlock()
	if thread == 0 {
		return nil
	}
	ret, _, callErr := procPostThreadMessageW.Call(uintptr(thread), wmQuit, 0, 0)
	if ret == 0 {
		return fmt.Errorf("post quit: %w", callErr)
	}
	return nil
}

func windowsLowLevelHotkeyCaptureProc(code int, wParam, lParam uintptr) uintptr {
	if code == hcAction {
		windowsHotkeyCaptureMu.Lock()
		listener := windowsHotkeyCaptureListener
		windowsHotkeyCaptureMu.Unlock()
		if listener != nil {
			kb := (*windowsKBDLLHookStruct)(unsafe.Pointer(lParam))
			listener.handleKeyMessage(uint32(wParam), kb.VkCode)
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return ret
}

func (h *hotkeyCaptureListener) handleKeyMessage(message uint32, vkCode uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch message {
	case wmKeyDown, wmSysKeyDown:
		h.pressed[vkCode] = true
	case wmKeyUp, wmSysKeyUp:
		delete(h.pressed, vkCode)
	default:
		return
	}

	keys := hotkeyKeysForPressed(h.pressed)
	if len(keys) > 0 {
		h.captured = append(h.captured[:0], keys...)
	}

	snapshot := hotkeyCaptureSnapshot(h.captured, true)
	if hotkeyCaptureSnapshotEqual(snapshot, currentWebHotkeyCaptureState()) {
		return
	}
	setWebHotkeyCaptureState(snapshot)
	publishHotkeyCaptureChanged(snapshot)
}

func hotkeyCaptureSnapshotEqual(a, b bridgepkg.HotkeyCaptureSnapshot) bool {
	return a.Display == b.Display &&
		a.Recording == b.Recording &&
		a.CanConfirm == b.CanConfirm &&
		slices.Equal(a.TriggerKey, b.TriggerKey)
}

func hotkeyKeysForPressed(pressed map[uint32]bool) []string {
	keys := make([]string, 0, 5)
	if windowsModifierPressed(pressed, vkControl, vkLControl, vkRControl) {
		keys = append(keys, "ctrl")
	}
	if windowsModifierPressed(pressed, vkShift, vkLShift, vkRShift) {
		keys = append(keys, "shift")
	}
	if windowsModifierPressed(pressed, vkMenu, vkLMenu, vkRMenu) {
		keys = append(keys, "option")
	}
	if windowsModifierPressed(pressed, vkLWin, vkRWin) {
		keys = append(keys, "cmd")
	}
	for vk := range pressed {
		name := keyNameForWindowsVK(vk)
		if name == "" || name == "ctrl" || name == "shift" || name == "option" || name == "cmd" {
			continue
		}
		keys = append(keys, name)
		break
	}
	return keys
}

func keyNameForWindowsVK(vk uint32) string {
	switch vk {
	case vkControl, vkLControl, vkRControl:
		return "ctrl"
	case vkShift, vkLShift, vkRShift:
		return "shift"
	case vkMenu, vkLMenu, vkRMenu:
		return "option"
	case vkLWin, vkRWin:
		return "cmd"
	case 0x20:
		return "space"
	case 0x09:
		return "tab"
	case 0x0D:
		return "return"
	case 0x1B:
		return "escape"
	case 0x2E:
		return "delete"
	}
	if vk >= 0x70 && vk <= 0x7B {
		return fmt.Sprintf("f%d", int(vk-0x70)+1)
	}
	if vk >= 'A' && vk <= 'Z' {
		return string(rune(vk + 32))
	}
	if vk >= '0' && vk <= '9' {
		return string(rune(vk))
	}
	return ""
}
