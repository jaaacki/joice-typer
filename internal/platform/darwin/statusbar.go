//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework Cocoa
#include "statusbar_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"os"
	"syscall"
	"unsafe"
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
	storeCurrentAppState(state)
	C.updateStatusBar(C.int(state))
}

// SetStatusBarHotkeyText sets the hotkey display string shown in the ready state.
func SetStatusBarHotkeyText(text string) {
	cText := C.CString(text)
	C.setStatusBarHotkeyText(cText)
	C.free(unsafe.Pointer(cText))
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
	// This callback runs on the main thread (NSApp event dispatch).
	// OpenPreferences does UI setup (must be main thread), then waits
	// for window close on a goroutine so [NSApp run] stays responsive.
	OpenPreferences()
}
