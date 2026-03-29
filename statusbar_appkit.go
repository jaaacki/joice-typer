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
		return
	}
	p.Signal(syscall.SIGTERM)
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
