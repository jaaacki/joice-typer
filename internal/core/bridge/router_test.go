package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

func TestRouterHandleRequest_SaveConfig(t *testing.T) {
	var saved ConfigSnapshot
	service := NewService(&Dependencies{
		SaveConfig: func(ctx context.Context, cfg configpkg.Config) error {
			saved = configSnapshotFromConfig(cfg)
			return nil
		},
	})
	router := NewRouter(service)

	params, err := json.Marshal(map[string]ConfigSnapshot{
		"config": {
			ModelSize:       "medium",
			Language:        "en",
			SampleRate:      16000,
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
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
	router := NewRouter(NewService(nil))

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
	service := NewService(&Dependencies{
		SaveConfig: func(ctx context.Context, cfg configpkg.Config) error {
			return errors.New("disk full")
		},
	})
	router := NewRouter(service)

	response := router.HandleRequest(context.Background(), RequestEnvelope{
		V:      ProtocolVersion,
		Kind:   KindRequest,
		ID:     "req-3",
		Method: SaveConfigMethod,
		Params: json.RawMessage(`{"config":{"modelSize":"small","language":"en","sampleRate":16000,"decodeMode":"beam","punctuationMode":"conservative"}}`),
	})

	if response.OK {
		t.Fatal("expected save failure response")
	}
	if response.Error == nil || response.Error.Code != ErrorCodeSaveFailure {
		t.Fatalf("Error = %#v, want code %q", response.Error, ErrorCodeSaveFailure)
	}
}

func TestRouterHandleRequest_PreservesContractErrorCode(t *testing.T) {
	service := NewService(&Dependencies{
		OpenPermissionSettings: func(ctx context.Context, target string) error {
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
	service := NewService(&Dependencies{
		LoadConfig: func(ctx context.Context) (configpkg.Config, error) {
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
	service := NewService(&Dependencies{
		LoadConfig: func(ctx context.Context) (configpkg.Config, error) {
			return configpkg.Config{
				ModelSize:       "small",
				Language:        "en",
				SampleRate:      16000,
				DecodeMode:      "beam",
				PunctuationMode: "conservative",
			}, nil
		},
		LoadAppState: func(context.Context) (apppkg.AppState, error) {
			return apppkg.StateReady, nil
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
}

func TestRouterHandleRequest_QueryMethods(t *testing.T) {
	service := NewService(&Dependencies{
		LoadPermissions: func(ctx context.Context) (PermissionsSnapshot, error) {
			return PermissionsSnapshot{Accessibility: true, InputMonitoring: false}, nil
		},
		ListDevices: func(ctx context.Context) ([]DeviceSnapshot, error) {
			return []DeviceSnapshot{{Name: "Built-in Microphone", IsDefault: true}}, nil
		},
		LoadModel: func(ctx context.Context) (ModelSnapshot, error) {
			return ModelSnapshot{Size: "small", Ready: true}, nil
		},
		LoadAppState: func(context.Context) (apppkg.AppState, error) {
			return apppkg.StateReady, nil
		},
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
		})
	}
}

func TestRouterHandleRequest_OpenPermissionSettings(t *testing.T) {
	var openedTarget string
	service := NewService(&Dependencies{
		OpenPermissionSettings: func(ctx context.Context, target string) error {
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
	service := NewService(&Dependencies{
		RefreshDevices: func(ctx context.Context) ([]DeviceSnapshot, error) {
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
	service := NewService(&Dependencies{
		RefreshDevices: func(ctx context.Context) ([]DeviceSnapshot, error) {
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
	service := NewService(&Dependencies{
		DownloadModel: func(ctx context.Context, size string) error {
			downloaded = size
			return nil
		},
		DeleteModel: func(ctx context.Context, size string) error {
			deleted = size
			return nil
		},
		UseModel: func(ctx context.Context, size string) error {
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
		deps     *Dependencies
	}{
		{
			name:     "download",
			method:   ModelDownloadMethod,
			wantCode: ErrorCodeModelDownloadFailed,
			deps: &Dependencies{DownloadModel: func(ctx context.Context, size string) error {
				return errors.New("download failed")
			}},
		},
		{
			name:     "delete",
			method:   ModelDeleteMethod,
			wantCode: ErrorCodeModelDeleteFailed,
			deps: &Dependencies{DeleteModel: func(ctx context.Context, size string) error {
				return errors.New("delete failed")
			}},
		},
		{
			name:     "use",
			method:   ModelUseMethod,
			wantCode: ErrorCodeModelUseFailed,
			deps: &Dependencies{UseModel: func(ctx context.Context, size string) error {
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
	router := NewRouter(NewService(nil))

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
}
