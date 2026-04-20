//go:build windows

package windows

import (
	"fmt"
	"log/slog"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfText           = 1
	cfOemText        = 7
	cfDib            = 8
	cfUnicodeText    = 13
	cfHDrop          = 15
	cfLocale         = 16
	cfDibV5          = 17
	cfDspText        = 0x0081
	cfDspOemText     = 0x0082
	cfDspUnicodeText = 0x0083
	gmemMoveable     = 0x0002
	gmemZeroInit     = 0x0040
	inputKeyboard    = 1
	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004
	vkV              = 0x56
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
	_           [8]byte // pad union to match MOUSEINPUT size: INPUT = 40 bytes on 64-bit
}

var (
	user32Paster                          = windows.NewLazySystemDLL("user32.dll")
	kernel32Paster                        = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard                     = user32Paster.NewProc("OpenClipboard")
	procCloseClipboard                    = user32Paster.NewProc("CloseClipboard")
	procEnumClipboardFormats              = user32Paster.NewProc("EnumClipboardFormats")
	procEmptyClipboard                    = user32Paster.NewProc("EmptyClipboard")
	procGetClipboardData                  = user32Paster.NewProc("GetClipboardData")
	procSetClipboardData                  = user32Paster.NewProc("SetClipboardData")
	procSendInput                         = user32Paster.NewProc("SendInput")
	procGlobalAlloc                       = kernel32Paster.NewProc("GlobalAlloc")
	procGlobalLock                        = kernel32Paster.NewProc("GlobalLock")
	procGlobalSize                        = kernel32Paster.NewProc("GlobalSize")
	procGlobalUnlock                      = kernel32Paster.NewProc("GlobalUnlock")
	procGlobalFree                        = kernel32Paster.NewProc("GlobalFree")
	windowsPasterClipboardText            = setWindowsClipboardText
	windowsPasterSendPasteShortcut        = sendWindowsPasteShortcut
	windowsPasterTypeUnicodeFallback      = typeWindowsUnicodeFallback
	windowsPasterReadClipboardSnapshot    = readWindowsClipboardSnapshot
	windowsPasterRestoreClipboardSnapshot = restoreWindowsClipboardSnapshot
)

type windowsClipboardEntry struct {
	format uint32
	data   []byte
}

type windowsClipboardSnapshot struct {
	entries []windowsClipboardEntry
}

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

	previousClipboard, snapshotErr := windowsPasterReadClipboardSnapshot()
	if snapshotErr != nil {
		p.logger.Warn("clipboard snapshot failed before paste", "operation", "Paste", "error", snapshotErr)
	}

	clipboardErr := windowsPasterClipboardText(text)
	if clipboardErr == nil {
		restoreClipboard := func() {
			if restoreErr := windowsPasterRestoreClipboardSnapshot(previousClipboard); restoreErr != nil {
				p.logger.Warn("restore clipboard failed after paste", "operation", "Paste", "error", restoreErr)
			}
		}
		if shortcutErr := windowsPasterSendPasteShortcut(); shortcutErr == nil {
			// Restore asynchronously: SendInput queues events but returns before
			// the target app processes Ctrl+V. Restore too early and the old
			// clipboard content replaces the transcription before paste fires.
			go func() {
				time.Sleep(50 * time.Millisecond)
				restoreClipboard()
			}()
			p.logger.Debug("pasted via clipboard shortcut", "operation", "Paste")
			return nil
		} else {
			restoreClipboard()
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

func readWindowsClipboardSnapshot() (windowsClipboardSnapshot, error) {
	if ok, _, callErr := procOpenClipboard.Call(0); ok == 0 {
		return windowsClipboardSnapshot{}, fmt.Errorf("open clipboard: %w", callErr)
	}
	defer procCloseClipboard.Call()

	snapshot := windowsClipboardSnapshot{}
	for format, _, _ := procEnumClipboardFormats.Call(0); format != 0; format, _, _ = procEnumClipboardFormats.Call(format) {
		if !isWindowsCloneableClipboardFormat(uint32(format)) {
			continue
		}
		handle, _, callErr := procGetClipboardData.Call(format)
		if handle == 0 {
			if callErr != nil && callErr.Error() != "The operation completed successfully." {
				return windowsClipboardSnapshot{}, fmt.Errorf("get clipboard data for format %d: %w", format, callErr)
			}
			continue
		}

		size, _, callErr := procGlobalSize.Call(handle)
		if size == 0 {
			continue
		}

		lock, _, callErr := procGlobalLock.Call(handle)
		if lock == 0 {
			return windowsClipboardSnapshot{}, fmt.Errorf("global lock for format %d: %w", format, callErr)
		}

		data := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(lock)), size)...)
		procGlobalUnlock.Call(handle)
		snapshot.entries = append(snapshot.entries, windowsClipboardEntry{
			format: uint32(format),
			data:   data,
		})
	}

	return snapshot, nil
}

func isWindowsCloneableClipboardFormat(format uint32) bool {
	switch format {
	case cfText, cfOemText, cfDib, cfUnicodeText, cfHDrop, cfLocale, cfDibV5, cfDspText, cfDspOemText, cfDspUnicodeText:
		return true
	default:
		return false
	}
}

func restoreWindowsClipboardSnapshot(snapshot windowsClipboardSnapshot) error {
	if ok, _, callErr := procOpenClipboard.Call(0); ok == 0 {
		return fmt.Errorf("open clipboard: %w", callErr)
	}
	defer procCloseClipboard.Call()

	if ok, _, callErr := procEmptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("empty clipboard: %w", callErr)
	}

	for _, entry := range snapshot.entries {
		mem, _, callErr := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, uintptr(len(entry.data)))
		if mem == 0 {
			return fmt.Errorf("global alloc for format %d: %w", entry.format, callErr)
		}

		lock, _, callErr := procGlobalLock.Call(mem)
		if lock == 0 {
			procGlobalFree.Call(mem)
			return fmt.Errorf("global lock for format %d: %w", entry.format, callErr)
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(lock)), len(entry.data)), entry.data)
		procGlobalUnlock.Call(mem)

		if handle, _, callErr := procSetClipboardData.Call(uintptr(entry.format), mem); handle == 0 {
			procGlobalFree.Call(mem)
			return fmt.Errorf("set clipboard data for format %d: %w", entry.format, callErr)
		}
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
