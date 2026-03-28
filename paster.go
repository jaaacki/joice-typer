package main

/*
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics
#include "paster_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"log/slog"
	"time"
	"unsafe"
)

type clipboardPaster struct {
	logger *slog.Logger
}

func NewPaster(logger *slog.Logger) Paster {
	return &clipboardPaster{
		logger: logger.With("component", "paster"),
	}
}

func (p *clipboardPaster) Paste(text string) error {
	p.logger.Debug("pasting", "operation", "Paste", "text_length", len(text))

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	result := C.setClipboard(cText)
	if result != 0 {
		return fmt.Errorf("paster.Paste: failed to set clipboard")
	}

	// Brief pause to let pasteboard settle before simulating keypress
	time.Sleep(50 * time.Millisecond)

	C.simulateCmdV()

	p.logger.Debug("pasted", "operation", "Paste")
	return nil
}
