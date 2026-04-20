//go:build windows

package windows

import (
	"context"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	loggingpkg "voicetype/internal/core/logging"
	transcriptionpkg "voicetype/internal/core/transcription"
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
	defaultModelPath = configpkg.DefaultModelPath
	removeFile       = os.Remove
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

func RunSetupWizard(context.Context, *slog.Logger) (string, error) {
	return "", nil
}

func OpenPreferences() {
	if !preferencesOpenCompareAndSwap(0, 1) {
		currentSettingsLogger().Info("preferences already open, reactivating existing window", "operation", "OpenPreferences")
		FocusWebSettingsWindow()
		return
	}
	currentSettingsLogger().Info("preferences opened", "operation", "OpenPreferences")

	cfgPath, err := webSettingsDefaultConfigPath()
	if err != nil {
		currentSettingsLogger().Error("failed to resolve config path", "operation", "OpenPreferences", "error", err)
		preferencesOpenStore(0)
		return
	}
	cfg, err := webSettingsLoadConfig(cfgPath)
	if err != nil {
		currentSettingsLogger().Error("failed to load config", "operation", "OpenPreferences", "error", err)
		preferencesOpenStore(0)
		return
	}

	if !shouldUseWebSettings() {
		currentSettingsLogger().Info("web settings disabled, no Windows native fallback is implemented", "operation", "OpenPreferences")
		preferencesOpenStore(0)
		return
	}

	prefsCtx, prefsCancel := context.WithCancel(context.Background())
	setPreferencesContext(prefsCtx, prefsCancel)

	currentSettingsLogger().Info("showing web settings window", "operation", "OpenPreferences")
	if err := ShowWebSettingsWindowWithBridge(prefsCtx, buildSettingsBridgeService(cfg)); err != nil {
		cancelPreferencesContext()
		currentSettingsLogger().Error("failed to show web settings window", "operation", "OpenPreferences", "error", err)
		preferencesOpenStore(0)
		return
	}

	notifyWebSettingsLogsUpdated()
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

func loadWebSettingsPermissionsSnapshot() bridgepkg.PermissionsSnapshot {
	return bridgepkg.PermissionsSnapshot{Accessibility: true, InputMonitoring: true}
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
		OpenPermissionSettings: func(context.Context, string) error {
			// Windows does not require the macOS-specific accessibility/input-monitoring grants.
			return nil
		},
		ListDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
			return webSettingsListInputDevices()
		},
		RefreshDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
			return webSettingsRefreshDevices()
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
	return []bridgepkg.DeviceSnapshot{
		{Name: "System default", IsDefault: true},
	}, nil
}

func refreshWebSettingsDevices() ([]bridgepkg.DeviceSnapshot, error) {
	return listWebSettingsInputDevices()
}

func downloadWebSettingsModel(ctx context.Context, size string) error {
	modelPath, err := defaultModelPath(size)
	if err != nil {
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
	snapshot, err := loadWebSettingsLogTailSnapshot()
	if err != nil {
		currentSettingsLogger().Warn("failed to refresh logs", "operation", "notifyWebSettingsLogsUpdated", "error", err)
	}
	publishLogsUpdated(snapshot)
}
