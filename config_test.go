package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_CreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Verify defaults
	if len(cfg.TriggerKey) != 2 || cfg.TriggerKey[0] != "fn" || cfg.TriggerKey[1] != "shift" {
		t.Errorf("expected trigger_key [fn shift], got %v", cfg.TriggerKey)
	}
	if cfg.ModelSize != "small" {
		t.Errorf("expected model_size small, got %s", cfg.ModelSize)
	}
	if cfg.Language != "" {
		t.Errorf("expected empty language, got %s", cfg.Language)
	}
	if cfg.SampleRate != 16000 {
		t.Errorf("expected sample_rate 16000, got %d", cfg.SampleRate)
	}
	if !cfg.SoundFeedback {
		t.Error("expected sound_feedback true")
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("config file not created at %s", path)
	}
}

func TestLoadConfig_ReadsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`trigger_key:
  - ctrl
  - shift
model_size: tiny
language: "en"
sample_rate: 16000
sound_feedback: false
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.TriggerKey) != 2 || cfg.TriggerKey[0] != "ctrl" || cfg.TriggerKey[1] != "shift" {
		t.Errorf("expected trigger_key [ctrl shift], got %v", cfg.TriggerKey)
	}
	if cfg.ModelSize != "tiny" {
		t.Errorf("expected model_size tiny, got %s", cfg.ModelSize)
	}
	if cfg.Language != "en" {
		t.Errorf("expected language en, got %s", cfg.Language)
	}
	if cfg.SoundFeedback {
		t.Error("expected sound_feedback false")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := Config{
		TriggerKey:    []string{"fn", "shift"},
		ModelSize:     "small",
		SampleRate:    16000,
		SoundFeedback: true,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidate_EmptyTriggerKey(t *testing.T) {
	cfg := Config{
		TriggerKey: []string{},
		ModelSize:  "small",
		SampleRate: 16000,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty trigger_key")
	}
	if !strings.Contains(err.Error(), "trigger_key") {
		t.Errorf("error should mention trigger_key, got: %v", err)
	}
}

func TestValidate_UnknownKey(t *testing.T) {
	cfg := Config{
		TriggerKey: []string{"fn", "banana"},
		ModelSize:  "small",
		SampleRate: 16000,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error should mention the bad key, got: %v", err)
	}
}

func TestValidate_InvalidModelSize(t *testing.T) {
	cfg := Config{
		TriggerKey: []string{"fn", "shift"},
		ModelSize:  "enormous",
		SampleRate: 16000,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid model_size")
	}
	if !strings.Contains(err.Error(), "model_size") {
		t.Errorf("error should mention model_size, got: %v", err)
	}
}

func TestValidate_InvalidSampleRate(t *testing.T) {
	cfg := Config{
		TriggerKey: []string{"fn", "shift"},
		ModelSize:  "small",
		SampleRate: 0,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero sample_rate")
	}
	if !strings.Contains(err.Error(), "sample_rate") {
		t.Errorf("error should mention sample_rate, got: %v", err)
	}
}

func TestValidate_InvalidTypeMode(t *testing.T) {
	cfg := Config{
		TriggerKey: []string{"fn", "shift"},
		ModelSize:  "small",
		SampleRate: 16000,
		TypeMode:   "banana",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid type_mode")
	}
	if !strings.Contains(err.Error(), "type_mode") {
		t.Errorf("error should mention type_mode, got: %v", err)
	}
}

func TestValidate_ValidTypeModes(t *testing.T) {
	for _, mode := range []string{"clipboard", "stream", ""} {
		cfg := Config{
			TriggerKey: []string{"fn", "shift"},
			ModelSize:  "small",
			SampleRate: 16000,
			TypeMode:   mode,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected type_mode %q to be valid, got error: %v", mode, err)
		}
	}
}

func TestValidate_InvalidLanguage(t *testing.T) {
	tests := []struct {
		lang    string
		wantErr bool
	}{
		{"", false},
		{"en", false},
		{"zh", false},
		{"yue", false},
		{"EN", true},          // uppercase
		{"en-US", true},       // has hyphen
		{"12", true},          // numbers
		{"abcde", true},       // too long
		{"javascript", true},  // way too long
	}
	for _, tt := range tests {
		cfg := Config{
			TriggerKey: []string{"fn", "shift"},
			ModelSize:  "small",
			SampleRate: 16000,
			Language:   tt.lang,
		}
		err := cfg.Validate()
		if tt.wantErr && err == nil {
			t.Errorf("expected error for language %q, got nil", tt.lang)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("expected no error for language %q, got: %v", tt.lang, err)
		}
	}
}

func TestValidate_ValidLanguages(t *testing.T) {
	for _, lang := range []string{"en", "zh", "ja", "ko", "es", "yue", "haw"} {
		cfg := Config{
			TriggerKey: []string{"fn"}, ModelSize: "small",
			SampleRate: 16000, Language: lang,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("language %q should be valid: %v", lang, err)
		}
	}
}

func TestValidate_InvalidLanguageCode(t *testing.T) {
	cfg := Config{
		TriggerKey: []string{"fn"}, ModelSize: "small",
		SampleRate: 16000, Language: "zzzz",
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for unsupported language code")
	}
}

func TestLoadConfig_DefaultTypeMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.TypeMode != "clipboard" {
		t.Errorf("expected default type_mode 'clipboard', got %q", cfg.TypeMode)
	}
}
