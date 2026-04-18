package bridge

import (
	"context"
	"testing"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

func TestBridge_NewServiceExposesConfigMethods(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("expected bridge service")
	}
	if _, err := svc.Config(context.Background()); err != nil {
		t.Fatalf("Config returned error: %v", err)
	}
}

func TestBridge_ConfigSnapshotTypeIsStable(t *testing.T) {
	snapshot := ConfigSnapshot{}
	_ = snapshot.TriggerKey
	_ = snapshot.ModelSize
	_ = snapshot.Language
	_ = snapshot.SampleRate
	_ = snapshot.SoundFeedback
	_ = snapshot.InputDevice
	_ = snapshot.DecodeMode
	_ = snapshot.PunctuationMode
	_ = snapshot.Vocabulary
}

func TestBridge_ConfigReflectsDependencySnapshot(t *testing.T) {
	svc := NewService(&Dependencies{
		LoadConfig: func(context.Context) (configpkg.Config, error) {
			return configpkg.Config{
				TriggerKey:      []string{"fn", "shift"},
				ModelSize:       "small",
				Language:        "en",
				SampleRate:      16000,
				SoundFeedback:   true,
				InputDevice:     "Built-in Microphone",
				DecodeMode:      "beam",
				PunctuationMode: "conservative",
				Vocabulary:      "joicetyper, whisper",
			}, nil
		},
	})

	snapshot, err := svc.Config(context.Background())
	if err != nil {
		t.Fatalf("Config returned error: %v", err)
	}
	if snapshot.ModelSize != "small" {
		t.Fatalf("ModelSize = %q, want small", snapshot.ModelSize)
	}
	if snapshot.TriggerKey[0] != "fn" || snapshot.TriggerKey[1] != "shift" {
		t.Fatalf("TriggerKey = %v, want [fn shift]", snapshot.TriggerKey)
	}
	if snapshot.DecodeMode != "beam" {
		t.Fatalf("DecodeMode = %q, want beam", snapshot.DecodeMode)
	}
}

func TestBridge_BootstrapIncludesConfigAndAppState(t *testing.T) {
	svc := NewService(&Dependencies{
		LoadConfig: func(context.Context) (configpkg.Config, error) {
			return configpkg.Config{
				TriggerKey:      []string{"fn", "shift"},
				ModelSize:       "medium",
				Language:        "en",
				SampleRate:      16000,
				SoundFeedback:   true,
				InputDevice:     "USB Headset",
				DecodeMode:      "beam",
				PunctuationMode: "opinionated",
				Vocabulary:      "joice",
			}, nil
		},
		LoadAppState: func(context.Context) (apppkg.AppState, error) {
			return apppkg.StateReady, nil
		},
	})

	bootstrap, err := svc.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	if bootstrap.Config.ModelSize != "medium" {
		t.Fatalf("Bootstrap.Config.ModelSize = %q, want medium", bootstrap.Config.ModelSize)
	}
	if bootstrap.AppState.State != "ready" {
		t.Fatalf("Bootstrap.AppState.State = %q, want ready", bootstrap.AppState.State)
	}
}
