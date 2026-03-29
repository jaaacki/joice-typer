package main

/*
#cgo LDFLAGS: -framework Cocoa
#include "statusbar_darwin.h"
*/
import "C"

import (
	"os"
	"syscall"
)

// InitStatusBar creates the menu bar icon. Must be called from the main thread.
func InitStatusBar() {
	C.initStatusBar()
}

// InitStatusBarAsync dispatches status bar creation to the main thread.
// Safe to call from any goroutine. Blocks until creation completes.
func InitStatusBarAsync() {
	C.initStatusBarOnMainThread()
}

// UpdateStatusBar changes the menu bar icon state.
func UpdateStatusBar(state AppState) {
	C.updateStatusBar(C.int(state))
}

//export statusBarQuitClicked
func statusBarQuitClicked() {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		// Last resort — FindProcess should never fail for own pid
		syscall.Kill(os.Getpid(), syscall.SIGTERM)
		return
	}
	p.Signal(syscall.SIGTERM)
	// Signal send failure is non-fatal — the signal handler will
	// catch it. If Signal itself fails, the process is in a bad
	// state and will eventually be cleaned up by the OS.
}

//export statusBarPreferencesClicked
func statusBarPreferencesClicked() {
	// Must NOT use `go` — Cocoa UI calls in OpenPreferences must run on
	// the main thread. This callback is already on the main thread (called
	// from NSApp's event dispatch). The modal window inside OpenPreferences
	// is a standard Cocoa pattern: [NSApp runModalForWindow:] nests within
	// the existing [NSApp run] event loop.
	OpenPreferences()
}
