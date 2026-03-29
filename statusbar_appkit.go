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
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		// Signal to self failed — should never happen, but don't leave
		// the app running silently. Force exit as last resort.
		os.Exit(1)
	}
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
