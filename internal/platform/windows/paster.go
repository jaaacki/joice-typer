//go:build windows

package windows

import (
	"fmt"
	"log/slog"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText  = 13
	gmemMoveable   = 0x0002
	gmemZeroInit   = 0x0040
	inputKeyboard  = 1
	keyeventfKeyUp = 0x0002
	keyeventfUnicode = 0x0004
	vkControl      = 0x11
	vkV            = 0x56
)

type windowsClipboardPaster struct {
	logger *slog.Logger
}

type windowsInput struct {
	Type uint32
	_    uint32
	Ki   windowsKeybdInput
}

type windowsKeybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

var (
	user32Paster                     = windows.NewLazySystemDLL("user32.dll")
	kernel32Paster                   = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard                = user32Paster.NewProc("OpenClipboard")
	procCloseClipboard               = user32Paster.NewProc("CloseClipboard")
	procEmptyClipboard               = user32Paster.NewProc("EmptyClipboard")
	procSetClipboardData             = user32Paster.NewProc("SetClipboardData")
	procSendInput                    = user32Paster.NewProc("SendInput")
	procGlobalAlloc                  = kernel32Paster.NewProc("GlobalAlloc")
	procGlobalLock                   = kernel32Paster.NewProc("GlobalLock")
	procGlobalUnlock                 = kernel32Paster.NewProc("GlobalUnlock")
	procGlobalFree                   = kernel32Paster.NewProc("GlobalFree")
	windowsPasterClipboardText       = setWindowsClipboardText
	windowsPasterSendPasteShortcut   = sendWindowsPasteShortcut
	windowsPasterTypeUnicodeFallback = typeWindowsUnicodeFallback
)

func NewPaster(logger *slog.Logger) Paster {
	if logger == nil {
		logger = slog.Default()
	}
	return &windowsClipboardPaster{
		logger: logger.With("component", "paster"),
	}
}

func (p *windowsClipboardPaster) Paste(text string) error {
	p.logger.Debug("pasting", "operation", "Paste", "text_length", len(text))

	clipboardErr := windowsPasterClipboardText(text)
	if clipboardErr == nil {
		if shortcutErr := windowsPasterSendPasteShortcut(); shortcutErr == nil {
			p.logger.Debug("pasted via clipboard shortcut", "operation", "Paste")
			return nil
		} else {
			p.logger.Warn("paste shortcut failed, falling back to unicode typing", "operation", "Paste", "error", shortcutErr)
		}
	} else {
		p.logger.Warn("clipboard paste failed, falling back to unicode typing", "operation", "Paste", "error", clipboardErr)
	}

	if err := windowsPasterTypeUnicodeFallback(text); err != nil {
		return fmt.Errorf("paster.Paste: %w", err)
	}

	p.logger.Debug("pasted via unicode typing fallback", "operation", "Paste")
	return nil
}

func setWindowsClipboardText(text string) error {
	utf16Text, err := windows.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("encode clipboard text: %w", err)
	}
	if ok, _, callErr := procOpenClipboard.Call(0); ok == 0 {
		return fmt.Errorf("open clipboard: %w", callErr)
	}
	defer procCloseClipboard.Call()

	if ok, _, callErr := procEmptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("empty clipboard: %w", callErr)
	}

	bytesLen := uintptr(len(utf16Text) * 2)
	mem, _, callErr := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, bytesLen)
	if mem == 0 {
		return fmt.Errorf("global alloc: %w", callErr)
	}

	lock, _, callErr := procGlobalLock.Call(mem)
	if lock == 0 {
		procGlobalFree.Call(mem)
		return fmt.Errorf("global lock: %w", callErr)
	}

	copy(unsafe.Slice((*uint16)(unsafe.Pointer(lock)), len(utf16Text)), utf16Text)
	procGlobalUnlock.Call(mem)

	if handle, _, callErr := procSetClipboardData.Call(cfUnicodeText, mem); handle == 0 {
		procGlobalFree.Call(mem)
		return fmt.Errorf("set clipboard data: %w", callErr)
	}
	return nil
}

func sendWindowsPasteShortcut() error {
	inputs := []windowsInput{
		{Type: inputKeyboard, Ki: windowsKeybdInput{WVk: vkControl}},
		{Type: inputKeyboard, Ki: windowsKeybdInput{WVk: vkV}},
		{Type: inputKeyboard, Ki: windowsKeybdInput{WVk: vkV, DwFlags: keyeventfKeyUp}},
		{Type: inputKeyboard, Ki: windowsKeybdInput{WVk: vkControl, DwFlags: keyeventfKeyUp}},
	}
	return sendWindowsInputs(inputs)
}

func typeWindowsUnicodeFallback(text string) error {
	codeUnits := utf16.Encode([]rune(text))
	if len(codeUnits) == 0 {
		return nil
	}
	inputs := make([]windowsInput, 0, len(codeUnits)*2)
	for _, unit := range codeUnits {
		inputs = append(inputs,
			windowsInput{Type: inputKeyboard, Ki: windowsKeybdInput{WScan: unit, DwFlags: keyeventfUnicode}},
			windowsInput{Type: inputKeyboard, Ki: windowsKeybdInput{WScan: unit, DwFlags: keyeventfUnicode | keyeventfKeyUp}},
		)
	}
	return sendWindowsInputs(inputs)
}

func sendWindowsInputs(inputs []windowsInput) error {
	if len(inputs) == 0 {
		return nil
	}
	ret, _, callErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(windowsInput{}),
	)
	if ret != uintptr(len(inputs)) {
		return fmt.Errorf("send input: sent %d/%d: %w", ret, len(inputs), callErr)
	}
	return nil
}
