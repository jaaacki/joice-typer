//go:build darwin

package darwin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
