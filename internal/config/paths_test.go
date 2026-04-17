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

func TestLegacyConfigDir_NonDarwinUsesLegacyAppNameUnderConfigRoot(t *testing.T) {
	root := filepath.Join("base", "config")
	if got, want := legacyConfigDir(root, filepath.Join("base", "home"), "linux"), filepath.Join(root, "voicetype"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLegacyConfigDir_DarwinUsesLegacyDotConfigHome(t *testing.T) {
	home := filepath.Join("Users", "alice")
	root := filepath.Join(home, "Library", "Application Support")
	if got, want := legacyConfigDir(root, home, "darwin"), filepath.Join(home, ".config", "voicetype"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
