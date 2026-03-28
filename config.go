package main

import (
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
}

var validModelSizes = map[string]bool{
	"tiny": true, "base": true, "small": true, "medium": true,
}

var validKeys = map[string]bool{
	"fn": true, "shift": true, "ctrl": true, "option": true, "cmd": true,
	"space": true,
	"a": true, "b": true, "c": true, "d": true, "e": true, "f": true,
	"g": true, "h": true, "i": true, "j": true, "k": true, "l": true,
	"m": true, "n": true, "o": true, "p": true, "q": true, "r": true,
	"s": true, "t": true, "u": true, "v": true, "w": true, "x": true,
	"y": true, "z": true,
	"f1": true, "f2": true, "f3": true, "f4": true, "f5": true, "f6": true,
	"f7": true, "f8": true, "f9": true, "f10": true, "f11": true, "f12": true,
}

func LoadConfig(path string) (Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config.LoadConfig: parse: %w", err)
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
	if c.SampleRate <= 0 {
		return fmt.Errorf("config.Validate: sample_rate must be positive, got %d", c.SampleRate)
	}
	return nil
}

func DefaultConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "voicetype")
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

func DefaultModelPath(modelSize string) string {
	return filepath.Join(DefaultConfigDir(), "models", "ggml-"+modelSize+".bin")
}
