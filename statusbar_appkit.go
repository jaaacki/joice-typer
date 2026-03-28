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
