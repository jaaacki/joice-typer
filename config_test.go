package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	if cfg.Language != "en" {
		t.Errorf("expected default language en, got %s", cfg.Language)
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
	if cfg.DecodeMode != "beam" {
		t.Errorf("expected default decode_mode beam, got %q", cfg.DecodeMode)
	}
	if cfg.PunctuationMode != "conservative" {
		t.Errorf("expected default punctuation_mode conservative, got %q", cfg.PunctuationMode)
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

func TestValidate_InvalidDecodeMode(t *testing.T) {
	cfg := Config{
		TriggerKey:      []string{"fn", "shift"},
		ModelSize:       "small",
		SampleRate:      16000,
		DecodeMode:      "banana",
		PunctuationMode: "conservative",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid decode_mode")
	}
	if !strings.Contains(err.Error(), "decode_mode") {
		t.Errorf("error should mention decode_mode, got: %v", err)
	}
}

func TestValidate_ValidDecodeModes(t *testing.T) {
	for _, mode := range []string{"greedy", "beam", ""} {
		cfg := Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			SampleRate:      16000,
			DecodeMode:      mode,
			PunctuationMode: "conservative",
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected decode_mode %q to be valid, got error: %v", mode, err)
		}
	}
}

func TestValidate_InvalidPunctuationMode(t *testing.T) {
	cfg := Config{
		TriggerKey:      []string{"fn", "shift"},
		ModelSize:       "small",
		SampleRate:      16000,
		DecodeMode:      "beam",
		PunctuationMode: "banana",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid punctuation_mode")
	}
	if !strings.Contains(err.Error(), "punctuation_mode") {
		t.Errorf("error should mention punctuation_mode, got: %v", err)
	}
}

func TestValidate_ValidPunctuationModes(t *testing.T) {
	for _, mode := range []string{"off", "conservative", "opinionated", ""} {
		cfg := Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			SampleRate:      16000,
			DecodeMode:      "beam",
			PunctuationMode: mode,
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected punctuation_mode %q to be valid, got error: %v", mode, err)
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
		{"EN", true},         // uppercase
		{"en-US", true},      // has hyphen
		{"12", true},         // numbers
		{"abcde", true},      // too long
		{"javascript", true}, // way too long
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

func TestLoadConfig_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("trigger_key: [fn]\nmodel_size: small\nsample_rate: 16000\nbogus_field: true\n")
	os.WriteFile(path, data, 0644)
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for unknown YAML field 'bogus_field'")
	}
}

func TestAtomicWriteFile_PartialWriteCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	// Write initial valid content
	if err := atomicWriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify the .tmp file doesn't linger after successful write
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("tmp file should not exist after successful write")
	}

	// Verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Errorf("expected 'original', got %q", string(data))
	}
}

func TestLoadConfig_DefaultDecodeAndPunctuationModes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.DecodeMode != "beam" {
		t.Errorf("expected default decode_mode 'beam', got %q", cfg.DecodeMode)
	}
	if cfg.PunctuationMode != "conservative" {
		t.Errorf("expected default punctuation_mode 'conservative', got %q", cfg.PunctuationMode)
	}
	if cfg.Language != "en" {
		t.Errorf("expected default language 'en', got %q", cfg.Language)
	}
}

func TestLoadConfig_AcceptsLegacyTypeMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("trigger_key: [fn]\nmodel_size: small\nsample_rate: 16000\ntype_mode: clipboard\nlanguage: en\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected legacy type_mode to be accepted, got error: %v", err)
	}
	if cfg.Language != "en" {
		t.Fatalf("expected language to load, got %q", cfg.Language)
	}
	if cfg.DecodeMode != "beam" {
		t.Fatalf("expected default decode_mode beam, got %q", cfg.DecodeMode)
	}
	if cfg.PunctuationMode != "conservative" {
		t.Fatalf("expected default punctuation_mode conservative, got %q", cfg.PunctuationMode)
	}
}

func TestLoadConfig_DefaultsExistingEmptyLanguageToEnglish(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("trigger_key: [fn]\nmodel_size: small\nsample_rate: 16000\nlanguage: \"\"\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Language != "en" {
		t.Fatalf("expected existing empty language to default to en, got %q", cfg.Language)
	}
}

func TestConfigRoundTrip_DropsLegacyTypeModeAndPreservesNewFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("trigger_key: [fn, shift]\nmodel_size: small\nsample_rate: 16000\ntype_mode: clipboard\nlanguage: \"\"\ndecode_mode: greedy\npunctuation_mode: off\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	marshaled, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	output := string(marshaled)

	if strings.Contains(output, "type_mode") {
		t.Fatalf("expected legacy type_mode to be dropped on round-trip, got %q", output)
	}
	for _, snippet := range []string{
		"language: en",
		"decode_mode: greedy",
		"punctuation_mode: \"off\"",
	} {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected round-trip output to contain %q, got %q", snippet, output)
		}
	}
}
