package config

import (
	"path/filepath"
	"testing"
)

func TestAppConfigDir_UsesProvidedConfigRoot(t *testing.T) {
	root := filepath.Join("base", "config")
	if got, want := appConfigDir(root), filepath.Join(root, "JoiceTyper"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLegacyConfigDir_UsesLegacyAppNameUnderConfigRoot(t *testing.T) {
	root := filepath.Join("base", "config")
	if got, want := legacyConfigDir(root), filepath.Join(root, "voicetype"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
