//go:build darwin

package darwin

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
	"strings"
	"time"
	"unsafe"

	bridgepkg "voicetype/internal/core/bridge"
	config "voicetype/internal/core/config"
	transcriptionpkg "voicetype/internal/core/transcription"

	"github.com/gordonklaus/portaudio"
)

var postNotification = PostNotification
var defaultModelPath = config.DefaultModelPath
var removeFile = os.Remove
var listAudioDevices = portaudio.Devices
var downloadModelWithProgress = transcriptionpkg.DownloadModelWithProgress
var webSettingsOpenPermissionSettings = openWebSettingsPermissionSettings

func loadWebSettingsPermissionsSnapshot() bridgepkg.PermissionsSnapshot {
	return bridgepkg.PermissionsSnapshot{
		Accessibility:   C.checkAccessibility() == 1,
		InputMonitoring: C.checkInputMonitoring() == 1,
	}
}

func listWebSettingsInputDevices() ([]bridgepkg.DeviceSnapshot, error) {
	devices, err := listAudioDevices()
	if err != nil {
		return nil, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeDevicesEnumerationFailed,
			"Failed to list input devices",
			true,
			nil,
			err,
		)
	}
	snapshots := make([]bridgepkg.DeviceSnapshot, 0, len(devices))
	defaultInput, defaultErr := portaudio.DefaultInputDevice()
	for _, device := range devices {
		if device.MaxInputChannels <= 0 {
			continue
		}
		isDefault := defaultErr == nil && defaultInput != nil && defaultInput.Name == device.Name
		snapshots = append(snapshots, bridgepkg.DeviceSnapshot{
			Name:      device.Name,
			IsDefault: isDefault,
		})
	}
	return snapshots, nil
}

func loadWebSettingsModelSnapshot(modelSize string) (bridgepkg.ModelSnapshot, error) {
	modelPath, err := defaultModelPath(modelSize)
	if err != nil {
		return bridgepkg.ModelSnapshot{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelUnavailable,
			"Failed to resolve model state",
			false,
			map[string]any{"modelSize": modelSize},
			err,
		)
	}
	_, statErr := os.Stat(modelPath)
	return bridgepkg.ModelSnapshot{
		Size:  modelSize,
		Path:  modelPath,
		Ready: statErr == nil,
	}, nil
}

func openWebSettingsPermissionSettings(target string) error {
	switch target {
	case "accessibility":
		C.openAccessibilitySettingsFromGo()
		return nil
	case "input_monitoring":
		C.openInputMonitoringSettingsFromGo()
		return nil
	default:
		return bridgepkg.NewContractError(
			bridgepkg.ErrorCodePermissionInvalidTarget,
			"Unsupported permission settings target",
			false,
			map[string]any{"target": target},
		)
	}
}

func refreshWebSettingsDevices() ([]bridgepkg.DeviceSnapshot, error) {
	if recorder := currentSettingsRecorder(); recorder != nil {
		if err := recorder.RefreshDevices(); err != nil {
			return nil, bridgepkg.WrapContractError(
				bridgepkg.ErrorCodeDevicesRefreshFailed,
				"Failed to refresh input devices",
				true,
				nil,
				err,
			)
		}
	}
	devices, err := listWebSettingsInputDevices()
	if err != nil {
		return nil, err
	}
	publishDevicesChanged(devices)
	return devices, nil
}

func downloadWebSettingsModel(modelSize string) error {
	modelPath, err := defaultModelPath(modelSize)
	if err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDownloadFailed,
			"Failed to resolve model download path",
			false,
			map[string]any{"size": modelSize},
			err,
		)
	}

	ctx := currentPreferencesContext()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := downloadModelWithProgress(ctx, modelPath, modelSize, func(progress float64, downloaded, total int64) {
		publishModelDownloadProgress(modelSize, progress, downloaded, total)
	}, currentSettingsLogger()); err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDownloadFailed,
			"Failed to download model",
			true,
			map[string]any{"size": modelSize},
			err,
		)
	}

	snapshot, err := loadWebSettingsModelSnapshot(modelSize)
	if err != nil {
		return err
	}
	publishModelChanged(snapshot)
	return nil
}

func deleteWebSettingsModel(modelSize string) error {
	if prefsActiveModel == modelSize {
		return bridgepkg.NewContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Cannot delete the active model",
			false,
			map[string]any{"size": modelSize},
		)
	}
	modelPath, err := defaultModelPath(modelSize)
	if err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Failed to resolve model path",
			false,
			map[string]any{"size": modelSize},
			err,
		)
	}
	if removeErr := removeFile(modelPath); removeErr != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Failed to delete model",
			false,
			map[string]any{"size": modelSize},
			removeErr,
		)
	}
	if removeErr := removeFile(modelPath + ".sha256"); removeErr != nil && !os.IsNotExist(removeErr) {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Failed to delete model hash cache",
			false,
			map[string]any{"size": modelSize},
			removeErr,
		)
	}
	publishModelChanged(bridgepkg.ModelSnapshot{
		Size:  modelSize,
		Path:  modelPath,
		Ready: false,
	})
	return nil
}

func useWebSettingsModel(modelSize string) error {
	modelPath, err := defaultModelPath(modelSize)
	if err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelUseFailed,
			"Failed to resolve model path",
			false,
			map[string]any{"size": modelSize},
			err,
		)
	}
	if _, statErr := os.Stat(modelPath); statErr != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelUseFailed,
			"Model is not available to use",
			false,
			map[string]any{"size": modelSize},
			statErr,
		)
	}
	prefsActiveModel = modelSize
	snapshot, err := loadWebSettingsModelSnapshot(modelSize)
	if err != nil {
		return err
	}
	publishModelChanged(snapshot)
	return nil
}

func resolveModelPathForSettings(modelSize string, operation string) (string, bool) {
	modelPath, err := defaultModelPath(modelSize)
	if err != nil {
		currentSettingsLogger().Error("failed to resolve model path", "operation", operation, "model", modelSize, "error", err)
		reportSettingsSaveError(err.Error())
		return "", false
	}
	return modelPath, true
}

// IsFirstRun returns true if no config file exists yet.
func IsFirstRun() bool {
	path, err := config.DefaultConfigPath()
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
	SetSettingsLogger(logger.With("component", "settings"))
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

	// Step 5-6: Populate decode and punctuation lists
	populateDecodeModeList("beam")
	populatePunctuationModeList("conservative")

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
		modelPath, ok := resolveModelPathForSettings(selected, "modelBtn1Clicked")
		if !ok {
			return
		}
		if removeErr := removeFile(modelPath); removeErr != nil {
			currentSettingsLogger().Error("failed to delete model", "operation", "modelBtn1Clicked", "error", removeErr)
			return
		}
		if removeErr := removeFile(modelPath + ".sha256"); removeErr != nil && !os.IsNotExist(removeErr) {
			currentSettingsLogger().Warn("failed to delete model hash cache", "operation", "modelBtn1Clicked", "error", removeErr)
		}
		currentSettingsLogger().Info("model deleted", "operation", "modelBtn1Clicked", "model", selected)
		populateModelList("")
		if snapshot, err := loadWebSettingsModelSnapshot(selected); err == nil {
			publishModelChanged(snapshot)
		}
		return
	}

	modelPath, ok := resolveModelPathForSettings(selected, "modelBtn1Clicked")
	if !ok {
		return
	}

	if _, statErr := os.Stat(modelPath); os.IsNotExist(statErr) {
		// Not downloaded — start download
		currentSettingsLogger().Info("model download started", "operation", "modelBtn1Clicked", "model", selected)
		C.updateModelButtons(3) // downloading state
		go func() {
			ctx := currentPreferencesContext()
			if ctx == nil {
				ctx = context.Background()
			}
			dlErr := transcriptionpkg.DownloadModelWithProgress(ctx, modelPath, selected, func(progress float64, downloaded, total int64) {
				C.updateDownloadProgress(C.double(progress), C.longlong(downloaded), C.longlong(total))
				publishModelDownloadProgress(selected, progress, downloaded, total)
			}, currentSettingsLogger())
			if dlErr != nil {
				currentSettingsLogger().Error("model download failed", "operation", "modelBtn1Clicked", "error", dlErr)
				C.updateModelButtons(4) // download failed
				return
			}
			currentSettingsLogger().Info("model downloaded", "operation", "modelBtn1Clicked", "model", selected)
			C.updateSetupDownloadComplete()
			populateModelList("")
			if snapshot, err := loadWebSettingsModelSnapshot(selected); err == nil {
				publishModelChanged(snapshot)
			}
		}()
	} else if prefsActiveModel == selected {
		// Active — "In Use" button, shouldn't be clickable but ignore
		return
	} else {
		// Downloaded, not active — "Use" clicked
		currentSettingsLogger().Info("model use clicked", "operation", "modelBtn1Clicked", "model", selected)
		prefsActiveModel = selected
		cSize := C.CString(selected)
		C.setActiveModelSize(cSize)
		C.free(unsafe.Pointer(cSize))
		populateModelList(prefsActiveModel)
		if snapshot, err := loadWebSettingsModelSnapshot(selected); err == nil {
			publishModelChanged(snapshot)
		}
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
	modelPath, ok := resolveModelPathForSettings(selected, "updateModelButtonState")
	if !ok {
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

	sizes := make([]*C.char, len(config.ModelOptions))
	descs := make([]*C.char, len(config.ModelOptions))
	defaultIdx := 0
	for i, m := range config.ModelOptions {
		sizes[i] = C.CString(m.Size)
		modelPath, pathErr := defaultModelPath(m.Size)
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
	C.populateSettingsModels(&sizes[0], &descs[0], C.int(len(config.ModelOptions)), C.int(defaultIdx))
	for _, s := range sizes {
		C.free(unsafe.Pointer(s))
	}
	for _, d := range descs {
		C.free(unsafe.Pointer(d))
	}
	updateModelButtonState()
}

func populateLanguageList(selectedCode string) {
	codes := make([]*C.char, len(config.WhisperLanguages))
	names := make([]*C.char, len(config.WhisperLanguages))
	defaultIdx := 0
	for i, lang := range config.WhisperLanguages {
		codes[i] = C.CString(lang.Code)
		names[i] = C.CString(lang.Name)
		if lang.Code == selectedCode {
			defaultIdx = i
		}
	}
	C.populateSettingsLanguages(&codes[0], &names[0], C.int(len(config.WhisperLanguages)), C.int(defaultIdx))
	for _, c := range codes {
		C.free(unsafe.Pointer(c))
	}
	for _, n := range names {
		C.free(unsafe.Pointer(n))
	}
}

func populateDecodeModeList(selectedCode string) {
	codes := make([]*C.char, len(config.DecodeModeOptions))
	names := make([]*C.char, len(config.DecodeModeOptions))
	defaultIdx := 0
	for i, opt := range config.DecodeModeOptions {
		codes[i] = C.CString(opt.Code)
		names[i] = C.CString(opt.Name)
		if opt.Code == selectedCode {
			defaultIdx = i
		}
	}
	C.populateSettingsDecodeModes(&codes[0], &names[0], C.int(len(config.DecodeModeOptions)), C.int(defaultIdx))
	for _, c := range codes {
		C.free(unsafe.Pointer(c))
	}
	for _, n := range names {
		C.free(unsafe.Pointer(n))
	}
}

func populatePunctuationModeList(selectedCode string) {
	codes := make([]*C.char, len(config.PunctuationModeOptions))
	names := make([]*C.char, len(config.PunctuationModeOptions))
	defaultIdx := 0
	for i, opt := range config.PunctuationModeOptions {
		codes[i] = C.CString(opt.Code)
		names[i] = C.CString(opt.Name)
		if opt.Code == selectedCode {
			defaultIdx = i
		}
	}
	C.populateSettingsPunctuationModes(&codes[0], &names[0], C.int(len(config.PunctuationModeOptions)), C.int(defaultIdx))
	for _, c := range codes {
		C.free(unsafe.Pointer(c))
	}
	for _, n := range names {
		C.free(unsafe.Pointer(n))
	}
}

func requireSettingSelection(fieldName string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("settings.requireSettingSelection: empty %s selection", fieldName)
	}
	switch fieldName {
	case "decode_mode":
		if !config.IsValidDecodeMode(value) {
			return "", fmt.Errorf("settings.requireSettingSelection: invalid %s selection %q", fieldName, value)
		}
	case "punctuation_mode":
		if !config.IsValidPunctuationMode(value) {
			return "", fmt.Errorf("settings.requireSettingSelection: invalid %s selection %q", fieldName, value)
		}
	}
	return value, nil
}

func reportSettingsSaveError(detail string) {
	postNotification("JoiceTyper Preferences", "Failed to save settings: "+detail)
}

func writeSetupConfig(deviceName string, language string, modelSize string, triggerKeys []string, l *slog.Logger) error {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return fmt.Errorf("settings.writeSetupConfig: %w", err)
	}

	selectedDecodeMode, err := requireSettingSelection("decode_mode", C.GoString(C.getSelectedDecodeMode()))
	if err != nil {
		return fmt.Errorf("settings.writeSetupConfig: %w", err)
	}
	selectedPunctuationMode, err := requireSettingSelection("punctuation_mode", C.GoString(C.getSelectedPunctuationMode()))
	if err != nil {
		return fmt.Errorf("settings.writeSetupConfig: %w", err)
	}

	cfg := config.Config{
		TriggerKey:      triggerKeys,
		ModelSize:       modelSize,
		Language:        language,
		SampleRate:      16000,
		SoundFeedback:   true,
		InputDevice:     deviceName,
		DecodeMode:      selectedDecodeMode,
		PunctuationMode: selectedPunctuationMode,
		Vocabulary:      C.GoString(C.getVocabularyText()),
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("settings.writeSetupConfig: invalid settings from UI: %w", err)
	}

	if err := config.SaveConfig(cfgPath, cfg); err != nil {
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

// signalHotkeyRestart stops the current hotkey listener and signals the
// main loop to reload config and restart. Called from OpenPreferences()
// on the main thread after the modal closes. hotkey.Stop() dispatches
// [NSApp stop:] which will cause [NSApp run] to exit on the next event
// loop iteration (after this call returns to Cocoa).
func signalHotkeyRestart() {
	currentSettingsLogger().Info("signalling hotkey restart", "operation", "signalHotkeyRestart")
	signalHotkeyRestartCh()
	h := ActiveHotkey()
	if h != nil {
		if err := h.Stop(); err != nil {
			currentSettingsLogger().Warn("failed to stop hotkey for restart",
				"operation", "signalHotkeyRestart", "error", err)
		}
	}
}

// OpenPreferences opens the settings window in preferences mode.
// Called from the main thread (statusbar callback). Sets up the UI on the
// main thread, then moves to a goroutine so [NSApp run] stays responsive.
func OpenPreferences() {
	if !preferencesOpenCompareAndSwap(0, 1) {
		currentSettingsLogger().Warn("preferences already open, ignoring",
			"operation", "OpenPreferences")
		return
	}
	currentSettingsLogger().Info("preferences opened", "operation", "OpenPreferences")

	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		currentSettingsLogger().Error("failed to resolve config path", "operation", "OpenPreferences", "error", err)
		preferencesOpenStore(0)
		return
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		currentSettingsLogger().Error("failed to load config", "operation", "OpenPreferences", "error", err)
		preferencesOpenStore(0)
		return
	}

	// Refresh audio devices to pick up newly connected mics (e.g. Bluetooth).
	if recorder := currentSettingsRecorder(); recorder != nil {
		if refreshErr := recorder.RefreshDevices(); refreshErr != nil {
			currentSettingsLogger().Warn("failed to refresh audio devices", "operation", "OpenPreferences", "error", refreshErr)
		} else if devices, devicesErr := listWebSettingsInputDevices(); devicesErr == nil {
			publishDevicesChanged(devices)
		}
	}

	if shouldUseWebSettings() {
		prefsActiveModel = cfg.ModelSize
		bridgeService := buildSettingsBridgeService(cfg)
		currentSettingsLogger().Info("showing web settings window", "operation", "OpenPreferences")
		if err := ShowWebSettingsWindowWithBridge(context.Background(), bridgeService); err != nil {
			currentSettingsLogger().Error("failed to show web settings window",
				"operation", "OpenPreferences", "error", err)
			postNotification("JoiceTyper Preferences", "Failed to open web preferences. Set JOICETYPER_USE_NATIVE_PREFERENCES=1 only if you need the debug fallback.")
			preferencesOpenStore(0)
			return
		} else {
			return
		}
	}

	// Cancel any previous download still running.
	prefsCtx, prefsCancel := context.WithCancel(context.Background())
	setPreferencesContext(prefsCtx, prefsCancel)

	currentSettingsLogger().Info("showing settings window", "operation", "OpenPreferences")
	C.showSettingsWindow(0)

	currentSettingsLogger().Info("setting permission state", "operation", "OpenPreferences")
	C.setPrefsPermissionState()

	currentSettingsLogger().Info("populating UI fields", "operation", "OpenPreferences")
	populateLanguageList(cfg.Language)
	populateDecodeModeList(cfg.DecodeMode)
	populatePunctuationModeList(cfg.PunctuationMode)
	prefsActiveModel = cfg.ModelSize
	populateModelList(cfg.ModelSize)
	populateMicList(cfg.InputDevice, currentSettingsLogger())

	display := FormatHotkeyDisplay(cfg.TriggerKey)
	cDisplay := C.CString(display)
	C.setSettingsHotkey(cDisplay)
	C.free(unsafe.Pointer(cDisplay))

	currentSettingsLogger().Info("setting vocabulary", "operation", "OpenPreferences", "length", len(cfg.Vocabulary))
	cVocab := C.CString(cfg.Vocabulary)
	C.setVocabularyText(cVocab)
	C.free(unsafe.Pointer(cVocab))

	currentSettingsLogger().Info("UI setup complete, starting wait goroutine", "operation", "OpenPreferences")
	go openPreferencesWait(cfg, cfgPath)
}

// openPreferencesWait blocks until the preferences window closes, then
// saves config and signals a hotkey restart. Runs on a background goroutine.
func openPreferencesWait(cfg config.Config, cfgPath string) {
	defer preferencesOpenStore(0)

	// Poll permissions in background so UI updates if user grants them
	prefsDone := make(chan struct{})
	go func() {
		lastSnapshot := bridgepkg.PermissionsSnapshot{}
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
			snapshot := bridgepkg.PermissionsSnapshot{
				Accessibility:   acc,
				InputMonitoring: inp,
			}
			if snapshot != lastSnapshot {
				publishPermissionsChanged(snapshot)
				lastSnapshot = snapshot
			}
			if acc && inp {
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Block until user closes the window
	currentSettingsLogger().Info("waiting for window close", "operation", "openPreferencesWait")
	C.runSetupEventLoop()

	currentSettingsLogger().Info("window closed", "operation", "openPreferencesWait")

	// Cancel downloads from this preferences session
	cancelPreferencesContext()

	// Stop permission polling goroutine
	close(prefsDone)

	// Re-check permissions and update status bar icon.
	if C.checkAccessibility() == 1 && C.checkInputMonitoring() == 1 {
		UpdateStatusBar(StateReady)
	} else {
		UpdateStatusBar(StateNoPermission)
	}

	if C.isSetupComplete() == 0 {
		currentSettingsLogger().Info("preferences cancelled", "operation", "openPreferencesWait")
		return
	}

	// Read selections
	currentSettingsLogger().Info("reading selections", "operation", "openPreferencesWait")
	selectedDevice := C.GoString(C.getSelectedDevice())
	selectedLang := C.GoString(C.getSelectedLanguage())
	selectedDecodeMode, err := requireSettingSelection("decode_mode", C.GoString(C.getSelectedDecodeMode()))
	if err != nil {
		currentSettingsLogger().Error("invalid decode mode selection", "operation", "openPreferencesWait", "error", err)
		reportSettingsSaveError(err.Error())
		return
	}
	selectedPunctuationMode, err := requireSettingSelection("punctuation_mode", C.GoString(C.getSelectedPunctuationMode()))
	if err != nil {
		currentSettingsLogger().Error("invalid punctuation mode selection", "operation", "openPreferencesWait", "error", err)
		reportSettingsSaveError(err.Error())
		return
	}
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
	cfg.DecodeMode = selectedDecodeMode
	cfg.PunctuationMode = selectedPunctuationMode
	cfg.TriggerKey = triggerKeys
	cfg.Vocabulary = C.GoString(C.getVocabularyText())
	if selectedModel != "" {
		cfg.ModelSize = selectedModel
	}

	if err := cfg.Validate(); err != nil {
		currentSettingsLogger().Error("invalid settings from UI, not saving", "operation", "OpenPreferences", "error", err)
		reportSettingsSaveError(err.Error())
		return
	}

	currentSettingsLogger().Info("saving config", "operation", "openPreferencesWait",
		"device", cfg.InputDevice, "language", cfg.Language,
		"model", cfg.ModelSize, "decode_mode", cfg.DecodeMode,
		"punctuation_mode", cfg.PunctuationMode, "vocabulary_length", len(cfg.Vocabulary))

	if writeErr := config.SaveConfig(cfgPath, cfg); writeErr != nil {
		currentSettingsLogger().Error("failed to write config", "operation", "openPreferencesWait", "error", writeErr)
		reportSettingsSaveError(writeErr.Error())
		return
	}

	currentSettingsLogger().Info("config saved", "operation", "openPreferencesWait")
	signalHotkeyRestart()
}

func FormatHotkeyDisplay(keys []string) string {
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
