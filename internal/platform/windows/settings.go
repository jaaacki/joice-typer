//go:build windows

package windows

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	audiopkg "voicetype/internal/core/audio"
	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	loggingpkg "voicetype/internal/core/logging"
	transcriptionpkg "voicetype/internal/core/transcription"

	"golang.org/x/sys/windows"
)

var webSettingsEnabled = func() bool {
	if os.Getenv("JOICETYPER_USE_NATIVE_PREFERENCES") == "1" {
		return false
	}
	if value := os.Getenv("JOICETYPER_USE_WEB_SETTINGS"); value != "" {
		return value == "1"
	}
	return true
}

var (
	webSettingsDefaultConfigPath = configpkg.DefaultConfigPath
	webSettingsLoadConfig        = configpkg.LoadConfig
	webSettingsSaveConfig        = configpkg.SaveConfig
	webSettingsSignalRestart     = signalHotkeyRestartCh
	webSettingsPostError         = func(message string) {
		currentSettingsLogger().Warn("web settings bridge request failed", "operation", "webSettingsPostError", "message", message)
	}
	webSettingsNativeTransportInfo = func(operation, message string) {
		currentSettingsLogger().Info(message, "operation", operation)
	}
	webSettingsNativeTransportWarning = func(operation, message string) {
		currentSettingsLogger().Warn(message, "operation", operation)
	}
	webSettingsLoadPermissions      = loadWebSettingsPermissionsSnapshot
	webSettingsListInputDevices     = listWebSettingsInputDevices
	webSettingsRefreshDevices       = refreshWebSettingsDevices
	webSettingsDownloadModel        = downloadWebSettingsModel
	webSettingsDeleteModel          = deleteWebSettingsModel
	webSettingsUseModel             = useWebSettingsModel
	webSettingsStartHotkeyCapture   = startWebSettingsHotkeyCapture
	webSettingsCancelHotkeyCapture  = cancelWebSettingsHotkeyCapture
	webSettingsConfirmHotkeyCapture = confirmWebSettingsHotkeyCapture
	webSettingsLogPath              = func() (string, error) {
		dir, err := configpkg.DefaultConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "voicetype.log"), nil
	}
	defaultModelPath                  = configpkg.DefaultModelPath
	removeFile                        = os.Remove
	listAudioDevices                  = audiopkg.ListInputDeviceSnapshots
	showWindowsPreferencesUnavailable = func(message string) {
		showWindowsMessageBox("JoiceTyper Preferences unavailable", message)
	}
	registerLogWriteObserver = loggingpkg.RegisterWriteObserver
)

var (
	webSettingsLogObserverOnce   sync.Once
	webSettingsLogUpdateMu       sync.Mutex
	webSettingsLogUpdateTimer    *time.Timer
	webSettingsLogRefreshRunning bool
	webSettingsDownloadMu        sync.Mutex
	webSettingsActiveDownload    string
	webSettingsInputLevelOnce    sync.Once
)

func shouldUseWebSettings() bool {
	return webSettingsEnabled()
}

func IsFirstRun() bool {
	path, err := configpkg.DefaultConfigPath()
	if err != nil {
		return true
	}
	_, err = os.Stat(path)
	return os.IsNotExist(err)
}

func RunSetupWizard(ctx context.Context, logger *slog.Logger) (string, error) {
	if err := openPreferences(); err != nil {
		return "", err
	}
	prefsCtx := currentPreferencesContext()
	if prefsCtx == nil {
		return "", fmt.Errorf("preferences context unavailable after opening setup")
	}
	select {
	case <-prefsCtx.Done():
		if logger != nil {
			logger.Info("first-run preferences closed", "operation", "RunSetupWizard")
		}
	case <-ctx.Done():
		cancelPreferencesContext()
		return "", ctx.Err()
	}
	return "", nil
}

func OpenPreferences() {
	if err := openPreferences(); err != nil {
		currentSettingsLogger().Error("failed to open preferences", "operation", "OpenPreferences", "error", err)
		showWindowsPreferencesUnavailable(decorateWebView2UnavailableMessage(err))
	}
}

func openPreferences() error {
	ensureWebSettingsLogObserver()
	ensureWebSettingsInputLevelPublisher()
	if !preferencesOpenCompareAndSwap(0, 1) {
		currentSettingsLogger().Info("preferences already open, reactivating existing window", "operation", "OpenPreferences")
		FocusWebSettingsWindow()
		return nil
	}
	currentSettingsLogger().Info("preferences opened", "operation", "OpenPreferences")

	cfgPath, err := webSettingsDefaultConfigPath()
	if err != nil {
		currentSettingsLogger().Error("failed to resolve config path", "operation", "OpenPreferences", "error", err)
		preferencesOpenStore(0)
		return fmt.Errorf("failed to resolve config path: %w", err)
	}
	cfg, err := webSettingsLoadConfig(cfgPath)
	if err != nil {
		currentSettingsLogger().Error("failed to load config", "operation", "OpenPreferences", "error", err)
		preferencesOpenStore(0)
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !shouldUseWebSettings() {
		currentSettingsLogger().Warn("web settings disabled on Windows", "operation", "OpenPreferences")
		preferencesOpenStore(0)
		return fmt.Errorf("web preferences are disabled on Windows; unset JOICETYPER_USE_NATIVE_PREFERENCES or enable WebView2-backed settings")
	}

	cfg = migrateWindowsInputDeviceConfig(cfg)

	prefsCtx, prefsCancel := context.WithCancel(context.Background())
	setPreferencesContext(prefsCtx, prefsCancel)

	currentSettingsLogger().Info("showing web settings window", "operation", "OpenPreferences")
	if err := ShowWebSettingsWindowWithBridge(prefsCtx, buildSettingsBridgeService(cfg)); err != nil {
		cancelPreferencesContext()
		preferencesOpenStore(0)
		return fmt.Errorf("failed to start the Windows preferences host: %w", err)
	}

	publishInputLevelChanged(bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"})
	notifyWebSettingsLogsUpdated()
	return nil
}

func signalHotkeyRestart() {
	currentSettingsLogger().Info("signalling hotkey restart", "operation", "signalHotkeyRestart")
	webSettingsSignalRestart()
	h := ActiveHotkey()
	if h != nil {
		if err := h.Stop(); err != nil {
			currentSettingsLogger().Warn("failed to stop hotkey for restart", "operation", "signalHotkeyRestart", "error", err)
		}
	}
}

const (
	mbOK        = 0x00000000
	mbIconError = 0x00000010
)

var procMessageBoxW = user32.NewProc("MessageBoxW")

func showWindowsMessageBox(title string, message string) {
	titlePtr, titleErr := windows.UTF16PtrFromString(title)
	messagePtr, messageErr := windows.UTF16PtrFromString(message)
	if titleErr != nil || messageErr != nil {
		currentSettingsLogger().Warn("failed to prepare native message box", "operation", "showWindowsMessageBox", "title_error", titleErr, "message_error", messageErr)
		return
	}
	if ret, _, callErr := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), mbOK|mbIconError); ret == 0 && callErr != nil && callErr.Error() != "The operation completed successfully." {
		currentSettingsLogger().Warn("failed to show native message box", "operation", "showWindowsMessageBox", "error", callErr)
	}
}

func decorateWebView2UnavailableMessage(err error) string {
	if err == nil {
		return webView2RuntimeInstallHelpMessage
	}
	message := err.Error()
	if !strings.Contains(message, webView2RuntimeError) {
		return message
	}
	if strings.Contains(message, webView2RuntimeInstallHelpMessage) {
		return message
	}
	return message + "\n\n" + webView2RuntimeInstallHelpMessage
}

func loadWebSettingsPermissionsSnapshot() bridgepkg.PermissionsSnapshot {
	return bridgepkg.PermissionsSnapshot{Accessibility: true, InputMonitoring: true}
}

func loadWebSettingsMachineInfo() bridgepkg.MachineInfoSnapshot {
	snapshot := bridgepkg.MachineInfoSnapshot{
		Platform:          "windows",
		WhisperSystemInfo: transcriptionpkg.WhisperSystemInfo(),
	}
	if version, err := detectInstalledWebView2Version(); err == nil {
		snapshot.WebViewRuntimeVersion = version
	}
	if cpuModel, err := windowsCPUModel(); err == nil {
		snapshot.CPUModel = cpuModel
	}
	if machineModel, err := windowsMachineModel(); err == nil {
		snapshot.MachineModel = machineModel
	}
	if modelPath, err := defaultModelPath("small"); err == nil {
		_ = modelPath
		backends := transcriptionpkg.WindowsBackendInventory()
		snapshot.InferenceBackends = backends
		for _, backend := range backends {
			label := backend.Description
			if label == "" {
				label = backend.Name
			}
			if label != "" {
				snapshot.Graphics = append(snapshot.Graphics, label)
			}
			if snapshot.IntegratedGPU == "" && backend.Kind == "igpu" {
				snapshot.IntegratedGPU = label
			}
		}
	}
	if snapshot.Chip == "" {
		snapshot.Chip = snapshot.CPUModel
	}
	return snapshot
}

func windowsCPUModel() (string, error) {
	out, err := exec.Command("cmd", "/C", "wmic cpu get Name /value").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Name=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Name=")), nil
		}
	}
	return "", fmt.Errorf("cpu model not found")
}

func windowsMachineModel() (string, error) {
	out, err := exec.Command("cmd", "/C", "wmic computersystem get Manufacturer,Model /value").Output()
	if err != nil {
		return "", err
	}
	manufacturer := ""
	model := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Manufacturer="):
			manufacturer = strings.TrimSpace(strings.TrimPrefix(line, "Manufacturer="))
		case strings.HasPrefix(line, "Model="):
			model = strings.TrimSpace(strings.TrimPrefix(line, "Model="))
		}
	}
	value := strings.TrimSpace(strings.TrimSpace(manufacturer + " " + model))
	if value == "" {
		return "", fmt.Errorf("machine model not found")
	}
	return value, nil
}

func migrateWindowsInputDeviceConfig(cfg configpkg.Config) configpkg.Config {
	devices, err := listWebSettingsInputDevices()
	if err != nil {
		return cfg
	}
	defaultDevice := bridgepkg.DeviceSnapshot{}
	for _, device := range devices {
		if device.IsDefault {
			defaultDevice = device
			break
		}
	}
	if cfg.InputDevice == "" {
		if defaultDevice.ID != "" {
			cfg.InputDevice = defaultDevice.ID
			cfg.InputDeviceName = defaultDevice.Name
		}
		return cfg
	}
	for _, device := range devices {
		if cfg.InputDevice == device.ID {
			if cfg.InputDeviceName == "" {
				cfg.InputDeviceName = device.Name
			}
			return cfg
		}
	}
	for _, device := range devices {
		if cfg.InputDevice == device.Name {
			cfg.InputDevice = device.ID
			cfg.InputDeviceName = device.Name
			return cfg
		}
	}
	if defaultDevice.ID != "" {
		cfg.InputDevice = defaultDevice.ID
		cfg.InputDeviceName = defaultDevice.Name
	}
	return cfg
}

func MigrateWindowsInputDeviceConfig(cfg configpkg.Config) configpkg.Config {
	return migrateWindowsInputDeviceConfig(cfg)
}

func buildSettingsBridgeService(_ configpkg.Config) *bridgepkg.Service {
	return bridgepkg.NewService(&bridgepkg.Dependencies{
		LoadConfig: func(context.Context) (configpkg.Config, error) {
			cfgPath, err := webSettingsDefaultConfigPath()
			if err != nil {
				return configpkg.Config{}, bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigLoadFailure,
					"Failed to resolve config path",
					false,
					nil,
					err,
				)
			}
			loaded, err := webSettingsLoadConfig(cfgPath)
			if err != nil {
				return configpkg.Config{}, bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigLoadFailure,
					"Failed to load config",
					false,
					nil,
					err,
				)
			}
			loaded = migrateWindowsInputDeviceConfig(loaded)
			return loaded, nil
		},
		SaveConfig: func(_ context.Context, updated configpkg.Config) error {
			if err := updated.Validate(); err != nil {
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigInvalid,
					"Config validation failed",
					false,
					nil,
					err,
				)
			}
			updated = migrateWindowsInputDeviceConfig(updated)
			cfgPath, err := webSettingsDefaultConfigPath()
			if err != nil {
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeSaveFailure,
					"Failed to resolve config path",
					false,
					nil,
					err,
				)
			}
			if err := webSettingsSaveConfig(cfgPath, updated); err != nil {
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeSaveFailure,
					"Failed to save config",
					false,
					nil,
					err,
				)
			}
			webSettingsSignalRestart()
			return nil
		},
		LoadAppState: func(context.Context) (AppState, error) {
			return currentAppState(), nil
		},
		LoadPermissions: func(context.Context) (bridgepkg.PermissionsSnapshot, error) {
			return webSettingsLoadPermissions(), nil
		},
		LoadMachineInfo: func(context.Context) (bridgepkg.MachineInfoSnapshot, error) {
			return loadWebSettingsMachineInfo(), nil
		},
		OpenPermissionSettings: func(_ context.Context, target string) error {
			// Windows does not require the macOS-specific accessibility/input-monitoring grants.
			return bridgepkg.WrapContractError(
				bridgepkg.ErrorCodePermissionOpenFailed,
				"Windows does not require additional accessibility or input-monitoring settings",
				false,
				map[string]any{"target": target},
				nil,
			)
		},
		ListDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
			return webSettingsListInputDevices()
		},
		RefreshDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
			return webSettingsRefreshDevices()
		},
		SetAudioInputMonitor: func(_ context.Context, inputDevice string) error {
			cfgPath, err := webSettingsDefaultConfigPath()
			if err != nil {
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeSaveFailure,
					"Failed to resolve config path",
					false,
					nil,
					err,
				)
			}
			cfg, err := webSettingsLoadConfig(cfgPath)
			if err != nil {
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigLoadFailure,
					"Failed to load config",
					false,
					nil,
					err,
				)
			}
			cfg.InputDevice = inputDevice
			cfg = migrateWindowsInputDeviceConfig(cfg)
			if monitor := currentSettingsInputMonitor(); monitor != nil {
				if err := monitor.Close(); err != nil {
					return bridgepkg.WrapContractError(
						bridgepkg.ErrorCodeDevicesRefreshFailed,
						"Failed to stop previous monitored input device",
						true,
						nil,
						err,
					)
				}
				SetSettingsInputMonitor(nil)
			}
			monitor, err := audiopkg.NewInputLevelMonitor(cfg.SampleRate, cfg.InputDevice, currentSettingsLogger())
			if err != nil {
				currentSettingsLogger().Warn("failed to start monitored input device", "operation", "SetAudioInputMonitor", "device", cfg.InputDevice, "error", err)
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeDevicesRefreshFailed,
					"Failed to start monitored input device",
					true,
					nil,
					err,
				)
			}
			SetSettingsInputMonitor(monitor)
			publishInputLevelChanged(monitor.Snapshot())
			return nil
		},
		StopAudioInputMonitor: func(context.Context) error {
			monitor := currentSettingsInputMonitor()
			if monitor == nil {
				publishInputLevelChanged(bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"})
				return nil
			}
			if err := monitor.Close(); err != nil {
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeDevicesRefreshFailed,
					"Failed to stop monitored input device",
					true,
					nil,
					err,
				)
			}
			SetSettingsInputMonitor(nil)
			publishInputLevelChanged(bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"})
			return nil
		},
		LoadModel: func(context.Context) (bridgepkg.ModelSnapshot, error) {
			return loadActiveWebSettingsModelSnapshot()
		},
		DownloadModel: func(ctx context.Context, size string) error {
			return webSettingsDownloadModel(ctx, size)
		},
		DeleteModel: func(ctx context.Context, size string) error {
			return webSettingsDeleteModel(ctx, size)
		},
		UseModel: func(ctx context.Context, size string) error {
			return webSettingsUseModel(ctx, size)
		},
		LoadLogsTail: func(context.Context) (bridgepkg.LogTailSnapshot, error) {
			return loadWebSettingsLogTailSnapshot()
		},
		LoadLogsFull: func(context.Context) (string, error) {
			return loadWebSettingsLogFullText()
		},
		WriteClipboardText: func(_ context.Context, text string) error {
			return setWindowsClipboardText(text)
		},
		LoadUpdater: func(context.Context) (bridgepkg.UpdaterSnapshot, error) {
			return loadWebSettingsUpdaterSnapshot(), nil
		},
		CheckForUpdates: func(context.Context) error {
			return checkWebSettingsForUpdates()
		},
		StartHotkeyCapture: func(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {
			return webSettingsStartHotkeyCapture()
		},
		CancelHotkeyCapture: func(context.Context) error {
			return webSettingsCancelHotkeyCapture()
		},
		ConfirmHotkeyCapture: func(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {
			return webSettingsConfirmHotkeyCapture()
		},
	})
}

// prefsActiveModel tracks the in-use model for the current preferences session.
// It may differ from the saved config while the user is previewing a different
// model in the current runtime.
var prefsActiveModel string

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

func downloadWebSettingsModel(ctx context.Context, size string) error {
	if !beginWebSettingsModelDownload(size) {
		return bridgepkg.NewContractError(
			bridgepkg.ErrorCodeModelDownloadFailed,
			"Another model download is already running",
			true,
			map[string]any{"size": size},
		)
	}

	modelPath, err := defaultModelPath(size)
	if err != nil {
		finishWebSettingsModelDownload(size)
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDownloadFailed,
			"Failed to resolve model download path",
			false,
			map[string]any{"size": size},
			err,
		)
	}

	if ctx == nil {
		ctx = currentPreferencesContext()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	go func(downloadCtx context.Context, modelSize string, path string) {
		defer finishWebSettingsModelDownload(modelSize)
		if err := runWebSettingsModelDownload(downloadCtx, modelSize, path); err != nil {
			message, retriable := describeWebSettingsModelDownloadFailure(err)
			currentSettingsLogger().Warn("model download failed", "operation", "downloadWebSettingsModel", "size", modelSize, "error", err)
			publishModelDownloadFailed(modelSize, message, retriable)
			return
		}
		publishModelDownloadCompleted(modelSize)
	}(ctx, size, modelPath)
	return nil
}

func runWebSettingsModelDownload(ctx context.Context, size string, modelPath string) error {
	lastPublishedAt := time.Time{}
	lastPublishedPercent := -1
	if err := transcriptionpkg.DownloadModelWithProgress(ctx, modelPath, size, func(progress float64, downloaded, total int64) {
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
		publishModelDownloadProgress(size, progress, downloaded, total)
	}, currentSettingsLogger()); err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDownloadFailed,
			"Failed to download model",
			true,
			map[string]any{"size": size},
			err,
		)
	}

	if prefsActiveModel == size {
		snapshot, err := loadWebSettingsModelSnapshot(size)
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

func deleteWebSettingsModel(ctx context.Context, size string) error {
	_ = ctx
	if prefsActiveModel == size {
		return bridgepkg.NewContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Cannot delete the active model",
			false,
			map[string]any{"size": size},
		)
	}

	modelPath, err := defaultModelPath(size)
	if err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Failed to resolve model path",
			false,
			map[string]any{"size": size},
			err,
		)
	}
	if removeErr := removeFile(modelPath); removeErr != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Failed to delete model",
			false,
			map[string]any{"size": size},
			removeErr,
		)
	}
	if removeErr := removeFile(modelPath + ".sha256"); removeErr != nil && !os.IsNotExist(removeErr) {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelDeleteFailed,
			"Failed to delete model hash cache",
			false,
			map[string]any{"size": size},
			removeErr,
		)
	}
	return nil
}

func useWebSettingsModel(ctx context.Context, size string) error {
	_ = ctx
	modelPath, err := defaultModelPath(size)
	if err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelUseFailed,
			"Failed to resolve model path",
			false,
			map[string]any{"size": size},
			err,
		)
	}
	if _, statErr := os.Stat(modelPath); statErr != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeModelUseFailed,
			"Model is not available to use",
			false,
			map[string]any{"size": size},
			statErr,
		)
	}

	prefsActiveModel = size
	snapshot, err := loadWebSettingsModelSnapshot(size)
	if err != nil {
		return err
	}
	publishModelChanged(snapshot)
	return nil
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
	if activeModelSize != "" {
		return loadWebSettingsModelSnapshot(activeModelSize)
	}

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
	prefsActiveModel = activeModelSize
	return loadWebSettingsModelSnapshot(activeModelSize)
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
				monitor := currentSettingsInputMonitor()
				if monitor == nil {
					continue
				}
				publishInputLevelChanged(monitor.Snapshot())
			}
		}()
	})
}
