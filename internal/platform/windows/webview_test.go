//go:build windows

package windows

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

func TestBuildSettingsBridgeService_UsesTrackedRuntimeState(t *testing.T) {
	originalState := currentAppState()
	originalDefaultConfigPath := webSettingsDefaultConfigPath
	originalLoadConfig := webSettingsLoadConfig
	originalLoadPermissions := webSettingsLoadPermissions
	originalDefaultModelPath := defaultModelPath
	defer func() {
		storeCurrentAppState(originalState)
		webSettingsDefaultConfigPath = originalDefaultConfigPath
		webSettingsLoadConfig = originalLoadConfig
		webSettingsLoadPermissions = originalLoadPermissions
		defaultModelPath = originalDefaultModelPath
	}()

	storeCurrentAppState(StateRecording)
	webSettingsDefaultConfigPath = func() (string, error) {
		return filepath.Join(t.TempDir(), "config.yaml"), nil
	}
	webSettingsLoadConfig = func(string) (configpkg.Config, error) {
		return configpkg.Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			SoundFeedback:   true,
			InputDevice:     "USB Headset",
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
			Vocabulary:      "joicetyper",
		}, nil
	}
	webSettingsLoadPermissions = func() bridgepkg.PermissionsSnapshot {
		return bridgepkg.PermissionsSnapshot{Accessibility: true, InputMonitoring: false}
	}
	defaultModelPath = func(modelSize string) (string, error) {
		return filepath.Join(t.TempDir(), "ggml-"+modelSize+".bin"), nil
	}

	service := buildSettingsBridgeService(configpkg.Config{})
	if service == nil {
		t.Fatal("expected bridge service")
	}

	bootstrap, err := service.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	if bootstrap.AppState.State != apppkg.StateRecording.String() {
		t.Fatalf("Bootstrap.AppState.State = %q, want %q", bootstrap.AppState.State, apppkg.StateRecording.String())
	}
	if len(bootstrap.Options.Models) == 0 || len(bootstrap.Options.Languages) == 0 {
		t.Fatalf("Bootstrap.Options = %#v, want populated shared options", bootstrap.Options)
	}
}

func TestBuildBootstrapPayload_UsesSharedBootstrapEnvelope(t *testing.T) {
	originalDefaultConfigPath := webSettingsDefaultConfigPath
	originalLoadConfig := webSettingsLoadConfig
	originalLoadPermissions := webSettingsLoadPermissions
	originalDefaultModelPath := defaultModelPath
	defer func() {
		webSettingsDefaultConfigPath = originalDefaultConfigPath
		webSettingsLoadConfig = originalLoadConfig
		webSettingsLoadPermissions = originalLoadPermissions
		defaultModelPath = originalDefaultModelPath
	}()

	webSettingsDefaultConfigPath = func() (string, error) {
		return filepath.Join(t.TempDir(), "config.yaml"), nil
	}
	webSettingsLoadConfig = func(string) (configpkg.Config, error) {
		return configpkg.Config{
			ModelSize:       "medium",
			Language:        "en",
			SampleRate:      16000,
			DecodeMode:      "beam",
			PunctuationMode: "opinionated",
		}, nil
	}
	webSettingsLoadPermissions = func() bridgepkg.PermissionsSnapshot {
		return bridgepkg.PermissionsSnapshot{Accessibility: true, InputMonitoring: true}
	}
	defaultModelPath = func(modelSize string) (string, error) {
		return filepath.Join(t.TempDir(), "ggml-"+modelSize+".bin"), nil
	}

	bootstrap, err := buildBootstrapPayload(context.Background(), buildSettingsBridgeService(configpkg.Config{}))
	if err != nil {
		t.Fatalf("buildBootstrapPayload returned error: %v", err)
	}

	payload, err := injectBootstrapScript([]byte("<html><head></head><body></body></html>"), bootstrap)
	if err != nil {
		t.Fatalf("injectBootstrapScript returned error: %v", err)
	}
	html := string(payload)
	for _, snippet := range []string{
		"__JOICETYPER_BOOTSTRAP__",
		`"kind":"response"`,
		`"id":"bootstrap.get"`,
		`"modelSize":"medium"`,
	} {
		if !strings.Contains(html, snippet) {
			t.Fatalf("expected bootstrap HTML to contain %q, got %s", snippet, html)
		}
	}
}

func TestProcessWebSettingsMessage_RoutesThroughSharedBridge(t *testing.T) {
	originalDefaultConfigPath := webSettingsDefaultConfigPath
	originalLoadConfig := webSettingsLoadConfig
	originalLoadPermissions := webSettingsLoadPermissions
	originalDefaultModelPath := defaultModelPath
	originalService, hadService := activeWebSettingsBridgeService()
	defer func() {
		webSettingsDefaultConfigPath = originalDefaultConfigPath
		webSettingsLoadConfig = originalLoadConfig
		webSettingsLoadPermissions = originalLoadPermissions
		defaultModelPath = originalDefaultModelPath
		if hadService {
			setActiveWebSettingsBridgeService(originalService)
		} else {
			clearActiveWebSettingsBridgeService()
		}
	}()

	webSettingsDefaultConfigPath = func() (string, error) {
		return filepath.Join(t.TempDir(), "config.yaml"), nil
	}
	webSettingsLoadConfig = func(string) (configpkg.Config, error) {
		return configpkg.Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
		}, nil
	}
	webSettingsLoadPermissions = func() bridgepkg.PermissionsSnapshot {
		return bridgepkg.PermissionsSnapshot{Accessibility: true, InputMonitoring: false}
	}
	defaultModelPath = func(modelSize string) (string, error) {
		return filepath.Join(t.TempDir(), "ggml-"+modelSize+".bin"), nil
	}

	setActiveWebSettingsBridgeService(buildSettingsBridgeService(configpkg.Config{}))

	result := processWebSettingsMessage(`{"v":1,"kind":"request","id":"cfg","method":"config.get","params":{}}`)
	if result.closeWindow {
		t.Fatal("expected config.get to keep the window open")
	}
	if !result.response.OK {
		t.Fatalf("expected successful response, got %#v", result.response.Error)
	}
	configResult, ok := result.response.Result.(bridgepkg.ConfigSnapshot)
	if !ok {
		t.Fatalf("Result = %#v, want ConfigSnapshot", result.response.Result)
	}
	if configResult.ModelSize != "small" || configResult.Language != "en" {
		t.Fatalf("ConfigSnapshot = %#v, want shared bridge config", configResult)
	}
}

func TestNotifyWebSettingsLogsUpdated_DispatchesBridgeEvent(t *testing.T) {
	originalLogPath := webSettingsLogPath
	originalDispatch := webSettingsDispatchEnvelope
	defer func() {
		webSettingsLogPath = originalLogPath
		webSettingsDispatchEnvelope = originalDispatch
	}()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "voicetype.log")
	if err := os.WriteFile(logPath, []byte("line 001\nline 002\n"), 0644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	var payload string
	webSettingsLogPath = func() (string, error) {
		return logPath, nil
	}
	webSettingsDispatchEnvelope = func(s string, _ bool) {
		payload = s
	}

	notifyWebSettingsLogsUpdated()

	for _, snippet := range []string{
		`"kind":"event"`,
		`"event":"logs.updated"`,
		`"text":"line 001\nline 002\n"`,
		`"truncated":false`,
	} {
		if !strings.Contains(payload, snippet) {
			t.Fatalf("expected logs.updated payload to contain %q, got %q", snippet, payload)
		}
	}
	if !strings.Contains(payload, `"byteSize":18`) {
		t.Fatalf("expected logs.updated payload to contain byteSize, got %q", payload)
	}
	if !strings.Contains(payload, `"updatedAt":"`) {
		t.Fatalf("expected logs.updated payload to contain updatedAt, got %q", payload)
	}
}

func TestDispatchWebSettingsEvent_LogsMarshalFailure(t *testing.T) {
	var logs bytes.Buffer
	originalDispatch := webSettingsDispatchEnvelope
	defer func() {
		webSettingsDispatchEnvelope = originalDispatch
		SetSettingsLogger(nil)
	}()

	webSettingsDispatchEnvelope = func(string, bool) {
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

func TestWebView2DocumentCreatedScript_ProvidesSharedBridgeShim(t *testing.T) {
	script := webView2DocumentCreatedScript()
	for _, snippet := range []string{
		"window.webkit.messageHandlers.joicetyper.postMessage",
		"chrome.webview.postMessage(JSON.stringify(payload))",
		"chrome.webview.addEventListener('message'",
		bridgepkg.BridgeEventName,
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected document-created script to contain %q, got %q", snippet, script)
		}
	}
}

func TestDefaultWebSettingsHostHooks_UseNamedWebView2Shim(t *testing.T) {
	tests := []struct {
		name     string
		fn       any
		expected string
	}{
		{name: "show", fn: webSettingsShowWindow, expected: "showWindowsWebView2Host"},
		{name: "focus", fn: webSettingsFocusWindow, expected: "focusWindowsWebView2Host"},
		{name: "dispatch", fn: webSettingsDispatchEnvelope, expected: "dispatchWindowsWebView2Envelope"},
	}

	for _, tc := range tests {
		fnName := runtime.FuncForPC(reflect.ValueOf(tc.fn).Pointer()).Name()
		if !strings.Contains(fnName, tc.expected) {
			t.Fatalf("%s hook = %q, want name containing %q", tc.name, fnName, tc.expected)
		}
	}
}
