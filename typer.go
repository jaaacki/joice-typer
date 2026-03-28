package main

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

// typeRune handles BMP and supplementary plane Unicode via UTF-16 surrogate pairs.
static inline int typeRune(unsigned int codepoint) {
	UniChar chars[2];
	int charCount;
	if (codepoint <= 0xFFFF) {
		chars[0] = (UniChar)codepoint;
		charCount = 1;
	} else {
		unsigned int cp = codepoint - 0x10000;
		chars[0] = (UniChar)(0xD800 + (cp >> 10));
		chars[1] = (UniChar)(0xDC00 + (cp & 0x3FF));
		charCount = 2;
	}

	CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
	if (down == NULL) return 1;
	CGEventSetFlags(down, 0); // clear modifiers so held Fn/Shift don't leak
	CGEventKeyboardSetUnicodeString(down, charCount, chars);
	CGEventPost(kCGHIDEventTap, down);

	CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
	if (up == NULL) {
		CFRelease(down);
		return 2;
	}
	CGEventSetFlags(up, 0);
	CGEventKeyboardSetUnicodeString(up, charCount, chars);
	CGEventPost(kCGHIDEventTap, up);

	CFRelease(down);
	CFRelease(up);
	return 0;
}

static inline int typeBackspace(void) {
	CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0x33, true);
	if (down == NULL) return 1;
	CGEventSetFlags(down, 0); // clear modifiers
	CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0x33, false);
	if (up == NULL) {
		CFRelease(down);
		return 2;
	}
	CGEventSetFlags(up, 0);
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(down);
	CFRelease(up);
	return 0;
}
*/
import "C"

import (
	"fmt"
	"log/slog"
	"time"
)

type cgEventTyper struct {
	logger    *slog.Logger
	charDelay time.Duration
}

func NewTyper(logger *slog.Logger) Typer {
	return &cgEventTyper{
		logger:    logger.With("component", "typer"),
		charDelay: 1 * time.Millisecond,
	}
}

func (t *cgEventTyper) Type(text string) error {
	t.logger.Debug("typing", "operation", "Type", "length", len(text))
	for _, r := range text {
		result := C.typeRune(C.uint(r))
		if result != 0 {
			return fmt.Errorf("typer.Type: CGEvent failed for char %q (error %d)", r, int(result))
		}
		if t.charDelay > 0 {
			time.Sleep(t.charDelay)
		}
	}
	return nil
}

func (t *cgEventTyper) Backspace(count int) error {
	t.logger.Debug("backspacing", "operation", "Backspace", "count", count)
	for i := 0; i < count; i++ {
		result := C.typeBackspace()
		if result != 0 {
			return fmt.Errorf("typer.Backspace: CGEvent failed (error %d)", int(result))
		}
		if t.charDelay > 0 {
			time.Sleep(t.charDelay)
		}
	}
	return nil
}

func (t *cgEventTyper) ReplaceAll(oldLen int, newText string) error {
	if oldLen > 0 {
		if err := t.Backspace(oldLen); err != nil {
			return fmt.Errorf("typer.ReplaceAll: %w", err)
		}
	}
	if len(newText) > 0 {
		if err := t.Type(newText); err != nil {
			return fmt.Errorf("typer.ReplaceAll: %w", err)
		}
	}
	return nil
}
