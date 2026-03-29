package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed config_default.yaml
var defaultConfigYAML []byte

type Config struct {
	TriggerKey    []string `yaml:"trigger_key"`
	ModelSize     string   `yaml:"model_size"`
	Language      string   `yaml:"language"`
	SampleRate    int      `yaml:"sample_rate"`
	SoundFeedback bool     `yaml:"sound_feedback"`
	InputDevice   string   `yaml:"input_device"`
	TypeMode      string   `yaml:"type_mode"`
}

var validModelSizes = map[string]bool{
	"tiny": true, "base": true, "small": true, "medium": true,
}

var validKeys = map[string]bool{
	"fn": true, "shift": true, "ctrl": true, "option": true, "cmd": true,
}

var validLanguages = map[string]bool{
	"en": true, "zh": true, "de": true, "es": true, "ru": true,
	"ko": true, "fr": true, "ja": true, "pt": true, "tr": true,
	"pl": true, "ca": true, "nl": true, "ar": true, "sv": true,
	"it": true, "id": true, "hi": true, "fi": true, "vi": true,
	"he": true, "uk": true, "el": true, "ms": true, "cs": true,
	"ro": true, "da": true, "hu": true, "ta": true, "no": true,
	"th": true, "ur": true, "hr": true, "bg": true, "lt": true,
	"la": true, "mi": true, "ml": true, "cy": true, "sk": true,
	"te": true, "fa": true, "lv": true, "bn": true, "sr": true,
	"az": true, "sl": true, "kn": true, "et": true, "mk": true,
	"br": true, "eu": true, "is": true, "hy": true, "ne": true,
	"mn": true, "bs": true, "kk": true, "sq": true, "sw": true,
	"gl": true, "mr": true, "pa": true, "si": true, "km": true,
	"sn": true, "yo": true, "so": true, "af": true, "oc": true,
	"ka": true, "be": true, "tg": true, "sd": true, "gu": true,
	"am": true, "yi": true, "lo": true, "uz": true, "fo": true,
	"ht": true, "ps": true, "tk": true, "nn": true, "mt": true,
	"sa": true, "lb": true, "my": true, "bo": true, "tl": true,
	"mg": true, "as": true, "tt": true, "haw": true, "ln": true,
	"ha": true, "ba": true, "jw": true, "su": true, "yue": true,
}

func LoadConfig(path string) (Config, error) {
	_, statErr := os.Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return Config{}, fmt.Errorf("config.LoadConfig: stat: %w", statErr)
	}
	if os.IsNotExist(statErr) {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return Config{}, fmt.Errorf("config.LoadConfig: create dir: %w", err)
		}
		if err := os.WriteFile(path, defaultConfigYAML, 0644); err != nil {
			return Config{}, fmt.Errorf("config.LoadConfig: write default: %w", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config.LoadConfig: read: %w", err)
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config.LoadConfig: parse: %w", err)
	}

	if cfg.TypeMode == "" {
		cfg.TypeMode = "clipboard"
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if len(c.TriggerKey) == 0 {
		return fmt.Errorf("config.Validate: trigger_key must have at least one key")
	}
	for _, k := range c.TriggerKey {
		if !validKeys[k] {
			return fmt.Errorf("config.Validate: unknown key %q in trigger_key", k)
		}
	}
	if !validModelSizes[c.ModelSize] {
		return fmt.Errorf("config.Validate: invalid model_size %q (must be tiny, base, small, or medium)", c.ModelSize)
	}
	if c.SampleRate <= 0 || c.SampleRate > 96000 {
		return fmt.Errorf("config.Validate: sample_rate must be between 1 and 96000, got %d", c.SampleRate)
	}
	validTypeModes := map[string]bool{"clipboard": true, "stream": true}
	if c.TypeMode != "" && !validTypeModes[c.TypeMode] {
		return fmt.Errorf("config.Validate: invalid type_mode %q (must be clipboard or stream)", c.TypeMode)
	}
	if c.Language != "" && !validLanguages[c.Language] {
		return fmt.Errorf("config.Validate: unsupported language %q", c.Language)
	}
	return nil
}

func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config.DefaultConfigDir: %w", err)
	}
	newDir := filepath.Join(home, "Library", "Application Support", "JoiceTyper")
	oldDir := filepath.Join(home, ".config", "voicetype")

	// Migrate from old path if it exists and new path doesn't
	if _, err := os.Stat(oldDir); err == nil {
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			if mkErr := os.MkdirAll(filepath.Dir(newDir), 0755); mkErr == nil {
				os.Rename(oldDir, newDir)
			}
		}
	}

	return newDir, nil
}

func DefaultConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", fmt.Errorf("config.DefaultConfigPath: %w", err)
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func DefaultModelPath(modelSize string) (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", fmt.Errorf("config.DefaultModelPath: %w", err)
	}
	return filepath.Join(dir, "models", "ggml-"+modelSize+".bin"), nil
}
