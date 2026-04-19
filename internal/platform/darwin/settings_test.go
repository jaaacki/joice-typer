//go:build darwin

package darwin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bridgepkg "voicetype/internal/core/bridge"
)

func TestRequireSettingSelection_RejectsEmptyDecodeMode(t *testing.T) {
	_, err := requireSettingSelection("decode_mode", "")
	if err == nil {
		t.Fatal("expected empty decode_mode selection to fail")
	}
	if !strings.Contains(err.Error(), "decode_mode") {
		t.Fatalf("expected error to mention decode_mode, got %v", err)
	}
}

func TestRequireSettingSelection_RejectsEmptyPunctuationMode(t *testing.T) {
	_, err := requireSettingSelection("punctuation_mode", "")
	if err == nil {
		t.Fatal("expected empty punctuation_mode selection to fail")
	}
	if !strings.Contains(err.Error(), "punctuation_mode") {
		t.Fatalf("expected error to mention punctuation_mode, got %v", err)
	}
}

func TestRequireSettingSelection_RejectsInvalidDecodeMode(t *testing.T) {
	_, err := requireSettingSelection("decode_mode", "turbo")
	if err == nil {
		t.Fatal("expected invalid decode_mode selection to fail")
	}
	if !strings.Contains(err.Error(), "decode_mode") {
		t.Fatalf("expected error to mention decode_mode, got %v", err)
	}
}

func TestRequireSettingSelection_RejectsInvalidPunctuationMode(t *testing.T) {
	_, err := requireSettingSelection("punctuation_mode", "chaos")
	if err == nil {
		t.Fatal("expected invalid punctuation_mode selection to fail")
	}
	if !strings.Contains(err.Error(), "punctuation_mode") {
		t.Fatalf("expected error to mention punctuation_mode, got %v", err)
	}
}

func TestSettingsDarwin_ReusedWindowUpdatesSaveButtonVisibility(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "settings_darwin.m"))
	if err != nil {
		t.Fatalf("read settings_darwin.m: %v", err)
	}
	source := string(data)
	for _, snippet := range []string{
		"static NSButton *sSaveButton = nil;",
		"sSaveButton.hidden = sIsOnboarding;",
		"sSaveButton = [[NSButton alloc] initWithFrame:",
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected settings_darwin.m to contain %q", snippet)
		}
	}
	if strings.Contains(source, "NSButton *saveBtn = [[NSButton alloc]") {
		t.Fatal("expected settings_darwin.m to stop using a local saveBtn variable")
	}
}

func TestReportSettingsSaveError_PostsNotification(t *testing.T) {
	var gotTitle, gotBody string
	original := postNotification
	postNotification = func(title, body string) {
		gotTitle, gotBody = title, body
	}
	defer func() { postNotification = original }()

	reportSettingsSaveError("invalid decode mode selection")

	if gotTitle == "" || gotBody == "" {
		t.Fatal("expected reportSettingsSaveError to post a notification")
	}
	if !strings.Contains(gotBody, "invalid decode mode selection") {
		t.Fatalf("expected notification body to include original error, got %q", gotBody)
	}
}

func TestResolveModelPathForSettings_ReportsNotification(t *testing.T) {
	originalPath := defaultModelPath
	originalNotify := postNotification
	defer func() {
		defaultModelPath = originalPath
		postNotification = originalNotify
	}()

	defaultModelPath = func(modelSize string) (string, error) {
		return "", os.ErrPermission
	}

	var gotTitle, gotBody string
	postNotification = func(title, body string) {
		gotTitle, gotBody = title, body
	}

	if _, ok := resolveModelPathForSettings("small", "testOp"); ok {
		t.Fatal("expected resolveModelPathForSettings to fail")
	}
	if gotTitle == "" || !strings.Contains(gotBody, "permission denied") {
		t.Fatalf("expected notification for model path failure, got title=%q body=%q", gotTitle, gotBody)
	}
}

func TestSettingsSource_UsesWebViewHostHook(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "settings.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ShowWebSettingsWindow") {
		t.Fatal("expected settings flow to reference web settings host")
	}
}

func TestSettingsSource_WebFlowDoesNotClearPreferencesOpenImmediately(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "settings.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	webFlowIndex := strings.Index(source, "if shouldUseWebSettings() {")
	if webFlowIndex == -1 {
		t.Fatal("expected web settings flow in settings.go")
	}
	webSlice := source[webFlowIndex:]
	if !strings.Contains(webSlice, "ShowWebSettingsWindowWithBridge") {
		t.Fatal("expected web settings flow to open the web settings window")
	}
	if !strings.Contains(webSlice, "preferencesOpenStore(0)") {
		t.Fatal("expected explicit cleanup on web settings open failure")
	}
	if !strings.Contains(webSlice, "postNotification(\"JoiceTyper Preferences\"") {
		t.Fatal("expected web settings failure path to notify the user")
	}
}

func TestSettingsSource_WebFlowNoLongerSilentlyFallsBackToNativePreferences(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "settings.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if strings.Contains(source, "falling back to native preferences") {
		t.Fatal("expected web settings flow to stop silently falling back to native preferences")
	}
	if !strings.Contains(source, "JOICETYPER_USE_NATIVE_PREFERENCES=1") {
		t.Fatal("expected web settings failure path to document the hidden native fallback env")
	}
}

func TestSettingsSource_WebFlowSeedsActiveModelBeforeOpen(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "settings.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	webFlowIndex := strings.Index(source, "if shouldUseWebSettings() {")
	if webFlowIndex == -1 {
		t.Fatal("expected web settings flow in settings.go")
	}
	webSlice := source[webFlowIndex:]
	seedIndex := strings.Index(webSlice, "prefsActiveModel = cfg.ModelSize")
	openIndex := strings.Index(webSlice, "ShowWebSettingsWindowWithBridge")
	if seedIndex == -1 {
		t.Fatal("expected web settings flow to seed prefsActiveModel from config")
	}
	if openIndex == -1 {
		t.Fatal("expected web settings flow to open the web settings window")
	}
	if seedIndex > openIndex {
		t.Fatal("expected prefsActiveModel seeding before opening web settings")
	}
}

func TestSettingsSource_ReactivatesExistingPreferencesWindow(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".", "settings.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	reopenIndex := strings.Index(source, `if !preferencesOpenCompareAndSwap(0, 1) {`)
	if reopenIndex == -1 {
		t.Fatal("expected preferences-open guard in settings.go")
	}
	reopenSlice := source[reopenIndex:]
	for _, required := range []string{
		`preferences already open, reactivating existing window`,
		`if shouldUseWebSettings() {`,
		`FocusWebSettingsWindow()`,
		`C.showSettingsWindow(0)`,
	} {
		if !strings.Contains(reopenSlice, required) {
			t.Fatalf("expected existing-open branch to contain %q", required)
		}
	}
	if strings.Contains(reopenSlice, `preferences already open, ignoring`) {
		t.Fatal("expected existing-open branch to stop ignoring repeated preferences clicks")
	}
}

func TestDeleteWebSettingsModel_RejectsActiveModel(t *testing.T) {
	originalPath := defaultModelPath
	originalRemove := removeFile
	originalActiveModel := prefsActiveModel
	defer func() {
		defaultModelPath = originalPath
		removeFile = originalRemove
		prefsActiveModel = originalActiveModel
	}()

	modelPath := filepath.Join(t.TempDir(), "ggml-small.bin")
	defaultModelPath = func(modelSize string) (string, error) {
		return modelPath, nil
	}

	var removeCalls int
	removeFile = func(string) error {
		removeCalls++
		return nil
	}

	prefsActiveModel = "small"
	err := deleteWebSettingsModel("small")
	if err == nil {
		t.Fatal("expected deleting the active model to fail")
	}
	contractErr, ok := bridgepkg.AsContractError(err)
	if !ok {
		t.Fatalf("expected contract error, got %T", err)
	}
	if contractErr.Code != bridgepkg.ErrorCodeModelDeleteFailed {
		t.Fatalf("contractErr.Code = %q, want %q", contractErr.Code, bridgepkg.ErrorCodeModelDeleteFailed)
	}
	if removeCalls != 0 {
		t.Fatalf("removeCalls = %d, want 0", removeCalls)
	}
}
