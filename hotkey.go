package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework Carbon -framework Cocoa -framework IOKit
#include <unistd.h>
#include "hotkey_darwin.h"
*/
import "C"

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	osExec "os/exec"
	"path/filepath"
	"sync"
)

func (e HotkeyEvent) String() string {
	switch e {
	case TriggerPressed:
		return "TriggerPressed"
	case TriggerReleased:
		return "TriggerReleased"
	default:
		return fmt.Sprintf("HotkeyEvent(%d)", int(e))
	}
}

// Package-level channel for the cgo callback to send events.
// Set by Start(), read by the exported callback.
var (
	hotkeyMu     sync.Mutex
	hotkeyEvents chan<- HotkeyEvent
)

var hotkeyLogger *slog.Logger

//export hotkeyFlagsChanged
func hotkeyFlagsChanged(flags C.uint64_t) {
	if hotkeyLogger != nil {
		f := uint64(flags)
		hotkeyLogger.Debug("flags changed",
			"operation", "eventTap",
			"raw_flags", fmt.Sprintf("0x%x", f),
			"fn", f&flagFn != 0,
			"shift", f&flagShift != 0,
			"ctrl", f&flagCtrl != 0,
			"option", f&flagOption != 0,
			"cmd", f&flagCmd != 0,
		)
	}
}

//export hotkeyCallback
func hotkeyCallback(eventType C.int) {
	hotkeyMu.Lock()
	ch := hotkeyEvents
	hotkeyMu.Unlock()
	if ch == nil {
		return
	}
	var event HotkeyEvent
	switch int(eventType) {
	case 0:
		event = TriggerPressed
	case 1:
		event = TriggerReleased
	default:
		return
	}
	if hotkeyLogger != nil {
		hotkeyLogger.Info("hotkey event", "operation", "hotkeyCallback", "event", event.String())
	}
	if event == TriggerReleased {
		// Release is critical control traffic — must never be lost.
		// Block if necessary; the consumer processes events quickly.
		ch <- event
	} else {
		select {
		case ch <- event:
		default:
			// Channel full — drop press (idempotent, will retry on next press)
			if hotkeyLogger != nil {
				hotkeyLogger.Warn("press event dropped, channel full",
					"operation", "hotkeyCallback", "event", event.String())
			}
		}
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

// WaitForPermissions polls Accessibility until granted (real-time API),
// then attempts to create a CGEvent tap to validate full permission.
// Input Monitoring cannot be reliably polled — IOHIDCheckAccess does not
// update for a running process. The tap creation is the true test.
// Calls onUpdate on each poll so the caller can update the UI.
func (h *cgEventHotkeyListener) WaitForPermissions(ctx context.Context, onUpdate func(accessibility, inputMonitoring bool)) error {
	// Fast path: if the event tap already works, skip everything.
	if C.probeEventTap() == 1 {
		h.logger.Info("permissions already valid", "operation", "WaitForPermissions")
		onUpdate(true, true)
		saveBinaryHash(h.logger)
		return nil
	}

	// Tap failed. Check if this is a new binary (reinstall) vs first install.
	if binaryHashChanged(h.logger) {
		// New binary — old TCC entries are stale and confusing. Clear them
		// so the user sees a clean slate instead of a ghost toggle.
		h.logger.Info("binary changed — resetting stale TCC entries",
			"operation", "WaitForPermissions")
		osExec.Command("tccutil", "reset", "Accessibility", "com.joicetyper.app").Run()
		osExec.Command("tccutil", "reset", "ListenEvent", "com.joicetyper.app").Run()
	}

	// Prompt once — shows the macOS dialog. Will not repeat on subsequent polls.
	C.checkAccessibility(1)
	C.checkInputMonitoring(1)

	// Poll silently until the event tap succeeds.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if C.probeEventTap() == 1 {
			h.logger.Info("permissions verified via event tap probe",
				"operation", "WaitForPermissions")
			onUpdate(true, true)
			saveBinaryHash(h.logger)
			return nil
		}
		acc := C.checkAccessibility(0) == 1
		onUpdate(acc, false)
		h.logger.Info("waiting for permissions",
			"operation", "WaitForPermissions",
			"accessibility", acc)
		C.usleep(2_000_000)
	}
}

// binaryHashChanged returns true if the current executable's hash differs
// from the one stored on the last successful permission grant. Returns true
// if no stored hash exists (first install — harmless to reset nothing).
func binaryHashChanged(logger *slog.Logger) bool {
	stored, err := readStoredHash()
	if err != nil {
		return true // no stored hash → first install or error → safe to reset
	}
	current, err := currentBinaryHash()
	if err != nil {
		logger.Warn("failed to hash current binary", "operation", "binaryHashChanged", "error", err)
		return true
	}
	return stored != current
}

// saveBinaryHash writes the current executable's SHA-256 to the config dir.
func saveBinaryHash(logger *slog.Logger) {
	hash, err := currentBinaryHash()
	if err != nil {
		logger.Warn("failed to hash binary for save", "operation", "saveBinaryHash", "error", err)
		return
	}
	dir, err := DefaultConfigDir()
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, ".binary-hash"), []byte(hash), 0644)
}

func readStoredHash() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, ".binary-hash"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func currentBinaryHash() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func (h *cgEventHotkeyListener) Start(events chan<- HotkeyEvent) error {
	hotkeyLogger = h.logger
	h.logger.Info("starting", "operation", "Start", "trigger_keys", h.triggerKeys)

	flags, err := keysToFlags(h.triggerKeys)
	if err != nil {
		return fmt.Errorf("hotkey.Start: %w", err)
	}

	hotkeyMu.Lock()
	hotkeyEvents = events
	hotkeyMu.Unlock()

	result := C.startHotkeyListener(C.uint64_t(flags))
	if result != 0 {
		return fmt.Errorf("hotkey.Start: failed to create event tap — grant both Accessibility and Input Monitoring in System Settings → Privacy & Security")
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

var flagToKey = map[uint64]string{
	flagFn:     "fn",
	flagShift:  "shift",
	flagCtrl:   "ctrl",
	flagOption: "option",
	flagCmd:    "cmd",
}

func flagsToKeys(flags uint64) []string {
	order := []uint64{flagFn, flagShift, flagCtrl, flagOption, flagCmd}
	var keys []string
	for _, f := range order {
		if flags&f != 0 {
			keys = append(keys, flagToKey[f])
		}
	}
	return keys
}
