package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"testing"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

func TestRouterHandleRequest_SaveConfig(t *testing.T) {
	var saved ConfigSnapshot
	service := NewService(&funcPlatform{
		SaveConfigFn: func(ctx context.Context, cfg configpkg.Config) error {
			saved = configSnapshotFromConfig(cfg)
			return nil
		},
	})
	router := NewRouter(service)

	params, err := json.Marshal(map[string]ConfigSnapshot{
		"config": {
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "medium",
			Language:        "en",
			SampleRate:      16000,
			SoundFeedback:   true,
			InputDevice:     "Built-in Microphone",
			InputDeviceName: "Built-in Microphone",
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
			Vocabulary:      "git rebase",
		},
	})
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-1",
		Method: SaveConfigMethod,
		Params: params,
	})

	if response.Kind != KindResponse {
		t.Fatalf("Kind = %q, want response", response.Kind)
	}
	if !response.OK {
		t.Fatalf("expected success response, got %#v", response.Error)
	}
	if saved.ModelSize != "medium" {
		t.Fatalf("saved.ModelSize = %q, want medium", saved.ModelSize)
	}
}

func TestRouterHandleRequest_UnsupportedMethod(t *testing.T) {
	router := NewRouter(NewService(funcPlatform{}))

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-2",
		Method: "unknown.method",
		Params: json.RawMessage(`{}`),
	})

	if response.OK {
		t.Fatal("expected unsupported method failure")
	}
	if response.Error == nil || response.Error.Code != ErrorCodeBadMethod {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodeBadMethod)
	}
}

func TestRouterHandleRequest_SaveConfigFailure(t *testing.T) {
	service := NewService(&funcPlatform{
		SaveConfigFn: func(ctx context.Context, cfg configpkg.Config) error {
			return errors.New("disk full")
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-3",
		Method: SaveConfigMethod,
		Params: json.RawMessage(`{"config":{"triggerKey":["fn","shift"],"modelSize":"small","language":"en","sampleRate":16000,"soundFeedback":true,"inputDevice":"","inputDeviceName":"","decodeMode":"beam","punctuationMode":"conservative","vocabulary":""}}`),
	})

	if response.OK {
		t.Fatal("expected save failure response")
	}
	if response.Error == nil || response.Error.Code != ErrorCodeSaveFailure {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodeSaveFailure)
	}
}

func TestRouterHandleRequest_SaveConfigRejectsMissingFields(t *testing.T) {
	router := NewRouter(NewService(&funcPlatform{
		SaveConfigFn: func(context.Context, configpkg.Config) error {
			t.Fatal("SaveConfig should not be called when required fields are missing")
			return nil
		},
	}))

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-save-missing",
		Method: SaveConfigMethod,
		Params: json.RawMessage(`{"config":{"modelSize":"small","language":"en","sampleRate":16000,"decodeMode":"beam","punctuationMode":"conservative"}}`),
	})

	if response.OK {
		t.Fatal("expected missing field save request to fail")
	}
	if response.Error == nil || response.Error.Code != ErrorCodeBadRequest {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodeBadRequest)
	}
	if got := response.Error.Details["field"]; got != "triggerKey" {
		t.Fatalf("Error.Details[field] = %#v, want triggerKey", got)
	}
}

func TestRouterHandleRequest_RejectsUnexpectedParamsForQueryMethods(t *testing.T) {
	router := NewRouter(NewService(&funcPlatform{
		LoadConfigFn: func(context.Context) (configpkg.Config, error) {
			return configpkg.Config{
				TriggerKey:      []string{"fn", "shift"},
				ModelSize:       "small",
				Language:        "en",
				SampleRate:      16000,
				SoundFeedback:   true,
				InputDevice:     "",
				DecodeMode:      "beam",
				PunctuationMode: "conservative",
			}, nil
		},
	}))

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-config-extra",
		Method: ConfigGetMethod,
		Params: json.RawMessage(`{"unexpected":true}`),
	})

	if response.OK {
		t.Fatal("expected config.get with unexpected params to fail")
	}
	if response.Error == nil || response.Error.Code != ErrorCodeBadRequest {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodeBadRequest)
	}
}

func TestRouterHandleRequest_PreservesContractErrorCode(t *testing.T) {
	service := NewService(&funcPlatform{
		OpenPermissionSettingsFn: func(ctx context.Context, target string) error {
			return NewContractError(
				ErrorCodePermissionInvalidTarget,
				"Unsupported permission settings target",
				false,
				map[string]any{"target": target},
			)
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-contract-error",
		Method: PermissionsOpenSettingsMethod,
		Params: json.RawMessage(`{"target":"banana"}`),
	})

	if response.OK {
		t.Fatal("expected failure response")
	}
	if response.Error == nil || response.Error.Code != ErrorCodePermissionInvalidTarget {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodePermissionInvalidTarget)
	}
	if got := response.Error.Details["target"]; got != "banana" {
		t.Fatalf("Error.Details[target] = %#v, want banana", got)
	}
}

func TestRouterHandleRequest_ConfigGetFailureUsesSpecificCode(t *testing.T) {
	service := NewService(&funcPlatform{
		LoadConfigFn: func(ctx context.Context) (configpkg.Config, error) {
			return configpkg.Config{}, errors.New("missing")
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-config-get",
		Method: ConfigGetMethod,
		Params: json.RawMessage(`{}`),
	})

	if response.OK {
		t.Fatal("expected config.get failure response")
	}
	if response.Error == nil || response.Error.Code != ErrorCodeConfigLoadFailure {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodeConfigLoadFailure)
	}
}

func TestRouterHandleRequest_BootstrapGet(t *testing.T) {
	service := NewService(&funcPlatform{
		LoadConfigFn: func(ctx context.Context) (configpkg.Config, error) {
			return configpkg.Config{
				ModelSize:       "small",
				Language:        "en",
				SampleRate:      16000,
				DecodeMode:      "beam",
				PunctuationMode: "conservative",
			}, nil
		},
		LoadAppStateFn: func(context.Context) (apppkg.AppState, error) {
			return apppkg.StateReady, nil
		},
		LoadPermissionsFn: func(context.Context) (PermissionsSnapshot, error) {
			return PermissionsSnapshot{Accessibility: true, InputMonitoring: false}, nil
		},
		LoadModelFn: func(context.Context) (ModelSnapshot, error) {
			return ModelSnapshot{Size: "small", Ready: true}, nil
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-4",
		Method: BootstrapMethod,
		Params: json.RawMessage(`{}`),
	})

	if !response.OK {
		t.Fatalf("expected bootstrap success, got %#v", response.Error)
	}
	payload, ok := response.Result.(BootstrapPayload)
	if !ok {
		t.Fatalf("Result = %#v, want BootstrapPayload", response.Result)
	}
	if payload.Config.ModelSize != "small" {
		t.Fatalf("payload.Config.ModelSize = %q, want small", payload.Config.ModelSize)
	}
	if payload.Model.Size != "small" || !payload.Model.Ready {
		t.Fatalf("payload.Model = %#v, want size=small ready=true", payload.Model)
	}
	if !payload.Permissions.Accessibility || payload.Permissions.InputMonitoring {
		t.Fatalf("payload.Permissions = %#v, want accessibility=true inputMonitoring=false", payload.Permissions)
	}
}

func TestRouterHandleRequest_QueryMethods(t *testing.T) {
	service := NewService(&funcPlatform{
		LoadPermissionsFn: func(ctx context.Context) (PermissionsSnapshot, error) {
			return PermissionsSnapshot{Accessibility: true, InputMonitoring: false}, nil
		},
		ListDevicesFn: func(ctx context.Context) ([]DeviceSnapshot, error) {
			return []DeviceSnapshot{{Name: "Built-in Microphone", IsDefault: true}}, nil
		},
		LoadModelFn: func(ctx context.Context) (ModelSnapshot, error) {
			return ModelSnapshot{Size: "small", Ready: true}, nil
		},
		LoadAppStateFn: func(context.Context) (apppkg.AppState, error) {
			return apppkg.StateReady, nil
		},
		LoadLogsTailFn: func(context.Context) (LogTailSnapshot, error) {
			return LogTailSnapshot{
				Text:      "line 499\nline 500\n",
				Truncated: true,
				ByteSize:  1234,
				UpdatedAt: "2026-04-20T03:04:05Z",
			}, nil
		},
		LoadLogsFullFn: func(context.Context) (string, error) {
			return "line 001\nline 002\nline 003\n", nil
		},
		LoadUpdaterFn: func(context.Context) (UpdaterSnapshot, error) {
			return UpdaterSnapshot{
				Enabled:             true,
				SupportsManualCheck: true,
				FeedURL:             "https://example.com/appcast.xml",
				Channel:             "stable",
			}, nil
		},
		CheckForUpdatesFn: func(context.Context) error {
			return nil
		},
		WriteClipboardTextFn: func(context.Context, string) error { return nil },
	})
	router := NewRouter(service)

	tests := []struct {
		method string
		id     string
	}{
		{method: PermissionsGetMethod, id: "req-perms"},
		{method: DevicesListMethod, id: "req-devices"},
		{method: ModelGetMethod, id: "req-model"},
		{method: RuntimeGetMethod, id: "req-runtime"},
		{method: LogsGetMethod, id: "req-logs-get"},
		{method: LogsCopyTailMethod, id: "req-logs-copy-tail"},
		{method: LogsCopyAllMethod, id: "req-logs-copy"},
		{method: UpdaterGetMethod, id: "req-updater-get"},
		{method: UpdaterCheckMethod, id: "req-updater-check"},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			response := router.HandleRequest(context.Background(), RequestEnvelope{
				V:      ProtocolVersion,
				Kind:   KindRequest,
				ID:     tc.id,
				Method: tc.method,
				Params: json.RawMessage(`{}`),
			})
			if !response.OK {
				t.Fatalf("expected success for %s, got %#v", tc.method, response.Error)
			}
			switch tc.method {
			case LogsGetMethod:
				payload, ok := response.Result.(LogTailSnapshot)
				if !ok {
					t.Fatalf("Result = %#v, want LogTailSnapshot", response.Result)
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("Marshal logs payload: %v", err)
				}
				for _, want := range []string{`"text":"line 499\nline 500\n"`, `"truncated":true`, `"byteSize":1234`, `"updatedAt":"2026-04-20T03:04:05Z"`} {
					if !bytes.Contains(raw, []byte(want)) {
						t.Fatalf("serialized payload = %s, want field %s", raw, want)
					}
				}
				if !payload.Truncated || payload.ByteSize != 1234 || payload.UpdatedAt != "2026-04-20T03:04:05Z" {
					t.Fatalf("payload = %#v, want tail metadata", payload)
				}
				if payload.Text != "line 499\nline 500\n" {
					t.Fatalf("payload.Text = %q, want tail text", payload.Text)
				}
			case LogsCopyAllMethod:
				payload, ok := response.Result.(string)
				if !ok {
					t.Fatalf("Result = %#v, want string", response.Result)
				}
				if payload != "line 001\nline 002\nline 003\n" {
					t.Fatalf("payload = %q, want full log text", payload)
				}
			case LogsCopyTailMethod:
				payload, ok := response.Result.(string)
				if !ok {
					t.Fatalf("Result = %#v, want string", response.Result)
				}
				if payload != "line 499\nline 500\n" {
					t.Fatalf("payload = %q, want tail log text", payload)
				}
			case UpdaterGetMethod:
				payload, ok := response.Result.(UpdaterSnapshot)
				if !ok {
					t.Fatalf("Result = %#v, want UpdaterSnapshot", response.Result)
				}
				if !payload.Enabled || !payload.SupportsManualCheck {
					t.Fatalf("payload = %#v, want enabled manual-check updater", payload)
				}
				if payload.FeedURL != "https://example.com/appcast.xml" {
					t.Fatalf("payload.FeedURL = %q, want appcast URL", payload.FeedURL)
				}
			case UpdaterCheckMethod:
				payload, ok := response.Result.(map[string]any)
				if !ok {
					t.Fatalf("Result = %#v, want result map", response.Result)
				}
				if payload["started"] != true {
					t.Fatalf("payload[started] = %#v, want true", payload["started"])
				}
			}
		})
	}
}

// Drift-safety for missing platform methods is now enforced at compile time
// by the Platform interface — the previous "missing dependency returns
// ErrorCodeInternal" test scenario is unreachable in production.

func TestRouterHandleRequest_OpenPermissionSettings(t *testing.T) {
	var openedTarget string
	service := NewService(&funcPlatform{
		OpenPermissionSettingsFn: func(ctx context.Context, target string) error {
			openedTarget = target
			return nil
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-open-perms",
		Method: PermissionsOpenSettingsMethod,
		Params: json.RawMessage(`{"target":"accessibility"}`),
	})

	if !response.OK {
		t.Fatalf("expected success, got %#v", response.Error)
	}
	if openedTarget != "accessibility" {
		t.Fatalf("openedTarget = %q, want accessibility", openedTarget)
	}
}

func TestRouterHandleRequest_DevicesRefresh(t *testing.T) {
	refreshed := false
	service := NewService(&funcPlatform{
		RefreshDevicesFn: func(ctx context.Context) ([]DeviceSnapshot, error) {
			refreshed = true
			return []DeviceSnapshot{{Name: "USB Headset", IsDefault: false}}, nil
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-refresh-devices",
		Method: DevicesRefreshMethod,
		Params: json.RawMessage(`{}`),
	})

	if !response.OK {
		t.Fatalf("expected success, got %#v", response.Error)
	}
	if !refreshed {
		t.Fatal("expected RefreshDevices to be called")
	}
	result, ok := response.Result.(DevicesRefreshResult)
	if !ok {
		t.Fatalf("Result = %#v, want DevicesRefreshResult", response.Result)
	}
	if len(result.Devices) != 1 || result.Devices[0].Name != "USB Headset" {
		t.Fatalf("devices = %#v, want USB Headset snapshot", result.Devices)
	}
}

func TestRouterHandleRequest_DevicesRefreshFailure(t *testing.T) {
	service := NewService(&funcPlatform{
		RefreshDevicesFn: func(ctx context.Context) ([]DeviceSnapshot, error) {
			return nil, errors.New("portaudio refresh failed")
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-refresh-devices-fail",
		Method: DevicesRefreshMethod,
		Params: json.RawMessage(`{}`),
	})

	if response.OK {
		t.Fatal("expected devices.refresh failure")
	}
	if response.Error == nil || response.Error.Code != ErrorCodeDevicesRefreshFailed {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodeDevicesRefreshFailed)
	}
}

func TestRouterHandleRequest_ModelCommands(t *testing.T) {
	var downloaded string
	var deleted string
	var selected string
	service := NewService(&funcPlatform{
		DownloadModelFn: func(ctx context.Context, size string) error {
			downloaded = size
			return nil
		},
		DeleteModelFn: func(ctx context.Context, size string) error {
			deleted = size
			return nil
		},
		UseModelFn: func(ctx context.Context, size string) error {
			selected = size
			return nil
		},
	})
	router := NewRouter(service)

	tests := []struct {
		name   string
		method string
		id     string
		check  func(*testing.T)
	}{
		{
			name:   "download",
			method: ModelDownloadMethod,
			id:     "req-model-download",
			check: func(t *testing.T) {
				if downloaded != "medium" {
					t.Fatalf("downloaded = %q, want medium", downloaded)
				}
			},
		},
		{
			name:   "delete",
			method: ModelDeleteMethod,
			id:     "req-model-delete",
			check: func(t *testing.T) {
				if deleted != "medium" {
					t.Fatalf("deleted = %q, want medium", deleted)
				}
			},
		},
		{
			name:   "use",
			method: ModelUseMethod,
			id:     "req-model-use",
			check: func(t *testing.T) {
				if selected != "medium" {
					t.Fatalf("selected = %q, want medium", selected)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := router.HandleRequest(context.Background(), RequestEnvelope{
				V:      ProtocolVersion,
				Kind:   KindRequest,
				ID:     tc.id,
				Method: tc.method,
				Params: json.RawMessage(`{"size":"medium"}`),
			})

			if !response.OK {
				t.Fatalf("expected success, got %#v", response.Error)
			}
			tc.check(t)
		})
	}
}

func TestRouterHandleRequest_ModelCommandFailuresUseExplicitCodes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		wantCode string
		deps     *funcPlatform
	}{
		{
			name:     "download",
			method:   ModelDownloadMethod,
			wantCode: ErrorCodeModelDownloadFailed,
			deps: &funcPlatform{DownloadModelFn: func(ctx context.Context, size string) error {
				return errors.New("download failed")
			}},
		},
		{
			name:     "delete",
			method:   ModelDeleteMethod,
			wantCode: ErrorCodeModelDeleteFailed,
			deps: &funcPlatform{DeleteModelFn: func(ctx context.Context, size string) error {
				return errors.New("delete failed")
			}},
		},
		{
			name:     "use",
			method:   ModelUseMethod,
			wantCode: ErrorCodeModelUseFailed,
			deps: &funcPlatform{UseModelFn: func(ctx context.Context, size string) error {
				return errors.New("use failed")
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(NewService(tc.deps))
			response := router.HandleRequest(context.Background(), RequestEnvelope{
				V:      ProtocolVersion,
				Kind:   KindRequest,
				ID:     "req-model-fail",
				Method: tc.method,
				Params: json.RawMessage(`{"size":"medium"}`),
			})

			if response.OK {
				t.Fatal("expected failure response")
			}
			if response.Error == nil || response.Error.Code != tc.wantCode {
				t.Fatalf("Error = %#v, want code %q", response.Error, tc.wantCode)
			}
		})
	}
}

func TestRouterHandleRequest_OptionsGet(t *testing.T) {
	router := NewRouter(NewService(funcPlatform{}))

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-options",
		Method: OptionsGetMethod,
		Params: json.RawMessage(`{}`),
	})

	if !response.OK {
		t.Fatalf("expected success, got %#v", response.Error)
	}
	options, ok := response.Result.(SettingsOptionsSnapshot)
	if !ok {
		t.Fatalf("Result = %#v, want SettingsOptionsSnapshot", response.Result)
	}
	if len(options.Models) == 0 || len(options.DecodeModes) == 0 || len(options.PunctuationModes) == 0 || len(options.Languages) == 0 {
		t.Fatalf("expected non-empty options sets, got %#v", options)
	}
	if len(options.Hotkey.Modifiers) == 0 || len(options.Hotkey.Keys) == 0 {
		t.Fatalf("expected hotkey capability options, got %#v", options.Hotkey)
	}
	if runtime.GOOS == "windows" {
		if options.Hotkey.Modifiers[0] != "shift" {
			t.Fatalf("expected Windows host modifiers to start with shift, got %#v", options.Hotkey.Modifiers)
		}
		if options.Permissions.Accessibility.Required || options.Permissions.Accessibility.Actionable {
			t.Fatalf("expected Windows accessibility permission to be optional/non-actionable, got %#v", options.Permissions.Accessibility)
		}
		if options.Permissions.InputMonitoring.Required || options.Permissions.InputMonitoring.Actionable {
			t.Fatalf("expected Windows input monitoring permission to be optional/non-actionable, got %#v", options.Permissions.InputMonitoring)
		}
	} else {
		if options.Hotkey.Modifiers[0] != "fn" {
			t.Fatalf("expected Darwin host modifiers to start with fn, got %#v", options.Hotkey.Modifiers)
		}
		if !options.Permissions.Accessibility.Required || !options.Permissions.Accessibility.Actionable {
			t.Fatalf("expected Darwin accessibility permission to be required/actionable, got %#v", options.Permissions.Accessibility)
		}
		if !options.Permissions.InputMonitoring.Required || !options.Permissions.InputMonitoring.Actionable {
			t.Fatalf("expected Darwin input monitoring permission to be required/actionable, got %#v", options.Permissions.InputMonitoring)
		}
	}
}

func TestBridgeContractIncludesLogsMethods(t *testing.T) {
	if LogsGetMethod != "logs.get" {
		t.Fatalf("LogsGetMethod = %q, want logs.get", LogsGetMethod)
	}
	if LogsCopyTailMethod != "logs.copy_tail" {
		t.Fatalf("LogsCopyTailMethod = %q, want logs.copy_tail", LogsCopyTailMethod)
	}
	if LogsCopyAllMethod != "logs.copy_all" {
		t.Fatalf("LogsCopyAllMethod = %q, want logs.copy_all", LogsCopyAllMethod)
	}
}

func TestBridgeContractIncludesUpdaterMethods(t *testing.T) {
	if UpdaterGetMethod != "updater.get" {
		t.Fatalf("UpdaterGetMethod = %q, want updater.get", UpdaterGetMethod)
	}
	if UpdaterCheckMethod != "updater.check" {
		t.Fatalf("UpdaterCheckMethod = %q, want updater.check", UpdaterCheckMethod)
	}
}
