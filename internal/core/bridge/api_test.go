package bridge

import (
	"context"
	"testing"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

// TestBridge_StubPlatformMethodsReturnNotStubbedError documents the new
// contract: with a fully-empty FuncPlatform (the test stub), every Service
// method bubbles up errPlatformMethodNotStubbed from the underlying call.
// In production, the Platform interface guarantees every method is
// implemented — there is no nil-dependency code path to test.
func TestBridge_StubPlatformMethodsReturnNotStubbedError(t *testing.T) {
	svc := NewService(FuncPlatform{})

	if _, err := svc.Config(context.Background()); err == nil {
		t.Fatal("expected Config to surface stub error")
	}
	if err := svc.SaveConfig(context.Background(), ConfigSnapshot{}); err == nil {
		t.Fatal("expected SaveConfig to surface stub error")
	}
	if _, err := svc.Permissions(context.Background()); err == nil {
		t.Fatal("expected Permissions to surface stub error")
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
	svc := NewService(&FuncPlatform{
		LoadConfigFn: func(context.Context) (configpkg.Config, error) {
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
	svc := NewService(&FuncPlatform{
		LoadConfigFn: func(context.Context) (configpkg.Config, error) {
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
		LoadAppStateFn: func(context.Context) (apppkg.AppState, error) {
			return apppkg.StateReady, nil
		},
		LoadPermissionsFn: func(context.Context) (PermissionsSnapshot, error) {
			return PermissionsSnapshot{Accessibility: true, InputMonitoring: false}, nil
		},
		LoadModelFn: func(context.Context) (ModelSnapshot, error) {
			return ModelSnapshot{Size: "medium", Path: "/tmp/ggml-medium.bin", Ready: true}, nil
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
	if !bootstrap.Permissions.Accessibility || bootstrap.Permissions.InputMonitoring {
		t.Fatalf("Bootstrap.Permissions = %#v, want accessibility=true inputMonitoring=false", bootstrap.Permissions)
	}
	if bootstrap.Model.Size != "medium" || !bootstrap.Model.Ready {
		t.Fatalf("Bootstrap.Model = %#v, want size=medium ready=true", bootstrap.Model)
	}
	if len(bootstrap.Options.Models) == 0 {
		t.Fatal("expected bootstrap options to include models")
	}
}

func TestBridge_LogsGetReturnsTailPayload(t *testing.T) {
	svc := NewService(&FuncPlatform{
		LoadLogsTailFn: func(context.Context) (LogTailSnapshot, error) {
			return LogTailSnapshot{
				Text:      "line 499\nline 500\n",
				Truncated: true,
				ByteSize:  1234,
				UpdatedAt: "2026-04-20T03:04:05Z",
			}, nil
		},
	})

	snapshot, err := svc.LogsGet(context.Background())
	if err != nil {
		t.Fatalf("LogsGet returned error: %v", err)
	}
	if !snapshot.Truncated {
		t.Fatal("expected truncated tail payload")
	}
	if snapshot.ByteSize != 1234 {
		t.Fatalf("ByteSize = %d, want 1234", snapshot.ByteSize)
	}
	if snapshot.UpdatedAt != "2026-04-20T03:04:05Z" {
		t.Fatalf("UpdatedAt = %q, want 2026-04-20T03:04:05Z", snapshot.UpdatedAt)
	}
	if snapshot.Text != "line 499\nline 500\n" {
		t.Fatalf("Text = %q, want tail text", snapshot.Text)
	}
}

func TestBridge_LogsCopyAllReturnsFullText(t *testing.T) {
	svc := NewService(&FuncPlatform{
		LoadLogsFullFn: func(context.Context) (string, error) {
			return "line 001\nline 002\nline 003\n", nil
		},
		WriteClipboardTextFn: func(context.Context, string) error { return nil },
	})

	text, err := svc.LogsCopyAll(context.Background())
	if err != nil {
		t.Fatalf("LogsCopyAll returned error: %v", err)
	}
	if text != "line 001\nline 002\nline 003\n" {
		t.Fatalf("text = %q, want full file text", text)
	}
}

func TestBridge_LogsCopyAllCopiesTextWhenNativeClipboardIsAvailable(t *testing.T) {
	var copied string
	svc := NewService(&FuncPlatform{
		LoadLogsFullFn: func(context.Context) (string, error) {
			return "line 001\nline 002\nline 003\n", nil
		},
		WriteClipboardTextFn: func(_ context.Context, text string) error {
			copied = text
			return nil
		},
	})

	text, err := svc.LogsCopyAll(context.Background())
	if err != nil {
		t.Fatalf("LogsCopyAll returned error: %v", err)
	}
	if copied != text {
		t.Fatalf("copied = %q, want %q", copied, text)
	}
}

func TestBridge_LogsCopyTailCopiesVisibleTextWhenNativeClipboardIsAvailable(t *testing.T) {
	var copied string
	svc := NewService(&FuncPlatform{
		LoadLogsTailFn: func(context.Context) (LogTailSnapshot, error) {
			return LogTailSnapshot{
				Text:      "tail 499\ntail 500\n",
				Truncated: true,
				ByteSize:  1234,
				UpdatedAt: "2026-04-20T03:04:05Z",
			}, nil
		},
		WriteClipboardTextFn: func(_ context.Context, text string) error {
			copied = text
			return nil
		},
	})

	text, err := svc.LogsCopyTail(context.Background())
	if err != nil {
		t.Fatalf("LogsCopyTail returned error: %v", err)
	}
	if text != "tail 499\ntail 500\n" {
		t.Fatalf("text = %q, want visible tail text", text)
	}
	if copied != text {
		t.Fatalf("copied = %q, want %q", copied, text)
	}
}

func TestBridge_UpdaterReturnsSnapshot(t *testing.T) {
	svc := NewService(&FuncPlatform{
		LoadUpdaterFn: func(context.Context) (UpdaterSnapshot, error) {
			return UpdaterSnapshot{
				Enabled:             true,
				SupportsManualCheck: true,
				FeedURL:             "https://example.com/appcast.xml",
				Channel:             "stable",
			}, nil
		},
	})

	snapshot, err := svc.Updater(context.Background())
	if err != nil {
		t.Fatalf("Updater returned error: %v", err)
	}
	if !snapshot.Enabled {
		t.Fatal("expected updater to be enabled")
	}
	if !snapshot.SupportsManualCheck {
		t.Fatal("expected updater manual check support to be true")
	}
	if snapshot.FeedURL != "https://example.com/appcast.xml" {
		t.Fatalf("FeedURL = %q, want appcast URL", snapshot.FeedURL)
	}
	if snapshot.Channel != "stable" {
		t.Fatalf("Channel = %q, want stable", snapshot.Channel)
	}
}

func TestBridge_CheckForUpdatesUsesDependency(t *testing.T) {
	called := false
	svc := NewService(&FuncPlatform{
		CheckForUpdatesFn: func(context.Context) error {
			called = true
			return nil
		},
	})

	if err := svc.CheckForUpdates(context.Background()); err != nil {
		t.Fatalf("CheckForUpdates returned error: %v", err)
	}
	if !called {
		t.Fatal("expected CheckForUpdates dependency to be called")
	}
}
