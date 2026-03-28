package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework Carbon
#include "hotkey_darwin.h"
*/
import "C"

import (
	"fmt"
	"log/slog"
)

// Package-level channel for the cgo callback to send events.
// Set by Start(), read by the exported callback.
var hotkeyEvents chan<- HotkeyEvent

//export hotkeyCallback
func hotkeyCallback(eventType C.int) {
	if hotkeyEvents == nil {
		return
	}
	switch int(eventType) {
	case 0:
		hotkeyEvents <- TriggerPressed
	case 1:
		hotkeyEvents <- TriggerReleased
	}
}

// macOS CGEvent modifier flag constants
const (
	flagFn     uint64 = 0x800000 // NX_SECONDARYFN / kCGEventFlagMaskSecondaryFn
	flagShift  uint64 = 0x20000  // kCGEventFlagMaskShift
	flagCtrl   uint64 = 0x40000  // kCGEventFlagMaskControl
	flagOption uint64 = 0x80000  // kCGEventFlagMaskAlternate
	flagCmd    uint64 = 0x100000 // kCGEventFlagMaskCommand
)

var keyToFlag = map[string]uint64{
	"fn":     flagFn,
	"shift":  flagShift,
	"ctrl":   flagCtrl,
	"option": flagOption,
	"cmd":    flagCmd,
}

type cgEventHotkeyListener struct {
	triggerKeys []string
	logger      *slog.Logger
}

func NewHotkeyListener(triggerKeys []string, logger *slog.Logger) HotkeyListener {
	return &cgEventHotkeyListener{
		triggerKeys: triggerKeys,
		logger:      logger.With("component", "hotkey"),
	}
}

func (h *cgEventHotkeyListener) Start(events chan<- HotkeyEvent) error {
	h.logger.Info("starting", "operation", "Start", "trigger_keys", h.triggerKeys)

	flags, err := keysToFlags(h.triggerKeys)
	if err != nil {
		return fmt.Errorf("hotkey.Start: %w", err)
	}

	hotkeyEvents = events

	result := C.startHotkeyListener(C.uint64_t(flags))
	if result != 0 {
		return fmt.Errorf("hotkey.Start: failed to create event tap — grant Accessibility permission in System Settings → Privacy & Security → Accessibility")
	}

	h.logger.Info("listening", "operation", "Start", "flags", fmt.Sprintf("0x%x", flags))

	// Blocks — runs the CFRunLoop on the calling (main) thread
	C.runMainLoop()

	return nil
}

func (h *cgEventHotkeyListener) Stop() error {
	h.logger.Info("stopping", "operation", "Stop")
	C.stopHotkeyListener()
	C.stopMainLoop()
	return nil
}

func keysToFlags(keys []string) (uint64, error) {
	var flags uint64
	for _, k := range keys {
		f, ok := keyToFlag[k]
		if !ok {
			return 0, fmt.Errorf("keysToFlags: key %q is not a modifier (only fn, shift, ctrl, option, cmd are supported as trigger keys)", k)
		}
		flags |= f
	}
	return flags, nil
}
