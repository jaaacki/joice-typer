//go:build windows

package windows

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	bridgepkg "voicetype/internal/core/bridge"

	edgepkg "github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	wmSize               = 0x0005
	wmClose              = 0x0010
	wmDestroy            = 0x0002
	pmRemove             = 0x0001
	swHide               = 0
	swShow               = 5
	wsOverlappedWindow   = 0x00CF0000
	cwUseDefault         = 0x80000000
	windowClassName      = "JoiceTyperWebSettingsWindow"
	windowTitle          = "JoiceTyper Preferences"
	webView2RuntimeError = "WebView2 host unavailable"
)

const (
	webView2RuntimeRegistryGUID       = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	webView2RuntimeInstallHelpMessage = "Install Microsoft Edge WebView2 Runtime and reopen JoiceTyper."
)

type windowsPoint struct {
	X int32
	Y int32
}

type windowsMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      windowsPoint
}

type windowsWndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type windowsHostCommand struct {
	fn   func(*windowsWebView2Host) error
	done chan error
}

type windowsWebView2Host struct {
	mu              sync.Mutex
	startOnce       sync.Once
	startReady      chan struct{}
	startErr        error
	commandQueue    chan windowsHostCommand
	done            chan struct{}
	threadID        uint32
	classRegistered bool
	className       *uint16
	hwnd            uintptr
	chromium        *edgepkg.Chromium
	visible         bool
}

var sharedWindowsWebView2Host = newWindowsWebView2Host()

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procUpdateWindow               = user32.NewProc("UpdateWindow")
	procSetFocus                   = user32.NewProc("SetFocus")
	procSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	procPeekMessageW               = user32.NewProc("PeekMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessageW           = user32.NewProc("DispatchMessageW")
	procPostQuitMessage            = user32.NewProc("PostQuitMessage")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID         = kernel32.NewProc("GetCurrentThreadId")
	windowsWebView2WndProcCallback = windows.NewCallback(windowsWebView2WndProc)
)

func newWindowsWebView2Host() *windowsWebView2Host {
	return &windowsWebView2Host{
		startReady:   make(chan struct{}),
		commandQueue: make(chan windowsHostCommand, 32),
		done:         make(chan struct{}),
	}
}

func showWindowsWebView2Host(html string) error {
	if err := sharedWindowsWebView2Host.ensureStarted(); err != nil {
		webSettingsNativeTransportWarning("showWindowsWebView2Host", err.Error())
		return err
	}
	webSettingsNativeTransportInfo("showWindowsWebView2Host", "showing windows webview2 settings window")
	if err := sharedWindowsWebView2Host.invoke(func(host *windowsWebView2Host) error {
		return host.show(html)
	}); err != nil {
		webSettingsNativeTransportWarning("showWindowsWebView2Host", err.Error())
		return err
	}
	return nil
}

func focusWindowsWebView2Host() {
	if err := sharedWindowsWebView2Host.ensureStarted(); err != nil {
		webSettingsNativeTransportWarning("focusWindowsWebView2Host", err.Error())
		return
	}
	if err := sharedWindowsWebView2Host.invoke(func(host *windowsWebView2Host) error {
		return host.focus()
	}); err != nil {
		webSettingsNativeTransportWarning("focusWindowsWebView2Host", err.Error())
	}
}

func dispatchWindowsWebView2Envelope(payloadJSON string, closeWindow bool) {
	if err := sharedWindowsWebView2Host.ensureStarted(); err != nil {
		webSettingsNativeTransportWarning("dispatchWindowsWebView2Envelope", err.Error())
		return
	}
	if err := sharedWindowsWebView2Host.invoke(func(host *windowsWebView2Host) error {
		return host.dispatch(payloadJSON, closeWindow)
	}); err != nil {
		webSettingsNativeTransportWarning("dispatchWindowsWebView2Envelope", err.Error())
	}
}

func (h *windowsWebView2Host) ensureStarted() error {
	h.startOnce.Do(func() {
		go h.run()
	})
	<-h.startReady
	return h.startErr
}

func (h *windowsWebView2Host) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		h.startErr = fmt.Errorf("%s: coinitialize: %w", webView2RuntimeError, err)
		close(h.startReady)
		close(h.done)
		return
	}
	defer windows.CoUninitialize()

	if err := h.initThread(); err != nil {
		h.startErr = err
		close(h.startReady)
		close(h.done)
		return
	}

	close(h.startReady)

	var msg windowsMsg
	for {
		for {
			select {
			case cmd := <-h.commandQueue:
				cmd.done <- cmd.fn(h)
			default:
				goto pumpMessages
			}
		}
	pumpMessages:
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)
		if ret != 0 {
			if msg.Message == wmDestroy {
				close(h.done)
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *windowsWebView2Host) initThread() error {
	threadID, _, _ := procGetCurrentThreadID.Call()
	h.threadID = uint32(threadID)

	version, err := detectInstalledWebView2Version()
	if err != nil {
		return fmt.Errorf("%s: detect runtime: %w", webView2RuntimeError, err)
	}
	if version == "" {
		return fmt.Errorf("%s: missing Microsoft Edge WebView2 Runtime. %s", webView2RuntimeError, webView2RuntimeInstallHelpMessage)
	}
	webSettingsNativeTransportInfo("initWindowsWebView2Host", "detected WebView2 runtime "+version)

	className, err := windows.UTF16PtrFromString(windowClassName)
	if err != nil {
		return fmt.Errorf("%s: class name: %w", webView2RuntimeError, err)
	}
	h.className = className

	instance, _, callErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return fmt.Errorf("%s: get module handle: %w", webView2RuntimeError, callErr)
	}

	wc := windowsWndClassExW{
		CbSize:        uint32(unsafe.Sizeof(windowsWndClassExW{})),
		HInstance:     windows.Handle(instance),
		LpszClassName: className,
		LpfnWndProc:   windowsWebView2WndProcCallback,
	}
	atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		// Class may already be registered in a restarted host. Keep going.
		webSettingsNativeTransportInfo("initWindowsWebView2Host", "window class already registered or reused")
	}
	h.classRegistered = true
	return nil
}

func detectInstalledWebView2Version() (string, error) {
	paths := []struct {
		root registry.Key
		path string
	}{
		{root: registry.LOCAL_MACHINE, path: `Software\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webView2RuntimeRegistryGUID},
		{root: registry.CURRENT_USER, path: `Software\Microsoft\EdgeUpdate\Clients\` + webView2RuntimeRegistryGUID},
		{root: registry.LOCAL_MACHINE, path: `Software\Microsoft\EdgeUpdate\Clients\` + webView2RuntimeRegistryGUID},
	}

	for _, candidate := range paths {
		key, err := registry.OpenKey(candidate.root, candidate.path, registry.QUERY_VALUE)
		if err != nil {
			if err == registry.ErrNotExist {
				continue
			}
			return "", err
		}
		version, _, err := key.GetStringValue("pv")
		key.Close()
		if err != nil {
			if err == registry.ErrNotExist {
				continue
			}
			return "", err
		}
		version = strings.TrimSpace(version)
		if version != "" && version != "0.0.0.0" {
			return version, nil
		}
	}

	return "", nil
}

func (h *windowsWebView2Host) invoke(fn func(*windowsWebView2Host) error) error {
	currentThreadID, _, _ := procGetCurrentThreadID.Call()
	if uint32(currentThreadID) == h.threadID {
		return fn(h)
	}
	done := make(chan error, 1)
	h.commandQueue <- windowsHostCommand{fn: fn, done: done}
	return <-done
}

func (h *windowsWebView2Host) show(html string) error {
	if err := h.ensureWindow(); err != nil {
		return err
	}
	h.chromium.NavigateToString(html)
	procShowWindow.Call(h.hwnd, swShow)
	procUpdateWindow.Call(h.hwnd)
	procSetForegroundWindow.Call(h.hwnd)
	procSetFocus.Call(h.hwnd)
	h.visible = true
	return nil
}

func (h *windowsWebView2Host) focus() error {
	if h.hwnd == 0 {
		return nil
	}
	procShowWindow.Call(h.hwnd, swShow)
	procSetForegroundWindow.Call(h.hwnd)
	procSetFocus.Call(h.hwnd)
	h.visible = true
	return nil
}

func (h *windowsWebView2Host) dispatch(payloadJSON string, closeWindow bool) error {
	if h.chromium == nil {
		return fmt.Errorf("%s: chromium host is not initialized", webView2RuntimeError)
	}
	h.chromium.Eval(webView2EnvelopeDispatchScript(payloadJSON))
	if closeWindow {
		procShowWindow.Call(h.hwnd, swHide)
		h.visible = false
		webSettingsWindowClosed()
	}
	return nil
}

func (h *windowsWebView2Host) ensureWindow() error {
	if h.hwnd != 0 && h.chromium != nil {
		return nil
	}
	if h.className == nil {
		return fmt.Errorf("%s: window class not initialized", webView2RuntimeError)
	}

	title, err := windows.UTF16PtrFromString(windowTitle)
	if err != nil {
		return fmt.Errorf("%s: window title: %w", webView2RuntimeError, err)
	}

	instance, _, callErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return fmt.Errorf("%s: get module handle: %w", webView2RuntimeError, callErr)
	}

	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(h.className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow,
		cwUseDefault,
		cwUseDefault,
		1180,
		860,
		0,
		0,
		instance,
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("%s: create window: %w", webView2RuntimeError, createErr)
	}

	chromium := edgepkg.NewChromium()
	chromium.SetErrorCallback(func(err error) {
		webSettingsNativeTransportWarning("windowsWebView2Host", err.Error())
	})
	chromium.MessageCallback = func(message string, _ *edgepkg.ICoreWebView2, _ *edgepkg.ICoreWebView2WebMessageReceivedEventArgs) {
		result := processWebSettingsMessage(message)
		dispatchWindowsWebView2Envelope(webSettingsResponseJSON(result.response), result.closeWindow)
	}

	if !chromium.Embed(hwnd) {
		procDestroyWindow.Call(hwnd)
		return bridgepkg.NewContractError(
			bridgepkg.ErrorCodeInternal,
			webView2RuntimeError,
			false,
			map[string]any{"operation": "Chromium.Embed"},
		)
	}
	chromium.Init(webView2DocumentCreatedScript())

	h.hwnd = hwnd
	h.chromium = chromium
	return nil
}

func windowsWebView2WndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmSize:
		if sharedWindowsWebView2Host.chromium != nil {
			sharedWindowsWebView2Host.chromium.Resize()
		}
		return 0
	case wmClose:
		procShowWindow.Call(hwnd, swHide)
		sharedWindowsWebView2Host.visible = false
		webSettingsWindowClosed()
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	default:
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return ret
	}
}

func webView2DocumentCreatedScript() string {
	return `(function() {
  if (window.__JOICETYPER_WEBVIEW2_BRIDGE_READY__) return;
  window.__JOICETYPER_WEBVIEW2_BRIDGE_READY__ = true;
  window.webkit = window.webkit || {};
  window.webkit.messageHandlers = window.webkit.messageHandlers || {};
  window.webkit.messageHandlers.joicetyper = {
    postMessage: function(payload) {
      if (!window.chrome || !window.chrome.webview) return;
      window.chrome.webview.postMessage(JSON.stringify(payload));
    }
  };
  window.__JOICETYPER_NATIVE_BRIDGE_DISPATCH__ = function(payload) {
    window.dispatchEvent(new CustomEvent("` + bridgepkg.BridgeEventName + `", { detail: payload }));
  };
  if (window.chrome && window.chrome.webview) {
    window.chrome.webview.addEventListener('message', function(event) {
      try {
        var payload = typeof event.data === 'string' ? JSON.parse(event.data) : event.data;
        window.__JOICETYPER_NATIVE_BRIDGE_DISPATCH__(payload);
      } catch (error) {
        console.error('joicetyper windows bridge dispatch failed', error);
      }
    });
  }
})();`
}

func webView2EnvelopeDispatchScript(payloadJSON string) string {
	var script strings.Builder
	script.WriteString("(function(){")
	script.WriteString("const payload = ")
	script.WriteString(payloadJSON)
	script.WriteString(";")
	script.WriteString("if (window.__JOICETYPER_NATIVE_BRIDGE_DISPATCH__) { window.__JOICETYPER_NATIVE_BRIDGE_DISPATCH__(payload); }")
	script.WriteString("})();")
	return script.String()
}
