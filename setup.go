package main

/*
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#include "setup_darwin.h"
#include "hotkey_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"log/slog"
	"os"
	"time"
	"unsafe"

	"github.com/gordonklaus/portaudio"
)

// IsFirstRun returns true if no config file exists yet.
func IsFirstRun() bool {
	path, err := DefaultConfigPath()
	if err != nil {
		return true
	}
	_, err = os.Stat(path)
	return os.IsNotExist(err)
}

// RunSetupWizard runs the first-run setup flow.
// Returns the selected device name and nil on success.
// Must be called from the main thread.
func RunSetupWizard(logger *slog.Logger) (selectedDevice string, err error) {
	l := logger.With("component", "setup")
	l.Info("starting setup wizard", "operation", "RunSetupWizard")

	C.showSetupWindow()

	// Step 1: Poll accessibility in background
	go func() {
		for {
			granted := C.checkAccessibility(1) == 1
			C.updateSetupAccessibility(boolToCInt(granted))
			if granted {
				l.Info("accessibility granted", "operation", "RunSetupWizard")
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Step 2: Populate mic list
	devices, devErr := portaudio.Devices()
	if devErr != nil {
		l.Error("failed to list devices", "operation", "RunSetupWizard", "error", devErr)
	} else {
		var inputNames []string
		for _, d := range devices {
			if d.MaxInputChannels > 0 {
				inputNames = append(inputNames, d.Name)
			}
		}
		if len(inputNames) > 0 {
			cNames := make([]*C.char, len(inputNames))
			for i, name := range inputNames {
				cNames[i] = C.CString(name)
			}
			C.populateSetupDevices(&cNames[0], C.int(len(inputNames)), 0)
			for _, cn := range cNames {
				C.free(unsafe.Pointer(cn))
			}
		}
	}

	// Step 3: Model download in background
	go func() {
		modelPath, pathErr := DefaultModelPath("small")
		if pathErr != nil {
			l.Error("failed to resolve model path", "operation", "RunSetupWizard", "error", pathErr)
			return
		}
		if info, statErr := os.Stat(modelPath); statErr == nil && info.Size() > 100*1024*1024 {
			C.updateSetupDownloadComplete()
			C.updateSetupReady()
			return
		}
		dlErr := downloadModelWithProgress(modelPath, func(progress float64, downloaded, total int64) {
			C.updateSetupDownloadProgress(C.double(progress), C.longlong(downloaded), C.longlong(total))
		}, l)
		if dlErr != nil {
			l.Error("model download failed", "operation", "RunSetupWizard", "error", dlErr)
			return
		}
		C.updateSetupDownloadComplete()
		C.updateSetupReady()
	}()

	// Return -- the caller runs [NSApp run] which processes events until Continue is clicked
	return "", nil
}

func boolToCInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
