//go:build darwin

package darwin

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
)

func TestBuildSettingsBridgeService_UsesTrackedRuntimeState(t *testing.T) {
	originalState := currentAppState()
	originalConfigPath := webSettingsDefaultConfigPath
	originalLoadConfig := webSettingsLoadConfig
	storeCurrentAppState(StateNoPermission)
	defer func() {
		storeCurrentAppState(originalState)
		webSettingsDefaultConfigPath = originalConfigPath
		webSettingsLoadConfig = originalLoadConfig
	}()
	webSettingsDefaultConfigPath = func() (string, error) { return filepath.Join(t.TempDir(), "config.yaml"), nil }
	webSettingsLoadConfig = func(string) (configpkg.Config, error) {
		return configpkg.Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			SoundFeedback:   true,
			InputDevice:     "Built-in Microphone",
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
		}, nil
	}

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

func TestShouldUseWebSettings_DefaultsToWebAndAllowsFallbackEnv(t *testing.T) {
	t.Setenv("JOICETYPER_USE_WEB_SETTINGS", "")
	t.Setenv("JOICETYPER_USE_NATIVE_PREFERENCES", "")
	if !shouldUseWebSettings() {
		t.Fatal("expected web settings to be the default path")
	}

	t.Setenv("JOICETYPER_USE_NATIVE_PREFERENCES", "1")
	if shouldUseWebSettings() {
		t.Fatal("expected native fallback env to disable web settings")
	}

	t.Setenv("JOICETYPER_USE_NATIVE_PREFERENCES", "")
	t.Setenv("JOICETYPER_USE_WEB_SETTINGS", "0")
	if shouldUseWebSettings() {
		t.Fatal("expected explicit JOICETYPER_USE_WEB_SETTINGS=0 to disable web settings")
	}
}

func TestBuildSettingsBridgeService_LoadsPermissionsDevicesAndModel(t *testing.T) {
	originalPermissions := webSettingsLoadPermissions
	originalDevices := webSettingsListInputDevices
	originalModelPath := defaultModelPath
	originalConfigPath := webSettingsDefaultConfigPath
	originalLoadConfig := webSettingsLoadConfig
	defer func() {
		webSettingsLoadPermissions = originalPermissions
		webSettingsListInputDevices = originalDevices
		defaultModelPath = originalModelPath
		webSettingsDefaultConfigPath = originalConfigPath
		webSettingsLoadConfig = originalLoadConfig
	}()

	webSettingsLoadPermissions = func() bridgepkg.PermissionsSnapshot {
		return bridgepkg.PermissionsSnapshot{Accessibility: true, InputMonitoring: false}
	}
	webSettingsListInputDevices = func() ([]bridgepkg.DeviceSnapshot, error) {
		return []bridgepkg.DeviceSnapshot{
			{Name: "Built-in Microphone", IsDefault: true},
			{Name: "USB Headset", IsDefault: false},
		}, nil
	}
	modelPath := filepath.Join(t.TempDir(), "ggml-small.bin")
	defaultModelPath = func(modelSize string) (string, error) {
		return modelPath, nil
	}
	webSettingsDefaultConfigPath = func() (string, error) { return filepath.Join(t.TempDir(), "config.yaml"), nil }
	webSettingsLoadConfig = func(string) (configpkg.Config, error) {
		return configpkg.Config{
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
		}, nil
	}

	service := buildSettingsBridgeService(configpkg.Config{
		ModelSize:       "small",
		Language:        "en",
		SampleRate:      16000,
		DecodeMode:      "beam",
		PunctuationMode: "conservative",
	})

	permissions, err := service.Permissions(context.Background())
	if err != nil {
		t.Fatalf("Permissions returned error: %v", err)
	}
	if !permissions.Accessibility || permissions.InputMonitoring {
		t.Fatalf("Permissions = %#v, want accessibility=true inputMonitoring=false", permissions)
	}

	devices, err := service.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices returned error: %v", err)
	}
	if len(devices) != 2 || !devices[0].IsDefault {
		t.Fatalf("Devices = %#v, want two devices with first default", devices)
	}

	model, err := service.Model(context.Background())
	if err != nil {
		t.Fatalf("Model returned error: %v", err)
	}
	if model.Size != "small" || model.Path != modelPath || model.Ready {
		t.Fatalf("Model = %#v, want size=small path=%q ready=false", model, modelPath)
	}
}

func TestBuildSettingsBridgeService_RefreshDevicesAndModelCommands(t *testing.T) {
	originalRefreshDevices := webSettingsRefreshDevices
	originalDownloadModel := webSettingsDownloadModel
	originalDeleteModel := webSettingsDeleteModel
	originalUseModel := webSettingsUseModel
	defer func() {
		webSettingsRefreshDevices = originalRefreshDevices
		webSettingsDownloadModel = originalDownloadModel
		webSettingsDeleteModel = originalDeleteModel
		webSettingsUseModel = originalUseModel
	}()

	var refreshed bool
	var downloaded string
	var deleted string
	var selected string
	webSettingsRefreshDevices = func() ([]bridgepkg.DeviceSnapshot, error) {
		refreshed = true
		return []bridgepkg.DeviceSnapshot{{Name: "USB Headset", IsDefault: false}}, nil
	}
	webSettingsDownloadModel = func(size string) error {
		downloaded = size
		return nil
	}
	webSettingsDeleteModel = func(size string) error {
		deleted = size
		return nil
	}
	webSettingsUseModel = func(size string) error {
		selected = size
		return nil
	}

	service := buildSettingsBridgeService(configpkg.Config{
		ModelSize:       "small",
		Language:        "en",
		SampleRate:      16000,
		DecodeMode:      "beam",
		PunctuationMode: "conservative",
	})

	devices, err := service.RefreshDevices(context.Background())
	if err != nil {
		t.Fatalf("RefreshDevices returned error: %v", err)
	}
	if !refreshed || len(devices) != 1 || devices[0].Name != "USB Headset" {
		t.Fatalf("devices = %#v, refreshed=%t", devices, refreshed)
	}
	if err := service.DownloadModel(context.Background(), "medium"); err != nil {
		t.Fatalf("DownloadModel returned error: %v", err)
	}
	if err := service.DeleteModel(context.Background(), "base"); err != nil {
		t.Fatalf("DeleteModel returned error: %v", err)
	}
	if err := service.UseModel(context.Background(), "small"); err != nil {
		t.Fatalf("UseModel returned error: %v", err)
	}
	if downloaded != "medium" || deleted != "base" || selected != "small" {
		t.Fatalf("downloaded=%q deleted=%q selected=%q", downloaded, deleted, selected)
	}
}

func TestBuildSettingsBridgeService_CommandFailuresPreserveContractCodes(t *testing.T) {
	originalRefreshDevices := webSettingsRefreshDevices
	originalDownloadModel := webSettingsDownloadModel
	originalDeleteModel := webSettingsDeleteModel
	originalUseModel := webSettingsUseModel
	defer func() {
		webSettingsRefreshDevices = originalRefreshDevices
		webSettingsDownloadModel = originalDownloadModel
		webSettingsDeleteModel = originalDeleteModel
		webSettingsUseModel = originalUseModel
	}()

	webSettingsRefreshDevices = func() ([]bridgepkg.DeviceSnapshot, error) {
		return nil, bridgepkg.NewContractError(bridgepkg.ErrorCodeDevicesRefreshFailed, "Failed to refresh input devices", true, nil)
	}
	webSettingsDownloadModel = func(size string) error {
		return bridgepkg.NewContractError(bridgepkg.ErrorCodeModelDownloadFailed, "Failed to download model", true, map[string]any{"size": size})
	}
	webSettingsDeleteModel = func(size string) error {
		return bridgepkg.NewContractError(bridgepkg.ErrorCodeModelDeleteFailed, "Failed to delete model", false, map[string]any{"size": size})
	}
	webSettingsUseModel = func(size string) error {
		return bridgepkg.NewContractError(bridgepkg.ErrorCodeModelUseFailed, "Failed to use model", false, map[string]any{"size": size})
	}

	service := buildSettingsBridgeService(configpkg.Config{
		ModelSize:       "small",
		Language:        "en",
		SampleRate:      16000,
		DecodeMode:      "beam",
		PunctuationMode: "conservative",
	})

	if _, err := service.RefreshDevices(context.Background()); err == nil {
		t.Fatal("expected RefreshDevices error")
	} else if contractErr, ok := bridgepkg.AsContractError(err); !ok || contractErr.Code != bridgepkg.ErrorCodeDevicesRefreshFailed {
		t.Fatalf("RefreshDevices error = %#v, want code %q", err, bridgepkg.ErrorCodeDevicesRefreshFailed)
	}
	if err := service.DownloadModel(context.Background(), "medium"); err == nil {
		t.Fatal("expected DownloadModel error")
	} else if contractErr, ok := bridgepkg.AsContractError(err); !ok || contractErr.Code != bridgepkg.ErrorCodeModelDownloadFailed {
		t.Fatalf("DownloadModel error = %#v, want code %q", err, bridgepkg.ErrorCodeModelDownloadFailed)
	}
	if err := service.DeleteModel(context.Background(), "medium"); err == nil {
		t.Fatal("expected DeleteModel error")
	} else if contractErr, ok := bridgepkg.AsContractError(err); !ok || contractErr.Code != bridgepkg.ErrorCodeModelDeleteFailed {
		t.Fatalf("DeleteModel error = %#v, want code %q", err, bridgepkg.ErrorCodeModelDeleteFailed)
	}
	if err := service.UseModel(context.Background(), "medium"); err == nil {
		t.Fatal("expected UseModel error")
	} else if contractErr, ok := bridgepkg.AsContractError(err); !ok || contractErr.Code != bridgepkg.ErrorCodeModelUseFailed {
		t.Fatalf("UseModel error = %#v, want code %q", err, bridgepkg.ErrorCodeModelUseFailed)
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
	for _, snippet := range []string{
		`"v":1`,
		`"kind":"response"`,
		`"id":"bootstrap.get"`,
		`"modelSize":"small"`,
	} {
		if !strings.Contains(string(out), snippet) {
			t.Fatalf("expected bootstrap payload JSON to contain %q", snippet)
		}
	}
}

func TestWebSettingsWindowClosed_ClearsPreferencesOpenFlag(t *testing.T) {
	preferencesOpenStore(1)
	dir := t.TempDir()
	trackWebSettingsAssetsRoot(dir)
	webSettingsWindowClosed()
	if !preferencesOpenCompareAndSwap(0, 1) {
		t.Fatal("expected webSettingsWindowClosed to clear preferences open flag")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected tracked web settings assets dir %q to be removed, stat err=%v", dir, err)
	}
	preferencesOpenStore(0)
}

func TestProcessWebSettingsMessage_ReturnsStructuredErrorResponse(t *testing.T) {
	originalSave := webSettingsSaveConfig
	originalPostError := webSettingsPostError
	originalConfigPath := webSettingsDefaultConfigPath
	originalLoadConfig := webSettingsLoadConfig
	defer func() {
		webSettingsSaveConfig = originalSave
		webSettingsPostError = originalPostError
		webSettingsDefaultConfigPath = originalConfigPath
		webSettingsLoadConfig = originalLoadConfig
	}()
	webSettingsDefaultConfigPath = func() (string, error) { return filepath.Join(t.TempDir(), "config.yaml"), nil }
	webSettingsLoadConfig = func(string) (configpkg.Config, error) {
		return configpkg.Config{
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			SoundFeedback:   true,
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
		}, nil
	}
	webSettingsSaveConfig = func(path string, cfg configpkg.Config) error {
		return os.ErrPermission
	}
	webSettingsPostError = func(string) {}

	result := processWebSettingsMessage(`{"v":1,"kind":"request","id":"abc","method":"config.save","params":{"config":{"modelSize":"small","language":"en","sampleRate":16000,"soundFeedback":true,"decodeMode":"beam","punctuationMode":"conservative"}}}`)
	response := result.response
	if response.ID != "abc" {
		t.Fatalf("ID = %q, want abc", response.ID)
	}
	if response.Kind != "response" {
		t.Fatalf("Kind = %q, want response", response.Kind)
	}
	if response.OK {
		t.Fatal("expected failed save response")
	}
	if response.Error == nil || response.Error.Message == "" {
		t.Fatal("expected error message in failed save response")
	}
	if response.Error.Code == "" {
		t.Fatal("expected structured error code in failed save response")
	}
}

func TestProcessWebSettingsMessage_ConfigGetUsesFullBridgeService(t *testing.T) {
	originalConfigPath := webSettingsDefaultConfigPath
	originalLoadConfig := webSettingsLoadConfig
	defer func() {
		webSettingsDefaultConfigPath = originalConfigPath
		webSettingsLoadConfig = originalLoadConfig
	}()

	webSettingsDefaultConfigPath = func() (string, error) { return filepath.Join(t.TempDir(), "config.yaml"), nil }
	webSettingsLoadConfig = func(string) (configpkg.Config, error) {
		return configpkg.Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "medium",
			Language:        "en",
			SampleRate:      16000,
			SoundFeedback:   true,
			InputDevice:     "USB Headset",
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
		}, nil
	}

	result := processWebSettingsMessage(`{"v":1,"kind":"request","id":"cfg","method":"config.get","params":{}}`)
	if result.closeWindow {
		t.Fatal("expected config.get to keep the window open")
	}
	if !result.response.OK {
		t.Fatalf("expected successful response, got %#v", result.response.Error)
	}
	configResult, ok := result.response.Result.(bridgepkg.ConfigSnapshot)
	if !ok {
		t.Fatalf("expected ConfigSnapshot result, got %T", result.response.Result)
	}
	if configResult.ModelSize != "medium" || configResult.InputDevice != "USB Headset" {
		t.Fatalf("configResult = %#v", configResult)
	}
}

func TestWebviewSource_UsesTrackedBridgeServiceForLiveRequests(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "webview.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	processIndex := strings.Index(source, "func processWebSettingsMessage(messageJSON string)")
	if processIndex == -1 {
		t.Fatal("expected processWebSettingsMessage in webview.go")
	}
	processSlice := source[processIndex:]
	if strings.Contains(processSlice, "buildSettingsBridgeService(configpkg.Config{})") {
		t.Fatal("expected processWebSettingsMessage to stop rebuilding a fresh bridge service per request")
	}
	if !strings.Contains(source, "setActiveWebSettingsBridgeService") {
		t.Fatal("expected webview host to track the active bridge service for live requests")
	}
	if !strings.Contains(processSlice, "activeWebSettingsBridgeService()") {
		t.Fatal("expected processWebSettingsMessage to use the tracked bridge service")
	}
}

func TestWebviewSource_DoesNotSilentlyDropEventOrCleanupFailures(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "webview.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{
		"_ = webSettingsCleanupDir(webSettingsAssetsRoot)",
		"_ = webSettingsCleanupDir(root)",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("expected webview.go not to silently drop failure path %q", forbidden)
		}
	}
	for _, required := range []string{
		"failed to marshal bridge event",
		"failed to clean up web settings assets",
		"dispatchWebSettingsEvent(",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected webview.go to contain %q", required)
		}
	}
}

func TestWebSettingsResponseScript_ReturnsBooleanResult(t *testing.T) {
	script := webSettingsResponseScript(webSettingsResponse{
		V:    1,
		Kind: "response",
		ID:   "abc",
		OK:   true,
	}, false)
	for _, snippet := range []string{
		`joicetyper-bridge-message`,
		`"kind":"response"`,
		`"id":"abc"`,
		`return false`,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected success script to contain %q, got %q", snippet, script)
		}
	}
}

func TestWebSettingsResponseScript_CanRequestWindowClose(t *testing.T) {
	script := webSettingsResponseScript(webSettingsResponse{
		V:    1,
		Kind: "response",
		ID:   "save",
		OK:   true,
	}, true)
	if !strings.Contains(script, "return true") {
		t.Fatalf("expected save script to request window close, got %q", script)
	}
}

func TestPublishRuntimeStateChanged_DispatchesBridgeEventScript(t *testing.T) {
	originalDispatch := webSettingsDispatchScript
	defer func() {
		webSettingsDispatchScript = originalDispatch
	}()

	var script string
	webSettingsDispatchScript = func(s string) {
		script = s
	}

	publishRuntimeStateChanged(StateRecording)

	for _, snippet := range []string{
		`joicetyper-bridge-message`,
		`"kind":"event"`,
		`"event":"runtime.state_changed"`,
		`"state":"recording"`,
		`"version":"`,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected runtime state event script to contain %q, got %q", snippet, script)
		}
	}
}

func TestDispatchWebSettingsEvent_LogsMarshalFailure(t *testing.T) {
	var logs bytes.Buffer
	originalDispatch := webSettingsDispatchScript
	defer func() {
		webSettingsDispatchScript = originalDispatch
		SetSettingsLogger(nil)
	}()

	webSettingsDispatchScript = func(string) {
		t.Fatal("did not expect dispatch when event marshaling fails")
	}
	SetSettingsLogger(slog.New(slog.NewJSONHandler(&logs, nil)))

	dispatchWebSettingsEvent(bridgepkg.EventEnvelope{
		V:       1,
		Kind:    "event",
		Event:   "test.failure",
		Payload: map[string]any{"bad": make(chan int)},
	})

	if !strings.Contains(logs.String(), "failed to marshal bridge event") {
		t.Fatalf("expected marshal failure to be logged, got %q", logs.String())
	}
}

func TestCleanupTrackedWebSettingsAssets_LogsCleanupFailure(t *testing.T) {
	var logs bytes.Buffer
	originalCleanup := webSettingsCleanupDir
	defer func() {
		webSettingsCleanupDir = originalCleanup
		SetSettingsLogger(nil)
	}()

	webSettingsCleanupDir = func(string) error {
		return os.ErrPermission
	}
	SetSettingsLogger(slog.New(slog.NewJSONHandler(&logs, nil)))

	trackWebSettingsAssetsRoot("/tmp/joicetyper-web-ui-stale")
	cleanupTrackedWebSettingsAssets()

	if !strings.Contains(logs.String(), "failed to clean up web settings assets") {
		t.Fatalf("expected cleanup failure to be logged, got %q", logs.String())
	}
}

func TestPublishPermissionsChanged_DispatchesBridgeEventScript(t *testing.T) {
	originalDispatch := webSettingsDispatchScript
	defer func() {
		webSettingsDispatchScript = originalDispatch
	}()

	var script string
	webSettingsDispatchScript = func(s string) {
		script = s
	}

	publishPermissionsChanged(bridgepkg.PermissionsSnapshot{
		Accessibility:   true,
		InputMonitoring: false,
	})

	for _, snippet := range []string{
		`joicetyper-bridge-message`,
		`"kind":"event"`,
		`"event":"permissions.changed"`,
		`"accessibility":true`,
		`"inputMonitoring":false`,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected permissions event script to contain %q, got %q", snippet, script)
		}
	}
}

func TestPublishModelChanged_DispatchesBridgeEventScript(t *testing.T) {
	originalDispatch := webSettingsDispatchScript
	defer func() {
		webSettingsDispatchScript = originalDispatch
	}()

	var script string
	webSettingsDispatchScript = func(s string) {
		script = s
	}

	publishModelChanged(bridgepkg.ModelSnapshot{
		Size:  "medium",
		Path:  "/tmp/model.bin",
		Ready: true,
	})

	for _, snippet := range []string{
		`joicetyper-bridge-message`,
		`"kind":"event"`,
		`"event":"model.changed"`,
		`"size":"medium"`,
		`"ready":true`,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected model event script to contain %q, got %q", snippet, script)
		}
	}
}

func TestPublishModelDownloadProgress_DispatchesBridgeEventScript(t *testing.T) {
	originalDispatch := webSettingsDispatchScript
	defer func() {
		webSettingsDispatchScript = originalDispatch
	}()

	var script string
	webSettingsDispatchScript = func(s string) {
		script = s
	}

	publishModelDownloadProgress("large", 0.5, 50, 100)

	for _, snippet := range []string{
		`joicetyper-bridge-message`,
		`"kind":"event"`,
		`"event":"model.download_progress"`,
		`"size":"large"`,
		`"progress":0.5`,
		`"bytesDownloaded":50`,
		`"bytesTotal":100`,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected model progress event script to contain %q, got %q", snippet, script)
		}
	}
}

func TestPublishConfigSaved_DispatchesBridgeEventScript(t *testing.T) {
	originalDispatch := webSettingsDispatchScript
	defer func() {
		webSettingsDispatchScript = originalDispatch
	}()

	var script string
	webSettingsDispatchScript = func(s string) {
		script = s
	}

	publishConfigSaved(bridgepkg.ConfigSnapshot{
		ModelSize:       "medium",
		Language:        "en",
		SampleRate:      16000,
		DecodeMode:      "beam",
		PunctuationMode: "conservative",
	})

	for _, snippet := range []string{
		`joicetyper-bridge-message`,
		`"kind":"event"`,
		`"event":"config.saved"`,
		`"modelSize":"medium"`,
		`"decodeMode":"beam"`,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected config saved event script to contain %q, got %q", snippet, script)
		}
	}
}

func TestPublishDevicesChanged_DispatchesBridgeEventScript(t *testing.T) {
	originalDispatch := webSettingsDispatchScript
	defer func() {
		webSettingsDispatchScript = originalDispatch
	}()

	var script string
	webSettingsDispatchScript = func(s string) {
		script = s
	}

	publishDevicesChanged([]bridgepkg.DeviceSnapshot{
		{Name: "Built-in Microphone", IsDefault: true},
		{Name: "USB Headset", IsDefault: false},
	})

	for _, snippet := range []string{
		`joicetyper-bridge-message`,
		`"kind":"event"`,
		`"event":"devices.changed"`,
		`"name":"Built-in Microphone"`,
		`"isDefault":true`,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected devices changed event script to contain %q, got %q", snippet, script)
		}
	}
}
