package main

/*
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#include "setup_darwin.h"
#include "hotkey_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/gordonklaus/portaudio"
	"gopkg.in/yaml.v3"
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
func RunSetupWizard(logger *slog.Logger) (string, error) {
	l := logger.With("component", "setup")
	l.Info("starting setup wizard", "operation", "RunSetupWizard")

	C.showSetupWindow()

	// Step 1: Accessibility polling in background goroutine
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			granted := C.checkAccessibility(1) == 1
			C.updateSetupAccessibility(boolToCInt(granted))
			if granted {
				l.Info("accessibility granted", "operation", "RunSetupWizard")
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Step 2: Populate mic list (synchronous — no dispatch_async issue)
	populateMicList(l)

	// Step 3: Model download in background goroutine
	go func() {
		const setupModelSize = "small"
		modelPath, pathErr := DefaultModelPath(setupModelSize)
		if pathErr != nil {
			l.Error("failed to resolve model path", "operation", "RunSetupWizard", "error", pathErr)
			cErr := C.CString(pathErr.Error())
			C.updateSetupDownloadFailed(cErr)
			C.free(unsafe.Pointer(cErr))
			return
		}
		// Validate cached model using the same path as main startup
		if validateCachedModel(modelPath, setupModelSize, l) {
			C.updateSetupDownloadComplete()
			C.updateSetupReady()
			return
		}
		dlErr := downloadModelWithProgress(modelPath, setupModelSize, func(progress float64, downloaded, total int64) {
			C.updateSetupDownloadProgress(C.double(progress), C.longlong(downloaded), C.longlong(total))
		}, l)
		if dlErr != nil {
			l.Error("model download failed", "operation", "RunSetupWizard", "error", dlErr)
			cErr := C.CString(dlErr.Error())
			C.updateSetupDownloadFailed(cErr)
			C.free(unsafe.Pointer(cErr))
			return
		}
		C.updateSetupDownloadComplete()
		C.updateSetupReady()
	}()

	// Block here — [NSApp run] processes events until Continue is clicked
	C.runSetupEventLoop()

	// Cleanup: signal accessibility goroutine to stop
	close(done)

	// Check if user completed or cancelled
	if C.isSetupComplete() == 0 {
		return "", fmt.Errorf("setup.RunSetupWizard: setup cancelled by user")
	}

	// Read selected device
	cDevice := C.getSelectedDevice()
	selectedDevice := C.GoString(cDevice)

	// Write config
	if err := writeSetupConfig(selectedDevice, l); err != nil {
		return "", fmt.Errorf("setup.RunSetupWizard: %w", err)
	}

	l.Info("setup complete", "operation", "RunSetupWizard", "device", selectedDevice)
	return selectedDevice, nil
}

func populateMicList(l *slog.Logger) {
	devices, devErr := portaudio.Devices()
	if devErr != nil {
		l.Error("failed to list devices", "operation", "populateMicList", "error", devErr)
		return
	}
	var inputNames []string
	for _, d := range devices {
		if d.MaxInputChannels > 0 {
			inputNames = append(inputNames, d.Name)
		}
	}
	if len(inputNames) == 0 {
		return
	}
	cNames := make([]*C.char, len(inputNames))
	for i, name := range inputNames {
		cNames[i] = C.CString(name)
	}
	C.populateSetupDevices(&cNames[0], C.int(len(inputNames)), 0)
	for _, cn := range cNames {
		C.free(unsafe.Pointer(cn))
	}
}

func writeSetupConfig(deviceName string, l *slog.Logger) error {
	cfgPath, err := DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("writeSetupConfig: %w", err)
	}

	cfg := Config{
		TriggerKey:    []string{"fn", "shift"},
		ModelSize:     "small",
		Language:      "",
		SampleRate:    16000,
		SoundFeedback: true,
		InputDevice:   deviceName,
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("writeSetupConfig: marshal: %w", err)
	}

	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("writeSetupConfig: create dir: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("writeSetupConfig: write: %w", err)
	}

	l.Info("config written", "operation", "writeSetupConfig", "path", cfgPath, "device", deviceName)
	return nil
}

func boolToCInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
