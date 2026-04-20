//go:build windows

package windows

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmAppTrayCallback = 0x8002
	wmCommand         = 0x0111
	wmPowerBroadcast  = 0x0218
	wmLButtonUp       = 0x0202
	wmRButtonUp       = 0x0205

	pbtAPMSuspend         = 0x0004
	pbtAPMResumeSuspend   = 0x0007
	pbtAPMResumeAutomatic = 0x0012

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	idiApplication = 32512
	niifInfo       = 0x00000001

	tpmLeftAlign   = 0x0000
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	trayMenuPreferences = 1001
	trayMenuQuit        = 1002

	windowsTrayClassName = "JoiceTyperTrayWindow"
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

type trayHost struct {
	mu           sync.Mutex
	startOnce    sync.Once
	startReady   chan struct{}
	startErr     error
	commandQueue chan func(*trayHost)
	hwnd         uintptr
	threadID     uint32
	tooltip      string
	icon         windows.Handle
}

var (
	shell32                    = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procLoadIconW              = user32.NewProc("LoadIconW")
	windowsTrayWndProcCallback = windows.NewCallback(windowsTrayWndProc)
	sharedWindowsTrayHost      = newWindowsTrayHost()
	statusBarHotkeyMu          sync.Mutex
	statusBarHotkeyText        string
)

func newWindowsTrayHost() *trayHost {
	return &trayHost{
		startReady:   make(chan struct{}),
		commandQueue: make(chan func(*trayHost), 32),
		tooltip:      "JoiceTyper",
	}
}

func InitStatusBar() {
	if err := sharedWindowsTrayHost.ensureStarted(); err != nil {
		currentSettingsLogger().Warn("failed to initialize windows tray", "operation", "InitStatusBar", "error", err)
	}
}

func UpdateStatusBar(state AppState) {
	storeCurrentAppState(state)
	publishRuntimeStateChanged(state)
	if err := sharedWindowsTrayHost.ensureStarted(); err != nil {
		currentSettingsLogger().Warn("failed to update windows tray state", "operation", "UpdateStatusBar", "error", err)
		return
	}
	sharedWindowsTrayHost.invoke(func(host *trayHost) {
		host.tooltip = windowsTrayTooltip(state, currentStatusBarHotkeyText())
		host.applyTooltip()
	})
}

func SetStatusBarHotkeyText(text string) {
	storeStatusBarHotkeyText(text)
	if err := sharedWindowsTrayHost.ensureStarted(); err != nil {
		currentSettingsLogger().Warn("failed to update windows tray hotkey text", "operation", "SetStatusBarHotkeyText", "error", err)
		return
	}
	sharedWindowsTrayHost.invoke(func(host *trayHost) {
		host.tooltip = windowsTrayTooltip(currentAppState(), text)
		host.applyTooltip()
	})
}

func showWindowsTrayNotification(title, body string) error {
	if err := sharedWindowsTrayHost.ensureStarted(); err != nil {
		return err
	}
	return sharedWindowsTrayHost.showNotification(title, body)
}

func (h *trayHost) ensureStarted() error {
	h.startOnce.Do(func() {
		go h.run()
	})
	<-h.startReady
	return h.startErr
}

func (h *trayHost) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	threadID, _, _ := procGetCurrentThreadID.Call()
	h.threadID = uint32(threadID)

	if err := h.initWindow(); err != nil {
		h.startErr = err
		close(h.startReady)
		return
	}
	close(h.startReady)

	var msg windowsMsg
	for {
		for {
			select {
			case fn := <-h.commandQueue:
				fn(h)
			default:
				goto pumpMessages
			}
		}
	pumpMessages:
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)
		if ret != 0 {
			if msg.Message == wmDestroy {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *trayHost) initWindow() error {
	className, err := windows.UTF16PtrFromString(windowsTrayClassName)
	if err != nil {
		return fmt.Errorf("tray class name: %w", err)
	}
	instance, _, callErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return fmt.Errorf("tray get module handle: %w", callErr)
	}

	wc := windowsWndClassExW{
		CbSize:        uint32(unsafe.Sizeof(windowsWndClassExW{})),
		HInstance:     windows.Handle(instance),
		LpszClassName: className,
		LpfnWndProc:   windowsTrayWndProcCallback,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		0,
		0,
		0,
		0,
		0,
		0,
		instance,
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("tray create window: %w", createErr)
	}
	h.hwnd = hwnd

	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	h.icon = windows.Handle(icon)

	data := h.notifyIconData()
	ret, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	if ret == 0 {
		return fmt.Errorf("tray shell notify add: %w", callErr)
	}
	return nil
}

func (h *trayHost) notifyIconData() notifyIconData {
	data := notifyIconData{
		CbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:             windows.Handle(h.hwnd),
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmAppTrayCallback,
		HIcon:            h.icon,
	}
	copy(data.SzTip[:], windows.StringToUTF16(h.tooltip))
	return data
}

func (h *trayHost) applyTooltip() {
	if h.hwnd == 0 {
		return
	}
	data := h.notifyIconData()
	ret, _, callErr := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&data)))
	if ret == 0 {
		currentSettingsLogger().Warn("failed to update windows tray tooltip", "operation", "tray.applyTooltip", "error", callErr)
	}
}

func (h *trayHost) showNotification(title, body string) error {
	if err := h.ensureStarted(); err != nil {
		return err
	}
	var invokeErr error
	h.invoke(func(host *trayHost) {
		if host.hwnd == 0 {
			invokeErr = fmt.Errorf("windows tray host is not initialized")
			return
		}
		data := host.notifyIconData()
		data.UFlags = nifInfo
		data.DwInfoFlags = niifInfo
		copy(data.SzInfo[:], windows.StringToUTF16(body))
		copy(data.SzInfoTitle[:], windows.StringToUTF16(title))
		ret, _, callErr := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&data)))
		if ret == 0 {
			invokeErr = fmt.Errorf("tray shell notify info: %w", callErr)
		}
	})
	return invokeErr
}

func (h *trayHost) invoke(fn func(*trayHost)) {
	if h.threadID == 0 {
		return
	}
	currentThreadID, _, _ := procGetCurrentThreadID.Call()
	if uint32(currentThreadID) == h.threadID {
		fn(h)
		return
	}
	done := make(chan struct{}, 1)
	h.commandQueue <- func(host *trayHost) {
		fn(host)
		done <- struct{}{}
	}
	<-done
}

func windowsTrayWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmAppTrayCallback:
		switch uint32(lParam) {
		case wmLButtonUp:
			go OpenPreferences()
		case wmRButtonUp:
			showWindowsTrayMenu(hwnd)
		}
		return 0
	case wmPowerBroadcast:
		switch uint32(wParam) {
		case pbtAPMSuspend:
			dispatchPowerEvent(PowerEventSleep)
		case pbtAPMResumeSuspend, pbtAPMResumeAutomatic:
			dispatchPowerEvent(PowerEventWake)
		}
		return 1
	case wmDestroy:
		if sharedWindowsTrayHost.hwnd != 0 {
			data := sharedWindowsTrayHost.notifyIconData()
			procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
			sharedWindowsTrayHost.hwnd = 0
		}
		procPostQuitMessage.Call(0)
		return 0
	default:
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return ret
	}
}

func showWindowsTrayMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	preferencesLabel, _ := windows.UTF16PtrFromString("Preferences")
	quitLabel, _ := windows.UTF16PtrFromString("Quit")
	procAppendMenuW.Call(menu, 0x0000, trayMenuPreferences, uintptr(unsafe.Pointer(preferencesLabel)))
	procAppendMenuW.Call(menu, 0x0000, trayMenuQuit, uintptr(unsafe.Pointer(quitLabel)))

	var point windowsPoint
	if ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&point))); ret == 0 {
		return
	}
	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmLeftAlign|tpmRightButton|tpmReturnCmd,
		uintptr(point.X),
		uintptr(point.Y),
		0,
		hwnd,
		0,
	)
	switch cmd {
	case trayMenuPreferences:
		go OpenPreferences()
	case trayMenuQuit:
		go RequestQuit()
	}
}

func storeStatusBarHotkeyText(text string) {
	statusBarHotkeyMu.Lock()
	statusBarHotkeyText = text
	statusBarHotkeyMu.Unlock()
}

func currentStatusBarHotkeyText() string {
	statusBarHotkeyMu.Lock()
	defer statusBarHotkeyMu.Unlock()
	return statusBarHotkeyText
}

func windowsTrayTooltip(state AppState, hotkey string) string {
	stateLabel := "Loading"
	switch state {
	case StateReady:
		stateLabel = "Ready"
	case StateRecording:
		stateLabel = "Recording"
	case StateTranscribing:
		stateLabel = "Transcribing"
	case StateNoPermission:
		stateLabel = "Needs permissions"
	case StateDependencyStuck:
		stateLabel = "Dependency issue"
	}
	if hotkey == "" {
		return "JoiceTyper - " + stateLabel
	}
	return "JoiceTyper - " + stateLabel + " (" + hotkey + ")"
}
