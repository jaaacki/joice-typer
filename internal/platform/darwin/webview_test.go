//go:build darwin

package darwin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
)

func TestBuildSettingsBridgeService_UsesTrackedRuntimeState(t *testing.T) {
	originalState := currentAppState()
	storeCurrentAppState(StateNoPermission)
	defer storeCurrentAppState(originalState)

	service := buildSettingsBridgeService(configpkg.Config{
		TriggerKey:      []string{"fn", "shift"},
		ModelSize:       "small",
		Language:        "en",
		SampleRate:      16000,
		SoundFeedback:   true,
		InputDevice:     "Built-in Microphone",
		DecodeMode:      "beam",
		PunctuationMode: "conservative",
	})

	bootstrap, err := service.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	if bootstrap.AppState.State != "no_permission" {
		t.Fatalf("AppState.State = %q, want no_permission", bootstrap.AppState.State)
	}
}

func TestApplyWebSettingsConfig_SavesAndSignalsRestart(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	originalPath := webSettingsDefaultConfigPath
	originalSave := webSettingsSaveConfig
	originalSignal := webSettingsSignalRestart
	defer func() {
		webSettingsDefaultConfigPath = originalPath
		webSettingsSaveConfig = originalSave
		webSettingsSignalRestart = originalSignal
	}()

	webSettingsDefaultConfigPath = func() (string, error) { return cfgPath, nil }

	var saved configpkg.Config
	webSettingsSaveConfig = func(path string, cfg configpkg.Config) error {
		saved = cfg
		return os.WriteFile(path, []byte("saved"), 0644)
	}

	var restartCount int
	webSettingsSignalRestart = func() {
		restartCount++
	}

	err := applyWebSettingsConfig(bridgepkg.ConfigSnapshot{
		TriggerKey:      []string{"fn", "shift"},
		ModelSize:       "medium",
		Language:        "en",
		SampleRate:      16000,
		SoundFeedback:   false,
		InputDevice:     "USB Headset",
		DecodeMode:      "beam",
		PunctuationMode: "opinionated",
		Vocabulary:      "joice, typer",
	})
	if err != nil {
		t.Fatalf("applyWebSettingsConfig returned error: %v", err)
	}
	if saved.ModelSize != "medium" {
		t.Fatalf("saved.ModelSize = %q, want medium", saved.ModelSize)
	}
	if saved.InputDevice != "USB Headset" {
		t.Fatalf("saved.InputDevice = %q, want USB Headset", saved.InputDevice)
	}
	if restartCount != 1 {
		t.Fatalf("restartCount = %d, want 1", restartCount)
	}
}

func TestInjectBootstrapScript_AddsPayload(t *testing.T) {
	indexHTML := []byte("<html><head></head><body></body></html>")

	out, err := injectBootstrapScript(indexHTML, bridgepkg.BootstrapPayload{
		Config: bridgepkg.ConfigSnapshot{ModelSize: "small"},
	})
	if err != nil {
		t.Fatalf("injectBootstrapScript returned error: %v", err)
	}
	if !strings.Contains(string(out), "__JOICETYPER_BOOTSTRAP__") {
		t.Fatal("expected bootstrap script to be injected")
	}
	if !strings.Contains(string(out), `"modelSize":"small"`) {
		t.Fatal("expected bootstrap payload JSON to be present")
	}
}
