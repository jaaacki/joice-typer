//go:build windows

package windows

import (
	"context"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
)

func buildSettingsBridgeService(_ configpkg.Config, deps *bridgepkg.Dependencies) *bridgepkg.Service {
	return bridgepkg.NewService(deps)
}

func ShowWebSettingsWindow() error {
	return nil
}

func FocusWebSettingsWindow() {}

func ShowWebSettingsWindowWithBridge(context.Context, *bridgepkg.Service) error {
	return nil
}
