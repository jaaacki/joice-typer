//go:build windows

package windows

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	loggingpkg "voicetype/internal/core/logging"
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
	webSettingsLoadPermissions = loadWebSettingsPermissionsSnapshot
	webSettingsLogPath         = func() (string, error) {
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
	return bridgepkg.PermissionsSnapshot{}
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
		LoadModel: func(context.Context) (bridgepkg.ModelSnapshot, error) {
			return loadActiveWebSettingsModelSnapshot()
		},
		LoadLogsTail: func(context.Context) (bridgepkg.LogTailSnapshot, error) {
			return loadWebSettingsLogTailSnapshot()
		},
		LoadLogsFull: func(context.Context) (string, error) {
			return loadWebSettingsLogFullText()
		},
	})
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
	return loadWebSettingsModelSnapshot(cfg.ModelSize)
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
