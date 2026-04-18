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
	if _, err := svc.Config(context.Background()); err == nil {
		t.Fatal("expected Config to fail when bridge dependencies are missing")
	} else if contractErr, ok := AsContractError(err); !ok || contractErr.Code != ErrorCodeInternal {
		t.Fatalf("Config error = %#v, want contract code %q", err, ErrorCodeInternal)
	}
}

func TestBridge_ServiceMethodsFailWhenDependenciesMissing(t *testing.T) {
	svc := NewService(nil)

	testCases := []struct {
		name string
		run  func() error
	}{
		{
			name: "SaveConfig",
			run: func() error {
				return svc.SaveConfig(context.Background(), ConfigSnapshot{})
			},
		},
		{
			name: "Permissions",
			run: func() error {
				_, err := svc.Permissions(context.Background())
				return err
			},
		},
		{
			name: "Devices",
			run: func() error {
				_, err := svc.Devices(context.Background())
				return err
			},
		},
		{
			name: "RefreshDevices",
			run: func() error {
				_, err := svc.RefreshDevices(context.Background())
				return err
			},
		},
		{
			name: "Model",
			run: func() error {
				_, err := svc.Model(context.Background())
				return err
			},
		},
		{
			name: "DownloadModel",
			run: func() error {
				return svc.DownloadModel(context.Background(), "small")
			},
		},
		{
			name: "DeleteModel",
			run: func() error {
				return svc.DeleteModel(context.Background(), "small")
			},
		},
		{
			name: "UseModel",
			run: func() error {
				return svc.UseModel(context.Background(), "small")
			},
		},
		{
			name: "StartHotkeyCapture",
			run: func() error {
				_, err := svc.StartHotkeyCapture(context.Background())
				return err
			},
		},
		{
			name: "CancelHotkeyCapture",
			run: func() error {
				return svc.CancelHotkeyCapture(context.Background())
			},
		},
		{
			name: "ConfirmHotkeyCapture",
			run: func() error {
				_, err := svc.ConfirmHotkeyCapture(context.Background())
				return err
			},
		},
		{
			name: "AppState",
			run: func() error {
				_, err := svc.AppState(context.Background())
				return err
			},
		},
		{
			name: "Bootstrap",
			run: func() error {
				_, err := svc.Bootstrap(context.Background())
				return err
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatalf("expected %s to fail when dependency is missing", tc.name)
			}
			contractErr, ok := AsContractError(err)
			if !ok {
				t.Fatalf("%s error = %#v, want ContractError", tc.name, err)
			}
			if contractErr.Code != ErrorCodeInternal {
				t.Fatalf("%s code = %q, want %q", tc.name, contractErr.Code, ErrorCodeInternal)
			}
			if contractErr.Details["operation"] == "" {
				t.Fatalf("%s details = %#v, want operation metadata", tc.name, contractErr.Details)
			}
		})
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
