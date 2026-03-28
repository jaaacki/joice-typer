# VoiceType Go v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local, offline push-to-talk voice-to-text macOS tool as a single Go binary using whisper.cpp with Metal GPU acceleration.

**Architecture:** Five components (config, hotkey, recorder, transcriber, paster) wired by an App orchestrator. CGEvent tap for global hotkey detection (supports Fn key). Direct cgo bindings to whisper.cpp C API. PortAudio for audio capture. Structured slog JSON logging throughout. macOS Objective-C code in separate `.m` files for clean cgo integration.

**Tech Stack:** Go 1.22+, whisper.cpp (cgo, Metal), PortAudio (cgo via gordonklaus/portaudio), macOS CoreGraphics/AppKit (cgo + Objective-C), log/slog, gopkg.in/yaml.v3

---

## File Structure

```
~/voicetype/
├── go.mod                   # module: voicetype
├── go.sum
├── Makefile                 # builds whisper.cpp, downloads model, builds Go binary
├── .gitignore
├── main.go                  # entry point, flag parsing, signal handling, wiring
├── app.go                   # App struct, event loop orchestrator
├── app_test.go              # mock-based orchestration tests
├── config.go                # Config struct, LoadConfig, Validate, defaults, embed
├── config_test.go           # config loading and validation tests
├── config_default.yaml      # embedded default config
├── logger.go                # slog JSON setup, file + stderr, truncation
├── logger_test.go           # logger setup and truncation tests
├── sound.go                 # async system sound playback via afplay
├── paster.go                # Paster interface + cgo implementation
├── paster_darwin.h          # C declarations for clipboard + key simulation
├── paster_darwin.m          # Objective-C: NSPasteboard + CGEvent Cmd+V
├── recorder.go              # Recorder interface + PortAudio implementation
├── transcriber.go           # Transcriber interface + whisper.cpp cgo wrapper + model download
├── hotkey.go                # HotkeyListener interface + cgo wrapper + key mapping
├── hotkey_darwin.h          # C declarations for CGEvent tap
├── hotkey_darwin.m          # CGEvent tap implementation for modifier detection
├── third_party/
│   └── whisper.cpp/         # git submodule
└── CLAUDE.md                # updated project instructions
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `.gitignore`
- Create: `go.mod`
- Create: `Makefile`
- Create: `third_party/whisper.cpp/` (git submodule)

- [ ] **Step 1: Initialize git repository**

```bash
cd ~/voicetype
git init
```

- [ ] **Step 2: Create .gitignore**

Create `.gitignore`:

```gitignore
# Binary
voicetype

# Build artifacts
*.o
*.a
*.dylib

# whisper.cpp build directory
third_party/whisper.cpp/build/

# Model files (large binaries)
*.bin

# macOS
.DS_Store

# IDE
.idea/
.vscode/
*.swp

# Go
vendor/

# Python (legacy, being removed)
.venv/
__pycache__/
*.pyc
```

- [ ] **Step 3: Remove legacy Python files**

```bash
cd ~/voicetype
rm -f voicetype.py requirements.txt setup.sh
rm -rf .venv
```

- [ ] **Step 4: Initialize Go module**

```bash
cd ~/voicetype
go mod init voicetype
```

- [ ] **Step 5: Add whisper.cpp as git submodule**

```bash
cd ~/voicetype
git submodule add https://github.com/ggerganov/whisper.cpp.git third_party/whisper.cpp
```

- [ ] **Step 6: Create Makefile**

Create `Makefile`:

```makefile
.PHONY: all setup build clean download-model whisper

WHISPER_DIR := third_party/whisper.cpp
WHISPER_BUILD := $(WHISPER_DIR)/build
MODEL_DIR := $(HOME)/.config/voicetype/models
MODEL_FILE := $(MODEL_DIR)/ggml-small.bin
MODEL_URL := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin

all: whisper build

setup:
	brew install portaudio cmake

whisper: $(WHISPER_BUILD)/src/libwhisper.a

$(WHISPER_BUILD)/src/libwhisper.a:
	cd $(WHISPER_DIR) && cmake -B build \
		-DWHISPER_METAL=ON \
		-DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DCMAKE_BUILD_TYPE=Release
	cd $(WHISPER_DIR) && cmake --build build --config Release -j$$(sysctl -n hw.ncpu)

build: whisper
	CGO_ENABLED=1 go build -o voicetype .

download-model: $(MODEL_FILE)

$(MODEL_FILE):
	mkdir -p $(MODEL_DIR)
	curl -L --progress-bar -o $(MODEL_FILE) $(MODEL_URL)

clean:
	rm -f voicetype
	rm -rf $(WHISPER_BUILD)

test:
	go test -v -count=1 ./...
```

- [ ] **Step 7: Install system dependencies**

```bash
make setup
```

Expected: `portaudio` and `cmake` installed via Homebrew.

- [ ] **Step 8: Build whisper.cpp with Metal**

```bash
cd ~/voicetype
make whisper
```

Expected: `third_party/whisper.cpp/build/src/libwhisper.a` exists. Build logs show Metal support enabled.

- [ ] **Step 9: Verify whisper.cpp library exists**

```bash
ls -la third_party/whisper.cpp/build/src/libwhisper.a
```

Expected: File exists. If the path differs (e.g., `build/libwhisper.a` instead of `build/src/libwhisper.a`), update the Makefile and note the correct path for use in Task 7's cgo directives.

- [ ] **Step 10: Download whisper small model**

```bash
make download-model
```

Expected: `~/.config/voicetype/models/ggml-small.bin` exists (~466MB).

- [ ] **Step 11: Commit**

```bash
cd ~/voicetype
git add .gitignore go.mod Makefile third_party/whisper.cpp
git commit -m "feat: project scaffolding with whisper.cpp submodule and Makefile"
```

---

### Task 2: Logger

**Files:**
- Create: `logger.go`
- Create: `logger_test.go`

- [ ] **Step 1: Write failing test for SetupLogger**

Create `logger_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupLogger_CreatesLogFile(t *testing.T) {
	dir := t.TempDir()

	logger, cleanup, err := SetupLogger(dir)
	if err != nil {
		t.Fatalf("SetupLogger: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("SetupLogger returned nil logger")
	}

	logPath := filepath.Join(dir, "voicetype.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("log file not created at %s", logPath)
	}
}

func TestSetupLogger_WritesToFile(t *testing.T) {
	dir := t.TempDir()

	logger, cleanup, err := SetupLogger(dir)
	if err != nil {
		t.Fatalf("SetupLogger: %v", err)
	}
	defer cleanup()

	logger.Info("test message", "component", "test", "operation", "TestWrite")

	logPath := filepath.Join(dir, "voicetype.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("log file is empty after writing")
	}
	if !contains(content, "test message") {
		t.Errorf("log file missing message, got: %s", content)
	}
	if !contains(content, `"component"`) {
		t.Errorf("log file missing component field, got: %s", content)
	}
}

func TestTruncateIfNeeded_TruncatesLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Create a file larger than 1KB (using small limit for testing)
	data := make([]byte, 2048)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := truncateIfNeeded(path, 1024); err != nil {
		t.Fatalf("truncateIfNeeded: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("expected truncated file to be 0 bytes, got %d", info.Size())
	}
}

func TestTruncateIfNeeded_LeavesSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	data := []byte("small content")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := truncateIfNeeded(path, 1024); err != nil {
		t.Fatalf("truncateIfNeeded: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(data)) {
		t.Errorf("expected file size %d, got %d", len(data), info.Size())
	}
}

func TestTruncateIfNeeded_NonexistentFile(t *testing.T) {
	err := truncateIfNeeded("/nonexistent/path/file.log", 1024)
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/voicetype
go test -v -run TestSetupLogger -count=1
```

Expected: FAIL — `SetupLogger` not defined.

- [ ] **Step 3: Implement logger**

Create `logger.go`:

```go
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const maxLogBytes int64 = 5 * 1024 * 1024 // 5MB

func SetupLogger(logDir string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: create dir: %w", err)
	}

	logPath := filepath.Join(logDir, "voicetype.log")

	if err := truncateIfNeeded(logPath, maxLogBytes); err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: truncate: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: open log file: %w", err)
	}

	writer := io.MultiWriter(os.Stderr, f)
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := slog.New(handler)
	cleanup := func() {
		f.Close()
	}

	return logger, cleanup, nil
}

func truncateIfNeeded(path string, maxBytes int64) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("logger.truncateIfNeeded: stat: %w", err)
	}
	if info.Size() > maxBytes {
		if err := os.Truncate(path, 0); err != nil {
			return fmt.Errorf("logger.truncateIfNeeded: truncate: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ~/voicetype
go test -v -run "TestSetupLogger|TestTruncate" -count=1
```

Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/voicetype
git add logger.go logger_test.go
git commit -m "feat: structured slog JSON logger with file rotation"
```

---

### Task 3: Config

**Files:**
- Create: `config_default.yaml`
- Create: `config.go`
- Create: `config_test.go`

- [ ] **Step 1: Create default config YAML**

Create `config_default.yaml`:

```yaml
# VoiceType configuration
trigger_key:
  - fn
  - shift
model_size: small
language: ""
sample_rate: 16000
sound_feedback: true
```

- [ ] **Step 2: Write failing tests for config**

Create `config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
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
  - space
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

	if len(cfg.TriggerKey) != 2 || cfg.TriggerKey[0] != "ctrl" || cfg.TriggerKey[1] != "space" {
		t.Errorf("expected trigger_key [ctrl space], got %v", cfg.TriggerKey)
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
	if !containsSubstring(err.Error(), "trigger_key") {
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
	if !containsSubstring(err.Error(), "banana") {
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
	if !containsSubstring(err.Error(), "model_size") {
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
	if !containsSubstring(err.Error(), "sample_rate") {
		t.Errorf("error should mention sample_rate, got: %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd ~/voicetype
go test -v -run "TestLoadConfig|TestValidate" -count=1
```

Expected: FAIL — `Config`, `LoadConfig` not defined.

- [ ] **Step 4: Implement config**

Create `config.go`:

```go
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
```

- [ ] **Step 5: Add yaml dependency**

```bash
cd ~/voicetype
go get gopkg.in/yaml.v3
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd ~/voicetype
go test -v -run "TestLoadConfig|TestValidate" -count=1
```

Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
cd ~/voicetype
git add config.go config_test.go config_default.yaml go.mod go.sum
git commit -m "feat: YAML config with validation and embedded defaults"
```

---

### Task 4: Sound

**Files:**
- Create: `sound.go`

- [ ] **Step 1: Implement async sound playback**

Create `sound.go`:

```go
package main

import (
	"log/slog"
	"os/exec"
)

type Sound struct {
	enabled bool
	logger  *slog.Logger
}

func NewSound(enabled bool, logger *slog.Logger) *Sound {
	return &Sound{
		enabled: enabled,
		logger:  logger.With("component", "sound"),
	}
}

func (s *Sound) Play(name string) {
	if !s.enabled {
		return
	}
	go func() {
		path := "/System/Library/Sounds/" + name + ".aiff"
		cmd := exec.Command("afplay", path)
		if err := cmd.Run(); err != nil {
			s.logger.Error("failed to play sound",
				"operation", "Play",
				"sound", name,
				"error", err,
			)
		}
	}()
}

func (s *Sound) PlayStart() {
	s.Play("Tink")
}

func (s *Sound) PlayStop() {
	s.Play("Pop")
}

func (s *Sound) PlayError() {
	s.Play("Basso")
}

func (s *Sound) PlayReady() {
	s.Play("Glass")
}
```

- [ ] **Step 2: Verify build**

```bash
cd ~/voicetype
go build ./...
```

Expected: Builds without errors. (Note: this may fail until all referenced types exist. If so, temporarily add a `// +build ignore` tag or just verify the file has no syntax errors with `go vet`.)

```bash
cd ~/voicetype
go vet ./sound.go
```

- [ ] **Step 3: Commit**

```bash
cd ~/voicetype
git add sound.go
git commit -m "feat: async system sound playback"
```

---

### Task 5: Paster

**Files:**
- Create: `paster_darwin.h`
- Create: `paster_darwin.m`
- Create: `paster.go`

- [ ] **Step 1: Create C header for paster**

Create `paster_darwin.h`:

```c
#ifndef PASTER_DARWIN_H
#define PASTER_DARWIN_H

// setClipboard copies text to the macOS general pasteboard.
// Returns 0 on success, non-zero on failure.
int setClipboard(const char* text);

// simulateCmdV simulates pressing Cmd+V to paste from clipboard.
void simulateCmdV(void);

#endif
```

- [ ] **Step 2: Implement Objective-C paster**

Create `paster_darwin.m`:

```objc
#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include "paster_darwin.h"

int setClipboard(const char* text) {
    @autoreleasepool {
        NSPasteboard* pb = [NSPasteboard generalPasteboard];
        [pb clearContents];
        NSString* str = [NSString stringWithUTF8String:text];
        if (str == nil) {
            return 1;
        }
        BOOL ok = [pb setString:str forType:NSPasteboardTypeString];
        return ok ? 0 : 1;
    }
}

void simulateCmdV(void) {
    // 'v' key = keycode 0x09
    CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 0x09, true);
    CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);
    CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, 0x09, false);
    CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

    CGEventPost(kCGHIDEventTap, keyDown);
    CGEventPost(kCGHIDEventTap, keyUp);

    CFRelease(keyDown);
    CFRelease(keyUp);
}
```

- [ ] **Step 3: Implement Go paster**

Create `paster.go`:

```go
package main

/*
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics
#include "paster_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"log/slog"
	"time"
	"unsafe"
)

type Paster interface {
	Paste(text string) error
}

type clipboardPaster struct {
	logger *slog.Logger
}

func NewPaster(logger *slog.Logger) Paster {
	return &clipboardPaster{
		logger: logger.With("component", "paster"),
	}
}

func (p *clipboardPaster) Paste(text string) error {
	p.logger.Info("pasting", "operation", "Paste", "text_length", len(text))

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	result := C.setClipboard(cText)
	if result != 0 {
		return fmt.Errorf("paster.Paste: failed to set clipboard")
	}

	// Brief pause to let pasteboard settle before simulating keypress
	time.Sleep(50 * time.Millisecond)

	C.simulateCmdV()

	p.logger.Info("pasted", "operation", "Paste")
	return nil
}
```

- [ ] **Step 4: Verify build**

```bash
cd ~/voicetype
go build -o /dev/null ./paster.go ./paster_darwin.h ./paster_darwin.m 2>&1 || echo "Build check — see errors above"
```

Note: This won't compile standalone because it depends on the Paster interface and logger. Run a full package build check instead:

```bash
cd ~/voicetype
go vet .
```

If vet fails due to missing types from other files that don't exist yet, that's expected. Verify there are no syntax errors in the three files by reading through them.

- [ ] **Step 5: Commit**

```bash
cd ~/voicetype
git add paster.go paster_darwin.h paster_darwin.m
git commit -m "feat: clipboard paste via NSPasteboard + CGEvent Cmd+V"
```

---

### Task 6: Recorder

**Files:**
- Create: `recorder.go`

- [ ] **Step 1: Add portaudio dependency**

```bash
cd ~/voicetype
go get github.com/gordonklaus/portaudio
```

- [ ] **Step 2: Implement recorder**

Create `recorder.go`:

```go
package main

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/gordonklaus/portaudio"
)

type Recorder interface {
	Start() error
	Stop() ([]float32, error)
	Close() error
}

type portaudioRecorder struct {
	sampleRate float64
	stream     *portaudio.Stream
	buffer     []float32
	chunks     [][]float32
	mu         sync.Mutex
	recording  bool
	done       chan struct{}
	logger     *slog.Logger
}

func NewRecorder(sampleRate int, logger *slog.Logger) Recorder {
	return &portaudioRecorder{
		sampleRate: float64(sampleRate),
		logger:     logger.With("component", "recorder"),
	}
}

func InitAudio() error {
	return portaudio.Initialize()
}

func TerminateAudio() error {
	return portaudio.Terminate()
}

func (r *portaudioRecorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("recorder.Start: already recording")
	}

	r.logger.Info("starting", "operation", "Start", "sample_rate", r.sampleRate)

	r.chunks = nil
	r.recording = true
	r.done = make(chan struct{})
	r.buffer = make([]float32, 1024)

	stream, err := portaudio.OpenDefaultStream(1, 0, r.sampleRate, len(r.buffer), r.buffer)
	if err != nil {
		r.recording = false
		return fmt.Errorf("recorder.Start: open stream: %w", err)
	}
	r.stream = stream

	if err := stream.Start(); err != nil {
		r.recording = false
		if closeErr := stream.Close(); closeErr != nil {
			r.logger.Error("failed to close stream after start error",
				"operation", "Start", "error", closeErr)
		}
		return fmt.Errorf("recorder.Start: start stream: %w", err)
	}

	// Read audio in a goroutine
	go r.readLoop()

	r.logger.Info("recording started", "operation", "Start")
	return nil
}

func (r *portaudioRecorder) readLoop() {
	defer close(r.done)
	for {
		if err := r.stream.Read(); err != nil {
			r.mu.Lock()
			isRecording := r.recording
			r.mu.Unlock()
			if !isRecording {
				return // expected: stream was stopped
			}
			r.logger.Error("read error", "operation", "readLoop", "error", err)
			return
		}

		r.mu.Lock()
		if !r.recording {
			r.mu.Unlock()
			return
		}
		chunk := make([]float32, len(r.buffer))
		copy(chunk, r.buffer)
		r.chunks = append(r.chunks, chunk)
		r.mu.Unlock()
	}
}

func (r *portaudioRecorder) Stop() ([]float32, error) {
	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return nil, fmt.Errorf("recorder.Stop: not recording")
	}
	r.recording = false
	r.mu.Unlock()

	r.logger.Info("stopping", "operation", "Stop")

	if err := r.stream.Stop(); err != nil {
		r.logger.Error("failed to stop stream", "operation", "Stop", "error", err)
	}

	// Wait for read goroutine to exit
	<-r.done

	if err := r.stream.Close(); err != nil {
		r.logger.Error("failed to close stream", "operation", "Stop", "error", err)
	}
	r.stream = nil

	r.mu.Lock()
	chunks := r.chunks
	r.chunks = nil
	r.mu.Unlock()

	// Flatten chunks
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}

	if total == 0 {
		r.logger.Warn("no audio captured", "operation", "Stop")
		return nil, nil
	}

	audio := make([]float32, 0, total)
	for _, chunk := range chunks {
		audio = append(audio, chunk...)
	}

	r.logger.Info("recording stopped", "operation", "Stop", "samples", len(audio),
		"duration_sec", float64(len(audio))/r.sampleRate)
	return audio, nil
}

func (r *portaudioRecorder) Close() error {
	r.logger.Info("closing", "operation", "Close")
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stream != nil {
		if err := r.stream.Close(); err != nil {
			return fmt.Errorf("recorder.Close: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 3: Verify build compiles with portaudio**

```bash
cd ~/voicetype
go vet .
```

Expected: No errors. If portaudio headers can't be found, verify `brew install portaudio` was completed and try:

```bash
pkg-config --cflags --libs portaudio-2.0
```

- [ ] **Step 4: Commit**

```bash
cd ~/voicetype
git add recorder.go go.mod go.sum
git commit -m "feat: PortAudio recorder with chunked audio capture"
```

---

### Task 7: Transcriber

**Files:**
- Create: `transcriber.go`

**Important:** This task depends on whisper.cpp being built (Task 1, Step 8). Verify `third_party/whisper.cpp/build/src/libwhisper.a` exists before starting. If the library is at a different path (e.g., `build/libwhisper.a`), adjust the `CGO_LDFLAGS` path in the code below accordingly.

- [ ] **Step 1: Locate whisper.cpp build artifacts**

```bash
find ~/voicetype/third_party/whisper.cpp/build -name "libwhisper*" -type f
find ~/voicetype/third_party/whisper.cpp -name "whisper.h" -path "*/include/*" -o -name "whisper.h" -not -path "*/build/*" | head -5
```

Record the exact paths. The code below assumes:
- Library: `third_party/whisper.cpp/build/src/libwhisper.a`
- Header: `third_party/whisper.cpp/include/whisper.h`

Adjust the `#cgo` directives if your paths differ.

- [ ] **Step 2: Implement transcriber with model download**

Create `transcriber.go`:

```go
package main

/*
#cgo CFLAGS: -I${SRCDIR}/third_party/whisper.cpp/include
#cgo LDFLAGS: -L${SRCDIR}/third_party/whisper.cpp/build/src -lwhisper -lstdc++ -framework Accelerate -framework Metal -framework Foundation -framework CoreML
#include <whisper.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

type Transcriber interface {
	Transcribe(audio []float32) (string, error)
	Close() error
}

type whisperTranscriber struct {
	ctx    *C.struct_whisper_context
	lang   string
	logger *slog.Logger
}

func NewTranscriber(modelPath string, language string, logger *slog.Logger) (Transcriber, error) {
	l := logger.With("component", "transcriber")

	if err := ensureModel(modelPath, l); err != nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: %w", err)
	}

	l.Info("loading model", "operation", "NewTranscriber", "model_path", modelPath)

	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	ctx := C.whisper_init_from_file(cPath)
	if ctx == nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: failed to load model from %s", modelPath)
	}

	l.Info("model loaded", "operation", "NewTranscriber")
	return &whisperTranscriber{ctx: ctx, lang: language, logger: l}, nil
}

func (t *whisperTranscriber) Transcribe(audio []float32) (string, error) {
	t.logger.Info("transcribing", "operation", "Transcribe", "samples", len(audio))

	params := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
	params.print_progress = C._Bool(false)
	params.print_timestamps = C._Bool(false)
	params.print_realtime = C._Bool(false)
	params.print_special = C._Bool(false)
	params.single_segment = C._Bool(false)

	if t.lang != "" {
		cLang := C.CString(t.lang)
		defer C.free(unsafe.Pointer(cLang))
		params.language = cLang
	}

	result := C.whisper_full(t.ctx, params, (*C.float)(unsafe.Pointer(&audio[0])), C.int(len(audio)))
	if result != 0 {
		return "", fmt.Errorf("transcriber.Transcribe: whisper_full returned %d", result)
	}

	nSegments := int(C.whisper_full_n_segments(t.ctx))
	var segments []string
	for i := 0; i < nSegments; i++ {
		text := C.GoString(C.whisper_full_get_segment_text(t.ctx, C.int(i)))
		segments = append(segments, text)
	}

	text := strings.TrimSpace(strings.Join(segments, ""))
	t.logger.Info("transcribed", "operation", "Transcribe",
		"segments", nSegments, "text_length", len(text))
	return text, nil
}

func (t *whisperTranscriber) Close() error {
	t.logger.Info("closing", "operation", "Close")
	if t.ctx != nil {
		C.whisper_free(t.ctx)
		t.ctx = nil
	}
	return nil
}

// ensureModel checks if the model file exists and downloads it if not.
func ensureModel(modelPath string, logger *slog.Logger) error {
	if _, err := os.Stat(modelPath); err == nil {
		logger.Info("model found", "operation", "ensureModel", "path", modelPath)
		return nil
	}

	// Extract model name from path (e.g., "ggml-small.bin" from the full path)
	modelFile := filepath.Base(modelPath)
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + modelFile

	logger.Info("model not found, downloading",
		"operation", "ensureModel",
		"url", url,
		"dest", modelPath,
	)

	dir := filepath.Dir(modelPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("ensureModel: create dir: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("ensureModel: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ensureModel: download returned status %d", resp.StatusCode)
	}

	tmpPath := modelPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("ensureModel: create file: %w", err)
	}

	written, err := io.Copy(f, &progressWriter{
		writer: f,
		reader: resp.Body,
		total:  resp.ContentLength,
		logger: logger,
	})
	if closeErr := f.Close(); closeErr != nil {
		return fmt.Errorf("ensureModel: close file: %w", closeErr)
	}
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ensureModel: write: %w", err)
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		return fmt.Errorf("ensureModel: rename: %w", err)
	}

	logger.Info("model downloaded", "operation", "ensureModel", "bytes", written)
	return nil
}

type progressWriter struct {
	writer  io.Writer
	reader  io.Reader
	total   int64
	written int64
	logger  *slog.Logger
	lastPct int
}

func (pw *progressWriter) Read(p []byte) (int, error) {
	n, err := pw.reader.Read(p)
	pw.written += int64(n)
	if pw.total > 0 {
		pct := int(pw.written * 100 / pw.total)
		if pct/10 > pw.lastPct/10 {
			pw.logger.Info("downloading model",
				"operation", "ensureModel",
				"progress_pct", pct,
				"bytes_written", pw.written,
				"bytes_total", pw.total,
			)
			pw.lastPct = pct
		}
	}
	return n, err
}
```

**Note:** The `#cgo LDFLAGS` line may need adjustment based on Step 1 findings. Common variations:
- `-L${SRCDIR}/third_party/whisper.cpp/build` (if library is in `build/` not `build/src/`)
- Additional `-framework` flags if the whisper.cpp build links against other frameworks
- The `C._Bool` syntax may need to be `C.bool` depending on Go/cgo version — if build fails, swap to `C.bool(false)`

- [ ] **Step 3: Fix the progressWriter usage**

The `ensureModel` function creates a `progressWriter` but uses it incorrectly — it should use `io.Copy` with the reader, not wrap both. Fix the download section:

Replace the `io.Copy` call in `ensureModel` with:

```go
	pr := &progressWriter{
		reader: resp.Body,
		total:  resp.ContentLength,
		logger: logger,
	}

	written, err := io.Copy(f, pr)
```

And remove the `writer` field from `progressWriter`:

```go
type progressWriter struct {
	reader  io.Reader
	total   int64
	written int64
	logger  *slog.Logger
	lastPct int
}
```

- [ ] **Step 4: Verify build**

```bash
cd ~/voicetype
CGO_ENABLED=1 go build -o /dev/null . 2>&1 | head -50
```

If this fails with linker errors about whisper symbols, check:
1. The `-L` path in `#cgo LDFLAGS` matches where `libwhisper.a` actually lives
2. The `-I` path in `#cgo CFLAGS` matches where `whisper.h` actually lives
3. All required frameworks are linked

If it fails with `C._Bool` errors, change to `C.bool(false)` in the params setup.

- [ ] **Step 5: Commit**

```bash
cd ~/voicetype
git add transcriber.go go.mod go.sum
git commit -m "feat: whisper.cpp transcriber with Metal GPU and auto model download"
```

---

### Task 8: Hotkey Listener

**Files:**
- Create: `hotkey_darwin.h`
- Create: `hotkey_darwin.m`
- Create: `hotkey.go`

**macOS requirement:** The Fn key is detected via `CGEventTap` monitoring `kCGEventFlagsChanged`. This requires **Accessibility permissions** (System Settings → Privacy & Security → Accessibility). The app will need to be granted this permission on first run.

- [ ] **Step 1: Create C header for hotkey**

Create `hotkey_darwin.h`:

```c
#ifndef HOTKEY_DARWIN_H
#define HOTKEY_DARWIN_H

#include <stdint.h>

// startHotkeyListener creates a CGEvent tap monitoring modifier flags.
// targetFlags is the bitmask of modifier flags that must all be held to trigger.
// Returns 0 on success, -1 if event tap creation fails (no Accessibility permission).
int startHotkeyListener(uint64_t targetFlags);

// stopHotkeyListener disables the event tap and releases resources.
void stopHotkeyListener(void);

// runMainLoop runs CFRunLoopRun on the current thread. Blocks until stopMainLoop is called.
void runMainLoop(void);

// stopMainLoop stops the CFRunLoop started by runMainLoop.
void stopMainLoop(void);

#endif
```

- [ ] **Step 2: Implement CGEvent tap in Objective-C**

Create `hotkey_darwin.m`:

```objc
#include "hotkey_darwin.h"
#import <CoreGraphics/CoreGraphics.h>
#import <Carbon/Carbon.h>

// Defined in hotkey.go via //export
extern void hotkeyCallback(int eventType);

static uint64_t sTargetFlags = 0;
static int sTriggered = 0;
static CFMachPortRef sEventTap = NULL;
static CFRunLoopSourceRef sRunLoopSource = NULL;

static CGEventRef eventTapCallback(
    CGEventTapProxy proxy,
    CGEventType type,
    CGEventRef event,
    void *userInfo
) {
    // Re-enable tap if it gets disabled by the system (timeout)
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (sEventTap != NULL) {
            CGEventTapEnable(sEventTap, true);
        }
        return event;
    }

    if (type != kCGEventFlagsChanged) {
        return event;
    }

    uint64_t flags = CGEventGetFlags(event);
    int allHeld = (flags & sTargetFlags) == sTargetFlags;

    if (allHeld && !sTriggered) {
        sTriggered = 1;
        hotkeyCallback(0);  // TriggerPressed
    } else if (!allHeld && sTriggered) {
        sTriggered = 0;
        hotkeyCallback(1);  // TriggerReleased
    }

    return event;
}

int startHotkeyListener(uint64_t targetFlags) {
    sTargetFlags = targetFlags;
    sTriggered = 0;

    CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged);

    sEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        eventTapCallback,
        NULL
    );

    if (sEventTap == NULL) {
        return -1;  // Accessibility permission not granted
    }

    sRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, sEventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetMain(), sRunLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(sEventTap, true);

    return 0;
}

void stopHotkeyListener(void) {
    if (sEventTap != NULL) {
        CGEventTapEnable(sEventTap, false);
        CFRelease(sEventTap);
        sEventTap = NULL;
    }
    if (sRunLoopSource != NULL) {
        CFRelease(sRunLoopSource);
        sRunLoopSource = NULL;
    }
}

void runMainLoop(void) {
    CFRunLoopRun();
}

void stopMainLoop(void) {
    CFRunLoopStop(CFRunLoopGetMain());
}
```

- [ ] **Step 3: Implement Go hotkey wrapper**

Create `hotkey.go`:

```go
package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework Carbon
#include "hotkey_darwin.h"
*/
import "C"

import (
	"fmt"
	"log/slog"
)

type HotkeyEvent int

const (
	TriggerPressed HotkeyEvent = iota
	TriggerReleased
)

func (e HotkeyEvent) String() string {
	switch e {
	case TriggerPressed:
		return "TriggerPressed"
	case TriggerReleased:
		return "TriggerReleased"
	default:
		return fmt.Sprintf("HotkeyEvent(%d)", int(e))
	}
}

type HotkeyListener interface {
	Start(events chan<- HotkeyEvent) error
	Stop() error
}

// Package-level channel for the cgo callback to send events.
// Set by Start(), read by the exported callback.
var hotkeyEvents chan<- HotkeyEvent

//export hotkeyCallback
func hotkeyCallback(eventType C.int) {
	if hotkeyEvents == nil {
		return
	}
	switch int(eventType) {
	case 0:
		hotkeyEvents <- TriggerPressed
	case 1:
		hotkeyEvents <- TriggerReleased
	}
}

// macOS CGEvent modifier flag constants
const (
	flagFn     uint64 = 0x800000  // NX_SECONDARYFN / kCGEventFlagMaskSecondaryFn
	flagShift  uint64 = 0x20000   // kCGEventFlagMaskShift
	flagCtrl   uint64 = 0x40000   // kCGEventFlagMaskControl
	flagOption uint64 = 0x80000   // kCGEventFlagMaskAlternate
	flagCmd    uint64 = 0x100000  // kCGEventFlagMaskCommand
)

var keyToFlag = map[string]uint64{
	"fn":     flagFn,
	"shift":  flagShift,
	"ctrl":   flagCtrl,
	"option": flagOption,
	"cmd":    flagCmd,
}

type cgEventHotkeyListener struct {
	triggerKeys []string
	logger      *slog.Logger
}

func NewHotkeyListener(triggerKeys []string, logger *slog.Logger) HotkeyListener {
	return &cgEventHotkeyListener{
		triggerKeys: triggerKeys,
		logger:      logger.With("component", "hotkey"),
	}
}

func (h *cgEventHotkeyListener) Start(events chan<- HotkeyEvent) error {
	h.logger.Info("starting", "operation", "Start", "trigger_keys", h.triggerKeys)

	flags, err := keysToFlags(h.triggerKeys)
	if err != nil {
		return fmt.Errorf("hotkey.Start: %w", err)
	}

	hotkeyEvents = events

	result := C.startHotkeyListener(C.uint64_t(flags))
	if result != 0 {
		return fmt.Errorf("hotkey.Start: failed to create event tap — grant Accessibility permission in System Settings → Privacy & Security → Accessibility")
	}

	h.logger.Info("listening", "operation", "Start", "flags", fmt.Sprintf("0x%x", flags))

	// Blocks — runs the CFRunLoop on the calling (main) thread
	C.runMainLoop()

	return nil
}

func (h *cgEventHotkeyListener) Stop() error {
	h.logger.Info("stopping", "operation", "Stop")
	C.stopHotkeyListener()
	C.stopMainLoop()
	return nil
}

func keysToFlags(keys []string) (uint64, error) {
	var flags uint64
	for _, k := range keys {
		f, ok := keyToFlag[k]
		if !ok {
			return 0, fmt.Errorf("keysToFlags: key %q is not a modifier (only fn, shift, ctrl, option, cmd are supported as trigger keys)", k)
		}
		flags |= f
	}
	return flags, nil
}
```

**Note on trigger keys:** The CGEvent tap approach monitors modifier flags only. This means only modifier keys (`fn`, `shift`, `ctrl`, `option`, `cmd`) can be used as trigger keys in v1. Regular keys (`space`, `a`, etc.) would require a different event monitoring approach. The config validation in `config.go` allows regular keys but `keysToFlags` will return an error at startup if a non-modifier is used. This is acceptable for v1 — document it.

- [ ] **Step 4: Verify build**

```bash
cd ~/voicetype
go vet .
```

Expected: No errors.

- [ ] **Step 5: Commit**

```bash
cd ~/voicetype
git add hotkey.go hotkey_darwin.h hotkey_darwin.m
git commit -m "feat: global hotkey via CGEvent tap with Fn key support"
```

---

### Task 9: App Orchestrator

**Files:**
- Create: `app.go`
- Create: `app_test.go`

- [ ] **Step 1: Write failing test for happy path**

Create `app_test.go`:

```go
package main

import (
	"log/slog"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockRecorder struct {
	startCalled bool
	stopCalled  bool
	closeCalled bool
	audio       []float32
	startErr    error
	stopErr     error
}

func (m *mockRecorder) Start() error {
	m.startCalled = true
	return m.startErr
}
func (m *mockRecorder) Stop() ([]float32, error) {
	m.stopCalled = true
	return m.audio, m.stopErr
}
func (m *mockRecorder) Close() error {
	m.closeCalled = true
	return nil
}

type mockTranscriber struct {
	text         string
	err          error
	closeCalled  bool
	receivedAudio []float32
}

func (m *mockTranscriber) Transcribe(audio []float32) (string, error) {
	m.receivedAudio = audio
	return m.text, m.err
}
func (m *mockTranscriber) Close() error {
	m.closeCalled = true
	return nil
}

type mockPaster struct {
	pastedText string
	err        error
}

func (m *mockPaster) Paste(text string) error {
	m.pastedText = text
	return m.err
}

// --- Tests ---

func TestApp_HappyPath(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}
	trans := &mockTranscriber{text: "hello world"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	// Simulate press → release
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	// Shut down
	close(events)
	<-done

	if !rec.startCalled {
		t.Error("recorder.Start was not called")
	}
	if !rec.stopCalled {
		t.Error("recorder.Stop was not called")
	}
	if paste.pastedText != "hello world" {
		t.Errorf("expected pasted text 'hello world', got %q", paste.pastedText)
	}
}

func TestApp_TranscriptionError_ContinuesListening(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{err: fmt.Errorf("whisper failed")}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	// First attempt — will fail transcription
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	// Second attempt — should still work (continues listening)
	rec.audio = []float32{0.3, 0.4}
	trans.err = nil
	trans.text = "recovered"

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	close(events)
	<-done

	if paste.pastedText != "recovered" {
		t.Errorf("expected pasted text 'recovered' after error recovery, got %q", paste.pastedText)
	}
}

func TestApp_EmptyAudio_NoPaste(t *testing.T) {
	rec := &mockRecorder{audio: nil}
	trans := &mockTranscriber{text: "should not be called"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	close(events)
	<-done

	if paste.pastedText != "" {
		t.Errorf("expected no paste for empty audio, got %q", paste.pastedText)
	}
}

func TestApp_EmptyText_NoPaste(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{text: ""}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	close(events)
	<-done

	if paste.pastedText != "" {
		t.Errorf("expected no paste for empty transcription, got %q", paste.pastedText)
	}
}
```

- [ ] **Step 2: Add missing import to test file**

The test file uses `fmt.Errorf` — add `"fmt"` to imports:

```go
import (
	"fmt"
	"log/slog"
	"testing"
	"time"
)
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd ~/voicetype
go test -v -run "TestApp" -count=1
```

Expected: FAIL — `NewApp`, `App.Run` not defined.

- [ ] **Step 4: Implement App orchestrator**

Create `app.go`:

```go
package main

import (
	"log/slog"
)

type App struct {
	recorder    Recorder
	transcriber Transcriber
	paster      Paster
	sound       *Sound
	logger      *slog.Logger
}

func NewApp(
	recorder Recorder,
	transcriber Transcriber,
	paster Paster,
	sound *Sound,
	logger *slog.Logger,
) *App {
	return &App{
		recorder:    recorder,
		transcriber: transcriber,
		paster:      paster,
		sound:       sound,
		logger:      logger.With("component", "app"),
	}
}

// Run processes hotkey events until the events channel is closed.
func (a *App) Run(events <-chan HotkeyEvent) {
	a.logger.Info("event loop started", "operation", "Run")

	for event := range events {
		switch event {
		case TriggerPressed:
			a.handlePress()
		case TriggerReleased:
			a.handleRelease()
		}
	}

	a.logger.Info("event loop stopped", "operation", "Run")
}

func (a *App) handlePress() {
	a.logger.Info("trigger pressed", "operation", "handlePress")
	a.sound.PlayStart()

	if err := a.recorder.Start(); err != nil {
		a.logger.Error("failed to start recording",
			"operation", "handlePress", "error", err)
		a.sound.PlayError()
	}
}

func (a *App) handleRelease() {
	a.logger.Info("trigger released", "operation", "handleRelease")

	audio, err := a.recorder.Stop()
	if err != nil {
		a.logger.Error("failed to stop recording",
			"operation", "handleRelease", "error", err)
		a.sound.PlayError()
		return
	}

	a.sound.PlayStop()

	if len(audio) == 0 {
		a.logger.Warn("no audio captured", "operation", "handleRelease")
		return
	}

	text, err := a.transcriber.Transcribe(audio)
	if err != nil {
		a.logger.Error("transcription failed",
			"operation", "handleRelease", "error", err)
		a.sound.PlayError()
		return
	}

	if text == "" {
		a.logger.Warn("no speech detected", "operation", "handleRelease")
		return
	}

	if err := a.paster.Paste(text); err != nil {
		a.logger.Error("paste failed",
			"operation", "handleRelease", "error", err)
		a.sound.PlayError()
		return
	}

	a.logger.Info("text pasted", "operation", "handleRelease",
		"text_length", len(text))
}

// Shutdown gracefully closes all components.
func (a *App) Shutdown() {
	a.logger.Info("shutting down", "operation", "Shutdown")

	if err := a.recorder.Close(); err != nil {
		a.logger.Error("failed to close recorder",
			"operation", "Shutdown", "error", err)
	}
	if err := a.transcriber.Close(); err != nil {
		a.logger.Error("failed to close transcriber",
			"operation", "Shutdown", "error", err)
	}

	a.logger.Info("shutdown complete", "operation", "Shutdown")
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd ~/voicetype
go test -v -run "TestApp" -count=1
```

Expected: All 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
cd ~/voicetype
git add app.go app_test.go
git commit -m "feat: app orchestrator with event loop and error recovery"
```

---

### Task 10: Main Entry Point + Build Verification

**Files:**
- Create: `main.go`
- Update: `CLAUDE.md`

- [ ] **Step 1: Implement main**

Create `main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	// The main goroutine must stay on the main OS thread for macOS CFRunLoop
	runtime.LockOSThread()

	configPath := flag.String("config", DefaultConfigPath(), "path to config file")
	flag.Parse()

	// --- Load config ---
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// --- Setup logger ---
	logger, logCleanup, err := SetupLogger(DefaultConfigDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer logCleanup()

	logger.Info("starting voicetype",
		"component", "main", "operation", "main",
		"config_path", *configPath,
		"model_size", cfg.ModelSize,
		"trigger_key", cfg.TriggerKey,
		"language", cfg.Language,
		"sample_rate", cfg.SampleRate,
	)

	// --- Init PortAudio ---
	if err := InitAudio(); err != nil {
		logger.Error("failed to initialize audio",
			"component", "main", "operation", "main", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := TerminateAudio(); err != nil {
			logger.Error("failed to terminate audio",
				"component", "main", "operation", "main", "error", err)
		}
	}()

	// --- Init transcriber (loads model — may download on first run) ---
	modelPath := DefaultModelPath(cfg.ModelSize)
	transcriber, err := NewTranscriber(modelPath, cfg.Language, logger)
	if err != nil {
		logger.Error("failed to initialize transcriber",
			"component", "main", "operation", "main", "error", err)
		os.Exit(1)
	}

	// --- Init recorder ---
	recorder := NewRecorder(cfg.SampleRate, logger)

	// --- Init paster ---
	paster := NewPaster(logger)

	// --- Init sound ---
	sound := NewSound(cfg.SoundFeedback, logger)

	// --- Create app ---
	app := NewApp(recorder, transcriber, paster, sound, logger)

	// --- Signal handling ---
	events := make(chan HotkeyEvent, 10)
	hotkey := NewHotkeyListener(cfg.TriggerKey, logger)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal",
			"component", "main", "operation", "signal",
			"signal", sig.String())
		if err := hotkey.Stop(); err != nil {
			logger.Error("failed to stop hotkey listener",
				"component", "main", "operation", "signal", "error", err)
		}
	}()

	// --- Start event processing goroutine ---
	go func() {
		app.Run(events)
		app.Shutdown()
	}()

	// --- Ready ---
	sound.PlayReady()
	logger.Info("ready — hold trigger key to record, release to transcribe",
		"component", "main", "operation", "main",
		"trigger_key", cfg.TriggerKey)

	// --- Start hotkey listener on main thread (blocks) ---
	if err := hotkey.Start(events); err != nil {
		logger.Error("hotkey listener failed",
			"component", "main", "operation", "main", "error", err)
		os.Exit(1)
	}

	// When hotkey.Start returns (after Stop()), close the events channel
	close(events)

	logger.Info("voicetype stopped",
		"component", "main", "operation", "main")
}
```

- [ ] **Step 2: Run all tests**

```bash
cd ~/voicetype
go test -v -count=1 ./...
```

Expected: All tests pass (logger, config, app tests).

- [ ] **Step 3: Build the full binary**

```bash
cd ~/voicetype
make build
```

Expected: `voicetype` binary created in the project root. No build errors.

- [ ] **Step 4: Verify binary runs**

```bash
cd ~/voicetype
./voicetype --help
```

Expected: Shows `-config` flag usage.

```bash
./voicetype 2>&1 &
sleep 2
kill %1
```

Expected: Starts, logs "ready" message (or fails with "Accessibility permission" which is expected on first run — grant permission in System Settings → Privacy & Security → Accessibility, then retry).

- [ ] **Step 5: Update CLAUDE.md**

Update `CLAUDE.md` with the new Go project information. Replace the existing content with:

```markdown
# VoiceType

## Goal

A lightweight, local, voice-to-text tool for macOS. Single Go binary.
The ONLY job: hold a hotkey, speak, release, text appears at the cursor. Wherever the cursor is.

## Non-Negotiable Requirements

- **Local/offline only** — no cloud APIs, no network calls (one-time model download excepted)
- **Fast** — near-zero latency between releasing key and text appearing
- **Accurate** — uses whisper.cpp `small` model with Metal GPU acceleration
- **Universal** — works in any app where you can type
- **Every error handled** — zero silent failures, no swallowed errors
- **One logging standard** — structured JSON via slog, component+operation on every entry
- **Contracts are absolute** — interfaces define exact behavior, no ambiguity

## Technical Context

- **Platform**: macOS (Apple Silicon / arm64)
- **Language**: Go with cgo (whisper.cpp, PortAudio, CoreGraphics, AppKit)
- **Speech model**: whisper.cpp `small` (~466MB) with Metal GPU
- **Audio capture**: PortAudio via `gordonklaus/portaudio`
- **Global hotkey**: CGEvent tap (supports Fn key detection)
- **Paste mechanism**: NSPasteboard + CGEvent Cmd+V simulation
- **Config**: `~/.config/voicetype/config.yaml` (YAML)
- **Default hotkey**: Fn+Shift (push-to-talk: hold to record, release to transcribe)

## Build

```bash
make setup         # install brew deps (portaudio, cmake)
make whisper       # build whisper.cpp with Metal
make download-model # download whisper small model (~466MB)
make build         # build Go binary
make test          # run tests
```

## Project Structure

Flat `package main`. All Go files in root. macOS Objective-C in `*_darwin.m` files.
whisper.cpp lives in `third_party/whisper.cpp/` as a git submodule.

## Dependencies

- **System**: portaudio, cmake (via Homebrew)
- **Go**: `gordonklaus/portaudio`, `gopkg.in/yaml.v3`
- **Submodule**: `third_party/whisper.cpp` (built with Metal)

## Engineering Standards

- Every Go error checked and handled with `component.operation` context
- Structured logging: `log/slog` JSON, always `component` + `operation` fields
- No `fmt.Println`, no `log.Printf`, no silent error swallowing
- Interfaces at every component boundary
- Tests use mocks at interface boundaries

## Roadmap

- **v1** (current): Core push-to-talk, configurable trigger key
- **v2**: Custom dictionary for improved recognition
- **v3**: Menu bar UI (Wails) for settings and dictionary management
```

- [ ] **Step 6: Commit**

```bash
cd ~/voicetype
git add main.go CLAUDE.md
git commit -m "feat: main entry point with signal handling and updated project docs"
```

- [ ] **Step 7: Final full build and test**

```bash
cd ~/voicetype
make clean
make
make test
```

Expected: Clean build, all tests pass, `voicetype` binary produced.

---

## Post-Implementation Notes

### macOS Permissions Required

On first run, the user must grant:
1. **Accessibility** (System Settings → Privacy & Security → Accessibility) — for global hotkey detection and key simulation
2. **Microphone** (System Settings → Privacy & Security → Microphone) — for audio capture

### Fn Key Behavior

The Fn key on modern Macs may be configured for other functions (emoji picker, dictation). Fn+Shift should work regardless of the single-Fn-press setting, since the system only intercepts Fn pressed alone, not Fn+modifier combos.

### Trigger Key Limitation (v1)

Only modifier keys (`fn`, `shift`, `ctrl`, `option`, `cmd`) work as trigger keys in v1. The CGEvent tap monitors modifier flags only. Regular key support would require monitoring `kCGEventKeyDown`/`kCGEventKeyUp` events, which is a v2 enhancement.
