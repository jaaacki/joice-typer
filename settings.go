package main

/*
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#include "settings_darwin.h"
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

	// Step 2: Input Monitoring polling in background goroutine
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			granted := C.checkInputMonitoring(1) == 1
			C.updateSetupInputMonitoring(boolToCInt(granted))
			if granted {
				l.Info("input monitoring granted", "operation", "RunSetupWizard")
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Step 3: Populate mic list (synchronous — no dispatch_async issue)
	populateMicList(l)

	// Step 4: Populate language list
	populateLanguageList("en")

	// Step 6: Model download in background goroutine
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

	// Set default hotkey display
	cDefaultHotkey := C.CString("Fn + Shift")
	C.setSettingsHotkey(cDefaultHotkey)
	C.free(unsafe.Pointer(cDefaultHotkey))

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

	// Read selected language
	selectedLang := C.GoString(C.getSelectedLanguage())

	// Read hotkey flags
	hotkeyFlags := uint64(C.getSettingsHotkeyFlags())
	var triggerKeys []string
	if hotkeyFlags != 0 {
		triggerKeys = flagsToKeys(hotkeyFlags)
	} else {
		triggerKeys = []string{"fn", "shift"}
	}

	// Write config
	if err := writeSetupConfig(selectedDevice, selectedLang, triggerKeys, l); err != nil {
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

func populateLanguageList(selectedCode string) {
	codes := make([]*C.char, len(WhisperLanguages))
	names := make([]*C.char, len(WhisperLanguages))
	defaultIdx := 0
	for i, lang := range WhisperLanguages {
		codes[i] = C.CString(lang.Code)
		names[i] = C.CString(lang.Name)
		if lang.Code == selectedCode {
			defaultIdx = i
		}
	}
	C.populateSettingsLanguages(&codes[0], &names[0], C.int(len(WhisperLanguages)), C.int(defaultIdx))
	for _, c := range codes {
		C.free(unsafe.Pointer(c))
	}
	for _, n := range names {
		C.free(unsafe.Pointer(n))
	}
}

func writeSetupConfig(deviceName string, language string, triggerKeys []string, l *slog.Logger) error {
	cfgPath, err := DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("writeSetupConfig: %w", err)
	}

	cfg := Config{
		TriggerKey:    triggerKeys,
		ModelSize:     "small",
		Language:      language,
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
