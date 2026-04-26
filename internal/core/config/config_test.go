package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type stubFileInfo struct {
	name string
	size int64
	dir  bool
}

func (s stubFileInfo) Name() string       { return s.name }
func (s stubFileInfo) Size() int64        { return s.size }
func (s stubFileInfo) Mode() os.FileMode  { return 0644 }
func (s stubFileInfo) ModTime() time.Time { return time.Time{} }
func (s stubFileInfo) IsDir() bool        { return s.dir }
func (s stubFileInfo) Sys() any           { return nil }

func TestLoadConfig_CreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	originalGOOS := runtimeGOOS
	defer func() { runtimeGOOS = originalGOOS }()
	runtimeGOOS = "darwin"

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

func TestLoadConfig_CreatesWindowsDefaultTriggerKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	originalGOOS := runtimeGOOS
	defer func() { runtimeGOOS = originalGOOS }()
	runtimeGOOS = "windows"

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.TriggerKey) != 2 || cfg.TriggerKey[0] != "ctrl" || cfg.TriggerKey[1] != "shift" {
		t.Errorf("expected windows trigger_key [ctrl shift], got %v", cfg.TriggerKey)
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
		{"EN", true},
		{"en-US", true},
		{"12", true},
		{"abcde", true},
		{"javascript", true},
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

func TestSupportedHotkeyModifiersForGOOS(t *testing.T) {
	if got := SupportedHotkeyModifiersForGOOS("windows"); strings.Join(got, ",") != "shift,ctrl,option,cmd" {
		t.Fatalf("SupportedHotkeyModifiersForGOOS(windows) = %#v, want [shift ctrl option cmd]", got)
	}
	if got := SupportedHotkeyModifiersForGOOS("darwin"); strings.Join(got, ",") != "fn,shift,ctrl,option,cmd" {
		t.Fatalf("SupportedHotkeyModifiersForGOOS(darwin) = %#v, want [fn shift ctrl option cmd]", got)
	}
}

func TestDefaultTriggerKeysForGOOS(t *testing.T) {
	if got := DefaultTriggerKeysForGOOS("windows"); strings.Join(got, ",") != "ctrl,shift" {
		t.Fatalf("DefaultTriggerKeysForGOOS(windows) = %#v, want [ctrl shift]", got)
	}
	if got := DefaultTriggerKeysForGOOS("darwin"); strings.Join(got, ",") != "fn,shift" {
		t.Fatalf("DefaultTriggerKeysForGOOS(darwin) = %#v, want [fn shift]", got)
	}
}

func TestSupportedHotkeyKeys_ReturnsSharedKeyNames(t *testing.T) {
	keys := SupportedHotkeyKeys()
	if len(keys) == 0 {
		t.Fatal("expected shared hotkey keys")
	}
	for _, required := range []string{"a", "space", "return", "f12"} {
		if !slices.Contains(keys, required) {
			t.Fatalf("expected SupportedHotkeyKeys to contain %q, got %#v", required, keys)
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

func TestLoadConfig_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`trigger_key: [fn, shift]
model_size: small
sample_rate: 16000
unknown_field: true
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected unknown field to fail")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("expected error to mention unknown_field, got %v", err)
	}
}

func TestLoadConfig_MigratesLegacyTypeMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`trigger_key: [fn, shift]
model_size: small
language: en
sample_rate: 16000
sound_feedback: true
type_mode: clipboard
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LegacyTypeMode != "" {
		t.Fatalf("expected legacy type_mode to be cleared, got %q", cfg.LegacyTypeMode)
	}
}

func TestLoadConfig_MigratesLegacyTranslateFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`trigger_key: [fn, shift]
model_size: small
language: en
sample_rate: 16000
sound_feedback: true
translate: true
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.OutputMode != "translation" {
		t.Fatalf("expected output_mode translation, got %q", cfg.OutputMode)
	}
}

func TestLoadConfig_AppliesNewDefaultsToExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`trigger_key: [fn, shift]
model_size: small
sample_rate: 16000
sound_feedback: true
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DecodeMode != "beam" {
		t.Fatalf("expected decode_mode beam, got %q", cfg.DecodeMode)
	}
	if cfg.PunctuationMode != "conservative" {
		t.Fatalf("expected punctuation_mode conservative, got %q", cfg.PunctuationMode)
	}
	if cfg.Language != "en" {
		t.Fatalf("expected language en, got %q", cfg.Language)
	}
}

func TestConfigRoundTrip_DropsLegacyTypeModeAndPreservesNewFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`trigger_key: [fn, shift]
model_size: small
language: en
sample_rate: 16000
sound_feedback: true
type_mode: clipboard
decode_mode: beam
punctuation_mode: conservative
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(saved), "type_mode:") {
		t.Fatalf("expected legacy type_mode to be omitted after save, got:\n%s", saved)
	}
	if !strings.Contains(string(saved), "decode_mode: beam") {
		t.Fatalf("expected decode_mode to survive round trip, got:\n%s", saved)
	}
	if !strings.Contains(string(saved), "punctuation_mode: conservative") {
		t.Fatalf("expected punctuation_mode to survive round trip, got:\n%s", saved)
	}
}

func TestDefaultConfigDir_MigratesDarwinLegacyDotConfigDir(t *testing.T) {
	originalUserConfigDir := userConfigDir
	originalUserHomeDir := userHomeDir
	originalStatPath := statPath
	originalMkdirAll := mkdirAll
	originalRenamePath := renamePath
	originalGOOS := runtimeGOOS
	defer func() {
		userConfigDir = originalUserConfigDir
		userHomeDir = originalUserHomeDir
		statPath = originalStatPath
		mkdirAll = originalMkdirAll
		renamePath = originalRenamePath
		runtimeGOOS = originalGOOS
	}()

	homeDir := filepath.Join(string(filepath.Separator), "Users", "alice")
	configRoot := filepath.Join(homeDir, "Library", "Application Support")
	newDir := filepath.Join(configRoot, "JoiceTyper")
	oldDir := filepath.Join(homeDir, ".config", "voicetype")

	userConfigDir = func() (string, error) { return configRoot, nil }
	userHomeDir = func() (string, error) { return homeDir, nil }
	runtimeGOOS = "darwin"

	statPath = func(path string) (os.FileInfo, error) {
		switch path {
		case oldDir:
			return stubFileInfo{name: "voicetype", dir: true}, nil
		case newDir:
			return nil, os.ErrNotExist
		default:
			return nil, os.ErrNotExist
		}
	}

	var mkdirTarget string
	mkdirAll = func(path string, perm os.FileMode) error {
		mkdirTarget = path
		return nil
	}

	var renameFrom, renameTo string
	renamePath = func(oldpath, newpath string) error {
		renameFrom, renameTo = oldpath, newpath
		return nil
	}

	dir, err := DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	if dir != newDir {
		t.Fatalf("expected config dir %q, got %q", newDir, dir)
	}
	if renameFrom != oldDir || renameTo != newDir {
		t.Fatalf("expected migration rename %q -> %q, got %q -> %q", oldDir, newDir, renameFrom, renameTo)
	}
	if mkdirTarget != filepath.Dir(newDir) {
		t.Fatalf("expected migrate mkdir target %q, got %q", filepath.Dir(newDir), mkdirTarget)
	}
}

func TestDefaultConfigDir_MigratesRealDarwinLegacyTree(t *testing.T) {
	originalUserConfigDir := userConfigDir
	originalUserHomeDir := userHomeDir
	originalStatPath := statPath
	originalMkdirAll := mkdirAll
	originalRenamePath := renamePath
	originalGOOS := runtimeGOOS
	defer func() {
		userConfigDir = originalUserConfigDir
		userHomeDir = originalUserHomeDir
		statPath = originalStatPath
		mkdirAll = originalMkdirAll
		renamePath = originalRenamePath
		runtimeGOOS = originalGOOS
	}()

	homeDir := filepath.Join(t.TempDir(), "home")
	legacyDir := filepath.Join(homeDir, ".config", "voicetype")
	legacyConfigPath := filepath.Join(legacyDir, "config.yaml")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(legacyConfigPath, []byte("trigger_key: [fn, shift]\nmodel_size: small\nsample_rate: 16000\n"), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	configRoot := filepath.Join(homeDir, "Library", "Application Support")
	userConfigDir = func() (string, error) { return configRoot, nil }
	userHomeDir = func() (string, error) { return homeDir, nil }
	statPath = os.Stat
	mkdirAll = os.MkdirAll
	renamePath = os.Rename
	runtimeGOOS = "darwin"

	gotDir, err := DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	wantDir := filepath.Join(configRoot, "JoiceTyper")
	if gotDir != wantDir {
		t.Fatalf("expected migrated dir %q, got %q", wantDir, gotDir)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "config.yaml")); err != nil {
		t.Fatalf("expected migrated config at new path, got %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("expected legacy dir to be moved away, stat err=%v", err)
	}
}
