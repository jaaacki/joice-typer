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
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gordonklaus/portaudio"
	"gopkg.in/yaml.v3"
)

var settingsLogger *slog.Logger
var settingsRecorder Recorder
var prefsOpen int32 // atomic: 1 = preferences window is open

var (
	prefsMu     sync.Mutex // protects prefsCtx and prefsCancel
	prefsCtx    context.Context
	prefsCancel context.CancelFunc
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
			granted := C.checkAccessibility() == 1
			C.updateSetupAccessibility(boolToCInt(granted))
			if granted {
				l.Info("accessibility granted", "operation", "RunSetupWizard")
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Step 2: Input Monitoring polling in background goroutine.
	// Uses CGPreflightListenEventAccess via checkInputMonitoring.
	go func() {
		// No prompt — our UI guides the user via "Open" buttons
		for {
			select {
			case <-done:
				return
			case <-wizardCtx.Done():
				return
			default:
			}
			granted := C.checkInputMonitoring() == 1
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
	prefsActiveModel = "small"
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

// prefsActiveModel tracks the in-use model for the current preferences session.
var prefsActiveModel string

// deleteConfirmPending tracks whether the delete confirmation is showing.
var deleteConfirmPending bool

//export modelBtn1Clicked
func modelBtn1Clicked() {
	selected := C.GoString(C.getDropdownModel())
	if selected == "" {
		return
	}

	if deleteConfirmPending {
		// "Confirm?" was clicked — actually delete
		deleteConfirmPending = false
		modelPath, err := DefaultModelPath(selected)
		if err != nil {
			return
		}
		if removeErr := os.Remove(modelPath); removeErr != nil {
			settingsLogger.Error("failed to delete model", "operation", "modelBtn1Clicked", "error", removeErr)
			return
		}
		os.Remove(modelPath + ".sha256")
		settingsLogger.Info("model deleted", "operation", "modelBtn1Clicked", "model", selected)
		populateModelList("")
		return
	}

	modelPath, err := DefaultModelPath(selected)
	if err != nil {
		return
	}

	if _, statErr := os.Stat(modelPath); os.IsNotExist(statErr) {
		// Not downloaded — start download
		settingsLogger.Info("model download started", "operation", "modelBtn1Clicked", "model", selected)
		C.updateModelButtons(3) // downloading state
		go func() {
			prefsMu.Lock()
			ctx := prefsCtx
			prefsMu.Unlock()
			if ctx == nil {
				ctx = context.Background()
			}
			dlErr := downloadModelWithProgress(ctx, modelPath, selected, func(progress float64, downloaded, total int64) {
				C.updateDownloadProgress(C.double(progress), C.longlong(downloaded), C.longlong(total))
			}, settingsLogger)
			if dlErr != nil {
				settingsLogger.Error("model download failed", "operation", "modelBtn1Clicked", "error", dlErr)
				C.updateModelButtons(4) // download failed
				return
			}
			settingsLogger.Info("model downloaded", "operation", "modelBtn1Clicked", "model", selected)
			C.updateSetupDownloadComplete()
			populateModelList("")
		}()
	} else if prefsActiveModel == selected {
		// Active — "In Use" button, shouldn't be clickable but ignore
		return
	} else {
		// Downloaded, not active — "Use" clicked
		settingsLogger.Info("model use clicked", "operation", "modelBtn1Clicked", "model", selected)
		prefsActiveModel = selected
		cSize := C.CString(selected)
		C.setActiveModelSize(cSize)
		C.free(unsafe.Pointer(cSize))
		populateModelList(prefsActiveModel)
	}
}

//export modelBtn2Clicked
func modelBtn2Clicked() {
	if deleteConfirmPending {
		// "Cancel" clicked — revert to normal state
		deleteConfirmPending = false
		updateModelButtonState()
		return
	}

	// "Delete" clicked — show confirmation
	deleteConfirmPending = true
	C.updateModelButtons(5) // delete confirm state
}

//export modelDropdownChanged
func modelDropdownChanged() {
	deleteConfirmPending = false
	updateModelButtonState()
}

func updateModelButtonState() {
	selected := C.GoString(C.getDropdownModel())
	if selected == "" {
		return
	}
	modelPath, err := DefaultModelPath(selected)
	if err != nil {
		return
	}
	if _, statErr := os.Stat(modelPath); os.IsNotExist(statErr) {
		C.updateModelButtons(0) // not downloaded
	} else if prefsActiveModel == selected {
		C.updateModelButtons(1) // active
	} else {
		C.updateModelButtons(2) // downloaded, not active
	}
}

// populateModelList rebuilds the dropdown. selectSize controls which model
// is selected in the dropdown (pass "" to keep current selection).
func populateModelList(selectSize string) {
	if selectSize == "" {
		selectSize = C.GoString(C.getDropdownModel())
	}

	cSize := C.CString(prefsActiveModel)
	C.setActiveModelSize(cSize)
	C.free(unsafe.Pointer(cSize))

	sizes := make([]*C.char, len(ModelOptions))
	descs := make([]*C.char, len(ModelOptions))
	defaultIdx := 0
	for i, m := range ModelOptions {
		sizes[i] = C.CString(m.Size)
		modelPath, pathErr := DefaultModelPath(m.Size)
		cached := ""
		if pathErr == nil {
			if _, err := os.Stat(modelPath); err == nil {
				cached = " \u2713"
			}
		}
		descs[i] = C.CString(m.Description + cached)
		if m.Size == selectSize {
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
	updateModelButtonState()
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
		return fmt.Errorf("settings.writeSetupConfig: %w", err)
	}

	cfg := Config{
		TriggerKey:    triggerKeys,
		ModelSize:     modelSize,
		Language:      language,
		SampleRate:    16000,
		SoundFeedback: true,
		InputDevice:   deviceName,
		Vocabulary:    C.GoString(C.getVocabularyText()),
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("settings.writeSetupConfig: invalid settings from UI: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("settings.writeSetupConfig: marshal: %w", err)
	}

	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("settings.writeSetupConfig: create dir: %w", err)
	}

	if err := atomicWriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("settings.writeSetupConfig: write: %w", err)
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
	settingsLogger.Info("signalling hotkey restart", "operation", "signalHotkeyRestart")
	select {
	case hotkeyRestartCh <- struct{}{}:
	default:
	}
	activeHotkeyMu.Lock()
	h := activeHotkey
	activeHotkeyMu.Unlock()
	if h != nil {
		if err := h.Stop(); err != nil {
			settingsLogger.Warn("failed to stop hotkey for restart",
				"operation", "signalHotkeyRestart", "error", err)
		}
	}
}

// OpenPreferences opens the settings window in preferences mode.
// Called from the main thread (statusbar callback). Sets up the UI on the
// main thread, then moves to a goroutine so [NSApp run] stays responsive.
func OpenPreferences() {
	if !atomic.CompareAndSwapInt32(&prefsOpen, 0, 1) {
		settingsLogger.Warn("preferences already open, ignoring",
			"operation", "OpenPreferences")
		return
	}
	settingsLogger.Info("preferences opened", "operation", "OpenPreferences")

	// Cancel any previous download still running
	prefsMu.Lock()
	if prefsCancel != nil {
		prefsCancel()
	}
	prefsCtx, prefsCancel = context.WithCancel(context.Background())
	prefsMu.Unlock()

	cfgPath, err := DefaultConfigPath()
	if err != nil {
		settingsLogger.Error("failed to resolve config path", "operation", "OpenPreferences", "error", err)
		atomic.StoreInt32(&prefsOpen, 0)
		return
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		settingsLogger.Error("failed to load config", "operation", "OpenPreferences", "error", err)
		atomic.StoreInt32(&prefsOpen, 0)
		return
	}

	settingsLogger.Info("showing settings window", "operation", "OpenPreferences")
	C.showSettingsWindow(0)

	settingsLogger.Info("setting permission state", "operation", "OpenPreferences")
	C.setPrefsPermissionState()

	// Refresh audio devices to pick up newly connected mics (e.g. Bluetooth).
	if settingsRecorder != nil {
		if refreshErr := settingsRecorder.RefreshDevices(); refreshErr != nil {
			settingsLogger.Warn("failed to refresh audio devices", "operation", "OpenPreferences", "error", refreshErr)
		}
	}

	settingsLogger.Info("populating UI fields", "operation", "OpenPreferences")
	populateLanguageList(cfg.Language)
	prefsActiveModel = cfg.ModelSize
	populateModelList(cfg.ModelSize)
	populateMicList(cfg.InputDevice, settingsLogger)

	display := formatHotkeyDisplay(cfg.TriggerKey)
	cDisplay := C.CString(display)
	C.setSettingsHotkey(cDisplay)
	C.free(unsafe.Pointer(cDisplay))

	settingsLogger.Info("setting vocabulary", "operation", "OpenPreferences", "length", len(cfg.Vocabulary))
	cVocab := C.CString(cfg.Vocabulary)
	C.setVocabularyText(cVocab)
	C.free(unsafe.Pointer(cVocab))

	settingsLogger.Info("UI setup complete, starting wait goroutine", "operation", "OpenPreferences")
	go openPreferencesWait(cfg, cfgPath)
}

// openPreferencesWait blocks until the preferences window closes, then
// saves config and signals a hotkey restart. Runs on a background goroutine.
func openPreferencesWait(cfg Config, cfgPath string) {
	defer atomic.StoreInt32(&prefsOpen, 0)

	// Poll permissions in background so UI updates if user grants them
	prefsDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-prefsDone:
				return
			default:
			}
			acc := C.checkAccessibility() == 1
			inp := C.checkInputMonitoring() == 1
			C.updateSetupAccessibility(boolToCInt(acc))
			C.updateSetupInputMonitoring(boolToCInt(inp))
			if acc && inp {
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Block until user closes the window
	settingsLogger.Info("waiting for window close", "operation", "openPreferencesWait")
	C.runSetupEventLoop()

	settingsLogger.Info("window closed", "operation", "openPreferencesWait")

	// Cancel downloads from this preferences session
	prefsMu.Lock()
	if prefsCancel != nil {
		prefsCancel()
	}
	prefsMu.Unlock()

	// Stop permission polling goroutine
	close(prefsDone)

	// Re-check permissions and update status bar icon.
	if C.checkAccessibility() == 1 && C.checkInputMonitoring() == 1 {
		UpdateStatusBar(StateReady)
	} else {
		UpdateStatusBar(StateNoPermission)
	}

	if C.isSetupComplete() == 0 {
		settingsLogger.Info("preferences cancelled", "operation", "openPreferencesWait")
		return
	}

	// Read selections
	settingsLogger.Info("reading selections", "operation", "openPreferencesWait")
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
	cfg.Vocabulary = C.GoString(C.getVocabularyText())
	if selectedModel != "" {
		cfg.ModelSize = selectedModel
	}

	if err := cfg.Validate(); err != nil {
		settingsLogger.Error("invalid settings from UI, not saving", "operation", "OpenPreferences", "error", err)
		return
	}

	settingsLogger.Info("saving config", "operation", "openPreferencesWait",
		"device", cfg.InputDevice, "language", cfg.Language,
		"model", cfg.ModelSize, "vocabulary_length", len(cfg.Vocabulary))

	data, marshalErr := yaml.Marshal(&cfg)
	if marshalErr != nil {
		settingsLogger.Error("failed to marshal config", "operation", "openPreferencesWait", "error", marshalErr)
		return
	}
	if writeErr := atomicWriteFile(cfgPath, data, 0644); writeErr != nil {
		settingsLogger.Error("failed to write config", "operation", "openPreferencesWait", "error", writeErr)
		return
	}

	settingsLogger.Info("config saved", "operation", "openPreferencesWait")
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

