package config

import "path/filepath"

func appConfigDir(configRoot string) string {
	return filepath.Join(configRoot, "JoiceTyper")
}

func legacyConfigDir(configRoot string) string {
	return filepath.Join(configRoot, "voicetype")
}
