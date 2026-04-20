//go:build windows

package windows

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whKeyboardLL = 13
	hcAction     = 0
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	wmQuit       = 0x0012

	vkShift    = 0x10
	vkControl  = 0x11
	vkMenu     = 0x12
	vkLWin     = 0x5B
	vkRWin     = 0x5C
	vkLShift   = 0xA0
	vkRShift   = 0xA1
	vkLControl = 0xA2
	vkRControl = 0xA3
	vkLMenu    = 0xA4
	vkRMenu    = 0xA5
)

type windowsKBDLLHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type hotkeyListener struct {
	logger *slog.Logger

	requiredModifiers map[string]bool
	requiredKey       uint32
	pressed           map[uint32]bool

	mu      sync.Mutex
	events  chan<- HotkeyEvent
	thread  uint32
	hook    windows.Handle
	active  bool
	running bool
}

var (
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")

	windowsHotkeyCallback = windows.NewCallback(windowsLowLevelKeyboardProc)

	windowsHotkeyMu       sync.Mutex
	windowsHotkeyListener *hotkeyListener
)

func NewHotkeyListener(triggerKeys []string, logger *slog.Logger) HotkeyListener {
	if logger == nil {
		logger = slog.Default()
	}
	requiredModifiers := make(map[string]bool)
	requiredKey := uint32(0)
	for _, key := range triggerKeys {
		switch key {
		case "ctrl", "shift", "option", "cmd":
			requiredModifiers[key] = true
		default:
			requiredKey = windowsVKForKey(key)
		}
	}
	return &hotkeyListener{
		logger:            logger.With("component", "hotkey"),
		requiredModifiers: requiredModifiers,
		requiredKey:       requiredKey,
		pressed:           make(map[uint32]bool),
	}
}

func (h *hotkeyListener) WaitForPermissions(ctx context.Context, onUpdate func(accessibility, inputMonitoring bool)) error {
	if onUpdate != nil {
		onUpdate(true, true)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (h *hotkeyListener) RunMainLoopOnly() {
	_ = h.runLoop(false, nil)
}

func (h *hotkeyListener) Start(events chan<- HotkeyEvent) error {
	return h.runLoop(true, events)
}

func (h *hotkeyListener) Stop() error {
	h.mu.Lock()
	thread := h.thread
	h.mu.Unlock()
	if thread == 0 {
		return nil
	}
	ret, _, callErr := procPostThreadMessageW.Call(uintptr(thread), wmQuit, 0, 0)
	if ret == 0 {
		return fmt.Errorf("hotkey.Stop: post quit: %w", callErr)
	}
	return nil
}

func (h *hotkeyListener) runLoop(withHook bool, events chan<- HotkeyEvent) error {
	if withHook {
		if err := h.validateSupportedHotkey(); err != nil {
			return err
		}
	}

	threadID, _, _ := procGetCurrentThreadID.Call()
	h.mu.Lock()
	h.thread = uint32(threadID)
	h.events = events
	h.running = true
	h.active = false
	clear(h.pressed)
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.thread = 0
		h.events = nil
		h.running = false
		h.active = false
		clear(h.pressed)
		h.mu.Unlock()
		if current := ActiveHotkey(); current != nil {
			if listener, ok := current.(*hotkeyListener); ok && listener == h {
				SetActiveHotkey(nil)
			}
		}
	}()

	if withHook {
		windowsHotkeyMu.Lock()
		windowsHotkeyListener = h
		windowsHotkeyMu.Unlock()
		hook, _, callErr := procSetWindowsHookExW.Call(
			whKeyboardLL,
			windowsHotkeyCallback,
			0,
			0,
		)
		if hook == 0 {
			windowsHotkeyMu.Lock()
			windowsHotkeyListener = nil
			windowsHotkeyMu.Unlock()
			return fmt.Errorf("hotkey.Start: set windows hook: %w", callErr)
		}
		h.mu.Lock()
		h.hook = windows.Handle(hook)
		h.mu.Unlock()
		SetActiveHotkey(h)
		h.logger.Info("windows hotkey hook installed", "operation", "Start")
		defer func() {
			procUnhookWindowsHookEx.Call(uintptr(hook))
			windowsHotkeyMu.Lock()
			windowsHotkeyListener = nil
			windowsHotkeyMu.Unlock()
			h.mu.Lock()
			h.hook = 0
			h.mu.Unlock()
		}()
	}

	var msg windowsMsg
	for {
		ret, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			return fmt.Errorf("hotkey message loop: %w", callErr)
		case 0:
			return nil
		default:
			if msg.Message == wmQuit {
				return nil
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func (h *hotkeyListener) validateSupportedHotkey() error {
	if len(h.requiredModifiers) == 0 && h.requiredKey == 0 {
		return fmt.Errorf("hotkey.Start: at least one key required")
	}
	for modifier := range h.requiredModifiers {
		if modifier != "ctrl" && modifier != "shift" && modifier != "option" && modifier != "cmd" {
			return fmt.Errorf("hotkey.Start: modifier %q is not supported on Windows", modifier)
		}
	}
	if h.requiredKey == 0 && len(h.requiredModifiers) == 0 {
		return fmt.Errorf("hotkey.Start: invalid trigger key")
	}
	return nil
}

func windowsLowLevelKeyboardProc(code int, wParam, lParam uintptr) uintptr {
	if code == hcAction {
		windowsHotkeyMu.Lock()
		listener := windowsHotkeyListener
		windowsHotkeyMu.Unlock()
		if listener != nil {
			kb := (*windowsKBDLLHookStruct)(unsafe.Pointer(lParam))
			listener.handleKeyMessage(uint32(wParam), kb.VkCode)
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return ret
}

func (h *hotkeyListener) handleKeyMessage(message uint32, vkCode uint32) {
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

	wasActive := h.active
	nowActive := h.triggerSatisfied()
	h.active = nowActive

	switch {
	case !wasActive && nowActive:
		h.emitLocked(TriggerPressed)
	case wasActive && !nowActive:
		h.emitLocked(TriggerReleased)
	}
}

func (h *hotkeyListener) triggerSatisfied() bool {
	if h.requiredModifiers["ctrl"] && !windowsModifierPressed(h.pressed, vkControl, vkLControl, vkRControl) {
		return false
	}
	if h.requiredModifiers["shift"] && !windowsModifierPressed(h.pressed, vkShift, vkLShift, vkRShift) {
		return false
	}
	if h.requiredModifiers["option"] && !windowsModifierPressed(h.pressed, vkMenu, vkLMenu, vkRMenu) {
		return false
	}
	if h.requiredModifiers["cmd"] && !windowsModifierPressed(h.pressed, vkLWin, vkRWin) {
		return false
	}
	if h.requiredKey != 0 && !h.pressed[h.requiredKey] {
		return false
	}
	return len(h.requiredModifiers) > 0 || h.requiredKey != 0
}

func windowsModifierPressed(pressed map[uint32]bool, variants ...uint32) bool {
	for _, vk := range variants {
		if pressed[vk] {
			return true
		}
	}
	return false
}

func (h *hotkeyListener) emitLocked(event HotkeyEvent) {
	if h.events == nil {
		return
	}
	if event == TriggerReleased {
		select {
		case h.events <- event:
		case <-time.After(100 * time.Millisecond):
			h.logger.Error("release event send timed out, channel blocked", "operation", "handleKeyMessage")
		}
		return
	}
	select {
	case h.events <- event:
	default:
		h.logger.Warn("press event dropped, channel full", "operation", "handleKeyMessage", "event", hotkeyEventString(event))
	}
}

func windowsVKForKey(name string) uint32 {
	switch name {
	case "":
		return 0
	case "space":
		return 0x20
	case "tab":
		return 0x09
	case "return":
		return 0x0D
	case "escape":
		return 0x1B
	case "delete":
		return 0x2E
	case "f1":
		return 0x70
	case "f2":
		return 0x71
	case "f3":
		return 0x72
	case "f4":
		return 0x73
	case "f5":
		return 0x74
	case "f6":
		return 0x75
	case "f7":
		return 0x76
	case "f8":
		return 0x77
	case "f9":
		return 0x78
	case "f10":
		return 0x79
	case "f11":
		return 0x7A
	case "f12":
		return 0x7B
	}
	if len(name) == 1 {
		ch := strings.ToUpper(name)[0]
		if ch >= 'A' && ch <= 'Z' {
			return uint32(ch)
		}
		if ch >= '0' && ch <= '9' {
			return uint32(ch)
		}
	}
	return 0
}

func FormatHotkeyDisplay(keys []string) string {
	display := make([]string, 0, len(keys))
	for _, key := range keys {
		switch key {
		case "ctrl":
			display = append(display, "Ctrl")
		case "shift":
			display = append(display, "Shift")
		case "option":
			display = append(display, "Alt")
		case "cmd":
			display = append(display, "Win")
		case "space":
			display = append(display, "Space")
		case "tab":
			display = append(display, "Tab")
		case "return":
			display = append(display, "Return")
		case "escape":
			display = append(display, "Escape")
		case "delete":
			display = append(display, "Delete")
		default:
			display = append(display, strings.ToUpper(key))
		}
	}
	return strings.Join(display, " + ")
}

func hotkeyEventString(e HotkeyEvent) string {
	switch e {
	case TriggerPressed:
		return "TriggerPressed"
	case TriggerReleased:
		return "TriggerReleased"
	default:
		return fmt.Sprintf("HotkeyEvent(%d)", int(e))
	}
}
