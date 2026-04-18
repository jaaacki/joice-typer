package config

import "path/filepath"

func appConfigDir(configRoot string) string {
	return filepath.Join(configRoot, "JoiceTyper")
}

func legacyConfigDir(configRoot string, homeDir string, goos string) string {
	if goos == "darwin" {
		return filepath.Join(homeDir, ".config", "voicetype")
	}
	return filepath.Join(configRoot, "voicetype")
}
