package main

/*
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#include "settings_darwin.h"
#include "hotkey_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/gordonklaus/portaudio"
	"gopkg.in/yaml.v3"
)

var settingsLogger *slog.Logger

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
func RunSetupWizard(ctx context.Context, logger *slog.Logger) (string, error) {
	l := logger.With("component", "setup")
	settingsLogger = logger.With("component", "settings")
	l.Info("starting setup wizard", "operation", "RunSetupWizard")

	C.showSettingsWindow(1)

	// Create a local context cancelled both by the parent ctx (SIGTERM)
	// and by modal close (done channel). This ensures the download
	// goroutine stops when the user closes the wizard, not just on SIGTERM.
	done := make(chan struct{})
	wizardCtx, wizardCancel := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-wizardCtx.Done():
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
			case <-wizardCtx.Done():
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
	populateMicList("", l)

	// Step 4: Populate language list
	populateLanguageList("en")

	// Step 5.5: Populate model list (default to small for onboarding)
	populateModelList("small")

	// Model download happens AFTER the user clicks Continue — not before.
	// This ensures the selected model (not the default) is downloaded.

	// Set default hotkey display
	cDefaultHotkey := C.CString("Fn + Shift")
	C.setSettingsHotkey(cDefaultHotkey)
	C.free(unsafe.Pointer(cDefaultHotkey))

	// Block here — [NSApp run] processes events until Continue is clicked
	C.runSetupEventLoop()

	// Cleanup: cancel all background goroutines (permissions + download)
	wizardCancel()
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

	// Read selected model
	selectedModel := C.GoString(C.getSelectedModel())
	if selectedModel == "" {
		selectedModel = "small"
	}

	// Read hotkey flags and keycode
	hotkeyFlags := uint64(C.getSettingsHotkeyFlags())
	hotkeyKeycode := int(C.getSettingsHotkeyKeycode())
	var triggerKeys []string
	if hotkeyFlags != 0 || hotkeyKeycode >= 0 {
		triggerKeys = hotkeyToKeys(hotkeyFlags, hotkeyKeycode)
	} else {
		triggerKeys = []string{"fn", "shift"}
	}

	// Write config
	if err := writeSetupConfig(selectedDevice, selectedLang, selectedModel, triggerKeys, l); err != nil {
		return "", fmt.Errorf("setup.RunSetupWizard: %w", err)
	}

	l.Info("setup complete", "operation", "RunSetupWizard", "device", selectedDevice)
	return selectedDevice, nil
}

func populateMicList(selectedDevice string, l *slog.Logger) {
	devices, err := portaudio.Devices()
	if err != nil {
		l.Error("failed to list devices", "operation", "populateMicList", "error", err)
		return
	}
	var inputNames []string
	defaultIdx := 0
	for _, d := range devices {
		if d.MaxInputChannels > 0 {
			if d.Name == selectedDevice {
				defaultIdx = len(inputNames)
			}
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
	C.populateSetupDevices(&cNames[0], C.int(len(inputNames)), C.int(defaultIdx))
	for _, cn := range cNames {
		C.free(unsafe.Pointer(cn))
	}
}

func populateModelList(selectedSize string) {
	l := settingsLogger
	if l == nil {
		l = slog.Default()
	}
	sizes := make([]*C.char, len(ModelOptions))
	descs := make([]*C.char, len(ModelOptions))
	defaultIdx := 0
	for i, m := range ModelOptions {
		sizes[i] = C.CString(m.Size)
		// Check if model is cached AND valid
		modelPath, _ := DefaultModelPath(m.Size)
		cached := ""
		if validateCachedModel(modelPath, m.Size, l) {
			cached = " \u2713"
		}
		descs[i] = C.CString(m.Description + cached)
		if m.Size == selectedSize {
			defaultIdx = i
		}
	}
	C.populateSettingsModels(&sizes[0], &descs[0], C.int(len(ModelOptions)), C.int(defaultIdx))
	for _, s := range sizes {
		C.free(unsafe.Pointer(s))
	}
	for _, d := range descs {
		C.free(unsafe.Pointer(d))
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

func writeSetupConfig(deviceName string, language string, modelSize string, triggerKeys []string, l *slog.Logger) error {
	cfgPath, err := DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("writeSetupConfig: %w", err)
	}

	cfg := Config{
		TriggerKey:    triggerKeys,
		ModelSize:     modelSize,
		Language:      language,
		SampleRate:    16000,
		SoundFeedback: true,
		InputDevice:   deviceName,
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("writeSetupConfig: invalid settings from UI: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("writeSetupConfig: marshal: %w", err)
	}

	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("writeSetupConfig: create dir: %w", err)
	}

	if err := atomicWriteFile(cfgPath, data, 0644); err != nil {
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

var hotkeyRestartCh = make(chan struct{}, 1)

// signalHotkeyRestart stops the current hotkey listener and signals the
// main loop to reload config and restart. Called from OpenPreferences()
// on the main thread after the modal closes. hotkey.Stop() dispatches
// [NSApp stop:] which will cause [NSApp run] to exit on the next event
// loop iteration (after this call returns to Cocoa).
func signalHotkeyRestart() {
	select {
	case hotkeyRestartCh <- struct{}{}:
	default:
	}
	activeHotkeyMu.Lock()
	h := activeHotkey
	activeHotkeyMu.Unlock()
	if h != nil {
		h.Stop()
	}
}

// OpenPreferences opens the settings window in preferences mode.
// Called from statusbar callback goroutine. Blocks until window closes.
func OpenPreferences() {
	cfgPath, err := DefaultConfigPath()
	if err != nil {
		if settingsLogger != nil {
			settingsLogger.Error("failed to resolve config path", "operation", "OpenPreferences", "error", err)
		}
		return
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		if settingsLogger != nil {
			settingsLogger.Error("failed to load config", "operation", "OpenPreferences", "error", err)
		}
		return
	}

	C.showSettingsWindow(0)

	// Set permission indicators from actual current state — don't assume
	// granted just because the app is running (prefs is reachable during
	// StateNoPermission via the status bar menu).
	C.setPrefsPermissionState()

	// Poll permissions in background so UI updates if user grants them
	prefsDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-prefsDone:
				return
			default:
			}
			acc := C.checkAccessibility(0) == 1
			inp := C.probeEventTap() == 1
			C.updateSetupAccessibility(boolToCInt(acc))
			C.updateSetupInputMonitoring(boolToCInt(inp))
			if acc && inp {
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Pre-populate from current config
	populateLanguageList(cfg.Language)
	populateModelList(cfg.ModelSize)
	populateMicList(cfg.InputDevice, settingsLogger)

	// Set current hotkey display
	display := formatHotkeyDisplay(cfg.TriggerKey)
	cDisplay := C.CString(display)
	C.setSettingsHotkey(cDisplay)
	C.free(unsafe.Pointer(cDisplay))

	// Block until user clicks Save or closes
	C.runSetupEventLoop()

	// Stop permission polling goroutine
	close(prefsDone)

	// Re-check permissions and update status bar icon.
	// If permissions were revoked/granted while prefs was open, the
	// status bar should reflect the current state.
	if C.probeEventTap() == 1 {
		UpdateStatusBar(StateReady)
	} else {
		UpdateStatusBar(StateNoPermission)
	}

	if C.isSetupComplete() == 0 {
		return // cancelled
	}

	// Read selections
	selectedDevice := C.GoString(C.getSelectedDevice())
	selectedLang := C.GoString(C.getSelectedLanguage())
	selectedModel := C.GoString(C.getSelectedModel())
	hotkeyFlags := uint64(C.getSettingsHotkeyFlags())
	hotkeyKeycode := int(C.getSettingsHotkeyKeycode())

	var triggerKeys []string
	if hotkeyFlags != 0 || hotkeyKeycode >= 0 {
		triggerKeys = hotkeyToKeys(hotkeyFlags, hotkeyKeycode)
	} else {
		triggerKeys = cfg.TriggerKey // keep existing
	}

	cfg.InputDevice = selectedDevice
	cfg.Language = selectedLang
	cfg.TriggerKey = triggerKeys
	if selectedModel != "" {
		cfg.ModelSize = selectedModel
	}

	if err := cfg.Validate(); err != nil {
		if settingsLogger != nil {
			settingsLogger.Error("invalid settings from UI, not saving", "operation", "OpenPreferences", "error", err)
		}
		return
	}

	data, marshalErr := yaml.Marshal(&cfg)
	if marshalErr != nil {
		if settingsLogger != nil {
			settingsLogger.Error("failed to marshal config", "operation", "OpenPreferences", "error", marshalErr)
		}
		return
	}
	if writeErr := atomicWriteFile(cfgPath, data, 0644); writeErr != nil {
		if settingsLogger != nil {
			settingsLogger.Error("failed to write config", "operation", "OpenPreferences", "error", writeErr)
		}
		return
	}

	// Signal hotkey restart
	signalHotkeyRestart()
}

func formatHotkeyDisplay(keys []string) string {
	nameMap := map[string]string{
		"fn": "Fn", "shift": "Shift", "ctrl": "Ctrl",
		"option": "Option", "cmd": "Cmd",
		"space": "Space", "tab": "Tab", "return": "Return",
		"escape": "Escape", "delete": "Delete",
	}
	var parts []string
	for _, k := range keys {
		if n, ok := nameMap[k]; ok {
			parts = append(parts, n)
		} else {
			parts = append(parts, strings.ToUpper(k))
		}
	}
	return strings.Join(parts, " + ")
}

