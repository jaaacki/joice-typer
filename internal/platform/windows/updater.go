//go:build windows

package windows

import bridgepkg "voicetype/internal/core/bridge"

func loadWebSettingsUpdaterSnapshot() bridgepkg.UpdaterSnapshot {
	return bridgepkg.UpdaterSnapshot{
		Enabled:             false,
		SupportsManualCheck: false,
		FeedURL:             "",
		Channel:             "stable",
	}
}

func checkWebSettingsForUpdates() error {
	return bridgepkg.NewContractError(
		bridgepkg.ErrorCodeUpdaterUnavailable,
		"Self-update is not configured on Windows yet",
		false,
		nil,
	)
}
