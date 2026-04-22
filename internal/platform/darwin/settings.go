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
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	audiopkg "voicetype/internal/core/audio"
	bridgepkg "voicetype/internal/core/bridge"
	config "voicetype/internal/core/config"
	loggingpkg "voicetype/internal/core/logging"
	transcriptionpkg "voicetype/internal/core/transcription"
)

var postNotification = PostNotification
var defaultModelPath = config.DefaultModelPath
var removeFile = os.Remove
var listAudioDevices = audiopkg.ListInputDeviceSnapshots
var downloadModelWithProgress = transcriptionpkg.DownloadModelWithProgress
var registerLogWriteObserver = loggingpkg.RegisterWriteObserver
var copyTextToClipboard = func(text string) error {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	if C.copyTextToClipboard(cText) != 1 {
		return fmt.Errorf("copy text to clipboard returned false")
	}
	return nil
}
var webSettingsOpenPermissionSettings = openWebSettingsPermissionSettings
var webSettingsLogPath = func() (string, error) {
	dir, err := config.DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "voicetype.log"), nil
}
var loadPermissionsSnapshot = func() bridgepkg.PermissionsSnapshot {
	return bridgepkg.PermissionsSnapshot{
		Accessibility:   C.checkAccessibility() == 1,
		InputMonitoring: C.checkInputMonitoring() == 1,
	}
}
var applyNativePermissionSnapshot = func(snapshot bridgepkg.PermissionsSnapshot) {
	C.updateSetupAccessibility(boolToCInt(snapshot.Accessibility))
	C.updateSetupInputMonitoring(boolToCInt(snapshot.InputMonitoring))
}
var permissionPollingInterval = 2 * time.Second

var (
	webSettingsLogObserverOnce   sync.Once
	webSettingsLogUpdateMu       sync.Mutex
	webSettingsLogUpdateTimer    *time.Timer
	webSettingsLogRefreshRunning bool
	webSettingsDownloadMu        sync.Mutex
	webSettingsActiveDownload    string
	webSettingsInputLevelOnce    sync.Once
)

func loadWebSettingsPermissionsSnapshot() bridgepkg.PermissionsSnapshot {
	return loadPermissionsSnapshot()
}

func startPermissionPolling(ctx context.Context, applyNativeState bool) {
	go func() {
		lastSnapshot := bridgepkg.PermissionsSnapshot{}
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			snapshot := loadPermissionsSnapshot()
			if applyNativeState {
				applyNativePermissionSnapshot(snapshot)
			}
			if snapshot != lastSnapshot {
				publishPermissionsChanged(snapshot)
				lastSnapshot = snapshot
			}
			if snapshot.Accessibility && snapshot.InputMonitoring {
				return
			}

			timer := time.NewTimer(permissionPollingInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}()
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
	return devices, nil
}

func loadWebSettingsModelSnapshot(modelSize string) (bridgepkg.ModelSnapshot, error) {
	modelPath, err := defaultModelPath(modelSize)
	if err != nil {
		return bridgepkg.ModelSnapshot{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelUnavailable,
			"Failed to resolve model state",
			false,
			map[string]any{"size": modelSize},
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

func loadActiveWebSettingsModelSnapshot() (bridgepkg.ModelSnapshot, error) {
	activeModelSize := prefsActiveModel
	if activeModelSize == "" {
		cfgPath, err := webSettingsDefaultConfigPath()
		if err != nil {
			return bridgepkg.ModelSnapshot{}, bridgepkg.WrapContractError(
				bridgepkg.ErrorCodeModelUnavailable,
				"Failed to resolve active model config path",
				false,
				nil,
				err,
			)
		}
		cfg, err := webSettingsLoadConfig(cfgPath)
		if err != nil {
			return bridgepkg.ModelSnapshot{}, bridgepkg.WrapContractError(
				bridgepkg.ErrorCodeModelUnavailable,
				"Failed to load active model config",
				false,
				nil,
				err,
			)
		}
		activeModelSize = cfg.ModelSize
	}
	return loadWebSettingsModelSnapshot(activeModelSize)
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

func loadWebSettingsLogTailSnapshot() (bridgepkg.LogTailSnapshot, error) {
	path, err := webSettingsLogPath()
	if err != nil {
		return bridgepkg.LogTailSnapshot{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeLogsUnavailable,
			"Failed to resolve log path",
			false,
			nil,
			err,
		)
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return bridgepkg.LogTailSnapshot{}, nil
		}
		return bridgepkg.LogTailSnapshot{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeLogsUnavailable,
			"Failed to inspect log file",
			false,
			nil,
			err,
		)
	}

	text, truncated, err := loggingpkg.ReadLogTail(path, 500)
	if err != nil {
		if os.IsNotExist(err) {
			return bridgepkg.LogTailSnapshot{}, nil
		}
		return bridgepkg.LogTailSnapshot{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeLogsUnavailable,
			"Failed to read log tail",
			false,
			nil,
			err,
		)
	}

	return bridgepkg.LogTailSnapshot{
		Text:      text,
		Truncated: truncated,
		ByteSize:  info.Size(),
		UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

func loadWebSettingsLogFullText() (string, error) {
	path, err := webSettingsLogPath()
	if err != nil {
		return "", bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeLogsUnavailable,
			"Failed to resolve log path",
			false,
			nil,
			err,
		)
	}

	full, err := loggingpkg.ReadFullLog(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeLogsUnavailable,
			"Failed to read full logs",
			false,
			nil,
			err,
		)
	}
	return full, nil
}

func copyWebSettingsLogFullText(ctx context.Context) (string, error) {
	full, err := loadWebSettingsLogFullText()
	if err != nil {
		return "", err
	}
	if err := copyTextToClipboard(full); err != nil {
		return "", bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeLogsUnavailable,
			"Failed to copy full logs",
			true,
			nil,
			err,
		)
	}
	return full, nil
}

func notifyWebSettingsLogsUpdated() {
	webSettingsLogUpdateMu.Lock()
	if webSettingsLogRefreshRunning {
		webSettingsLogUpdateMu.Unlock()
		return
	}
	webSettingsLogRefreshRunning = true
	webSettingsLogUpdateMu.Unlock()
	defer func() {
		webSettingsLogUpdateMu.Lock()
		webSettingsLogRefreshRunning = false
		webSettingsLogUpdateMu.Unlock()
	}()

	snapshot, err := loadWebSettingsLogTailSnapshot()
	if err != nil {
		currentSettingsLogger().Warn("failed to refresh logs", "operation", "notifyWebSettingsLogsUpdated", "error", err)
	}
	publishLogsUpdated(snapshot)
}

func ensureWebSettingsLogObserver() {
	webSettingsLogObserverOnce.Do(func() {
		registerLogWriteObserver(func(path string) {
			if preferencesOpenLoad() == 0 {
				return
			}
			webSettingsLogUpdateMu.Lock()
			refreshRunning := webSettingsLogRefreshRunning
			webSettingsLogUpdateMu.Unlock()
			if refreshRunning {
				return
			}
			expectedPath, err := webSettingsLogPath()
			if err != nil || expectedPath == "" || path != expectedPath {
				return
			}
			scheduleWebSettingsLogsUpdated()
		})
	})
}

func scheduleWebSettingsLogsUpdated() {
	webSettingsLogUpdateMu.Lock()
	defer webSettingsLogUpdateMu.Unlock()
	if webSettingsLogUpdateTimer == nil {
		webSettingsLogUpdateTimer = time.AfterFunc(150*time.Millisecond, func() {
			notifyWebSettingsLogsUpdated()
			webSettingsLogUpdateMu.Lock()
			webSettingsLogUpdateTimer = nil
			webSettingsLogUpdateMu.Unlock()
		})
		return
	}
	webSettingsLogUpdateTimer.Reset(150 * time.Millisecond)
}

func ensureWebSettingsInputLevelPublisher() {
	webSettingsInputLevelOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(150 * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				if preferencesOpenLoad() == 0 {
					continue
				}
				recorder := currentSettingsRecorder()
				if recorder == nil {
					continue
				}
				samples := recorder.Snapshot()
				var sumSq float64
				for _, s := range samples {
					sumSq += float64(s) * float64(s)
				}
				level := 0.0
				if len(samples) > 0 {
					level = math.Sqrt(sumSq / float64(len(samples))) * 12
				}
				if level > 1 {
					level = 1
				}
				quality := "poor"
				if level >= 0.35 {
					quality = "good"
				} else if level >= 0.12 {
					quality = "acceptable"
				}
				publishInputLevelChanged(bridgepkg.InputLevelSnapshot{Level: level, Quality: quality})
			}
		}()
	})
}

func downloadWebSettingsModel(ctx context.Context, modelSize string) error {
	if !beginWebSettingsModelDownload(modelSize) {
		return bridgepkg.NewContractError(
			bridgepkg.ErrorCodeModelDownloadFailed,
			"Another model download is already running",
			true,
			map[string]any{"size": modelSize},
		)
	}

	modelPath, err := defaultModelPath(modelSize)
	if err != nil {
		finishWebSettingsModelDownload(modelSize)
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDownloadFailed,
			"Failed to resolve model download path",
			false,
			map[string]any{"size": modelSize},
			err,
		)
	}

	if ctx == nil {
		ctx = currentPreferencesContext()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func(downloadCtx context.Context, size string, path string) {
		defer finishWebSettingsModelDownload(size)
		if err := runWebSettingsModelDownload(downloadCtx, size, path); err != nil {
			message, retriable := describeWebSettingsModelDownloadFailure(err)
			currentSettingsLogger().Warn("model download failed", "operation", "downloadWebSettingsModel", "size", size, "error", err)
			publishModelDownloadFailed(size, message, retriable)
			return
		}
		publishModelDownloadCompleted(size)
	}(ctx, modelSize, modelPath)
	return nil
}

func runWebSettingsModelDownload(ctx context.Context, modelSize string, modelPath string) error {
	lastPublishedAt := time.Time{}
	lastPublishedPercent := -1
	if err := downloadModelWithProgress(ctx, modelPath, modelSize, func(progress float64, downloaded, total int64) {
		now := time.Now()
		percent := int(math.Round(progress * 100))
		shouldPublish := downloaded == 0 || downloaded == total || lastPublishedPercent == -1
		if !shouldPublish {
			shouldPublish = percent != lastPublishedPercent && percent%5 == 0
		}
		if !shouldPublish && now.Sub(lastPublishedAt) >= 200*time.Millisecond {
			shouldPublish = true
		}
		if !shouldPublish {
			return
		}
		lastPublishedAt = now
		lastPublishedPercent = percent
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
	if prefsActiveModel == modelSize {
		snapshot, err := loadActiveWebSettingsModelSnapshot()
		if err != nil {
			return err
		}
		publishModelChanged(snapshot)
	}
	return nil
}

func beginWebSettingsModelDownload(size string) bool {
	webSettingsDownloadMu.Lock()
	defer webSettingsDownloadMu.Unlock()
	if webSettingsActiveDownload != "" {
		return false
	}
	webSettingsActiveDownload = size
	return true
}

func finishWebSettingsModelDownload(size string) {
	webSettingsDownloadMu.Lock()
	if webSettingsActiveDownload == size {
		webSettingsActiveDownload = ""
	}
	webSettingsDownloadMu.Unlock()
}

func describeWebSettingsModelDownloadFailure(err error) (string, bool) {
	if contractErr, ok := bridgepkg.AsContractError(err); ok {
		return contractErr.Message, contractErr.Retriable
	}
	if err != nil && err.Error() != "" {
		return err.Error(), false
	}
	return "Failed to download model", false
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
	devices, err := audiopkg.ListInputDeviceSnapshots()
	if err != nil {
		l.Error("failed to list devices", "operation", "populateMicList", "error", err)
		return
	}
	var inputNames []string
	defaultIdx := 0
	for _, device := range devices {
		if device.Name == selectedDevice {
			defaultIdx = len(inputNames)
		}
		inputNames = append(inputNames, device.Name)
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
	ensureWebSettingsLogObserver()
	ensureWebSettingsInputLevelPublisher()

	if !preferencesOpenCompareAndSwap(0, 1) {
		currentSettingsLogger().Info("preferences already open, reactivating existing window",
			"operation", "OpenPreferences")
		if shouldUseWebSettings() {
			currentSettingsLogger().Info("focusing existing web settings window", "operation", "OpenPreferences")
			FocusWebSettingsWindow()
			return
		}
		currentSettingsLogger().Info("re-showing native settings window", "operation", "OpenPreferences")
		C.showSettingsWindow(0)
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

	if shouldUseWebSettings() {
		prefsActiveModel = cfg.ModelSize
		prefsCtx, prefsCancel := context.WithCancel(context.Background())
		setPreferencesContext(prefsCtx, prefsCancel)
		bridgeService := buildSettingsBridgeService(cfg)
		currentSettingsLogger().Info("showing web settings window", "operation", "OpenPreferences")
		if err := ShowWebSettingsWindowWithBridge(prefsCtx, bridgeService); err != nil {
			cancelPreferencesContext()
			currentSettingsLogger().Error("failed to show web settings window",
				"operation", "OpenPreferences", "error", err)
			postNotification("JoiceTyper Preferences", "Failed to open web preferences. Set JOICETYPER_USE_NATIVE_PREFERENCES=1 only if you need the debug fallback.")
			preferencesOpenStore(0)
			return
		} else {
			startPermissionPolling(prefsCtx, false)
			notifyWebSettingsLogsUpdated()
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

	startPermissionPolling(currentPreferencesContext(), true)

	// Block until user closes the window
	currentSettingsLogger().Info("waiting for window close", "operation", "openPreferencesWait")
	C.runSetupEventLoop()

	currentSettingsLogger().Info("window closed", "operation", "openPreferencesWait")

	// Cancel downloads from this preferences session
	cancelPreferencesContext()

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
	notifyWebSettingsLogsUpdated()
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
