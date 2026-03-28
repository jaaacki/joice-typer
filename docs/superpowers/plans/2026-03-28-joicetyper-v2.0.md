# JoiceTyper v2.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add live streaming transcription mode — text appears at cursor as user speaks, self-corrects in real-time, with a final accuracy pass on key release.

**Architecture:** New `Typer` (CGEvent keystrokes) and `Streamer` (1-second transcription loop with diff) run alongside existing clipboard path. Config switch `type_mode` selects mode. Existing code untouched.

**Tech Stack:** Go + cgo (CoreGraphics CGEvent), whisper.cpp (same model, reused), existing PortAudio recorder with new Snapshot() method

---

## File Structure

```
New:
  typer.go          — Typer interface + CGEvent keystroke implementation
  streamer.go       — Streaming loop: tick, snapshot, transcribe, diff, type
  streamer_test.go  — Mock-based tests for streaming loop and diff

Modified:
  contracts.go      — Add Typer interface, Snapshot() to Recorder
  recorder.go       — Implement Snapshot()
  config.go         — Add TypeMode field + validation
  config_default.yaml — Add type_mode default
  app.go            — Branch handlePress/handleRelease on type mode
  app_test.go       — Add stream mode tests
  main.go           — Create Typer, pass to App
  CLAUDE.md         — Update for v2.0
```

---

### Task 1: Contracts Update

**Files:**
- Modify: `contracts.go`

- [ ] **Step 1: Add Typer interface and Snapshot to Recorder**

Add to `/Users/noonoon/voicetype/contracts.go`:

```go
// Typer streams text at the cursor via simulated keystrokes.
type Typer interface {
	Type(text string) error
	Backspace(count int) error
	ReplaceAll(oldLen int, newText string) error
}
```

Add `Snapshot() []float32` to the Recorder interface:

```go
type Recorder interface {
	Start() error
	Stop() ([]float32, error)
	Snapshot() []float32 // copy of audio captured so far, without stopping
	Close() error
}
```

- [ ] **Step 2: Update mock recorder in app_test.go**

Add Snapshot to mockRecorder:

```go
func (m *mockRecorder) Snapshot() []float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.audio
}
```

- [ ] **Step 3: Verify build**

```bash
go vet ./...
```

- [ ] **Step 4: Commit**

```bash
git add contracts.go app_test.go
git commit -m "feat: add Typer interface and Snapshot() to Recorder contract"
```

---

### Task 2: Recorder Snapshot

**Files:**
- Modify: `recorder.go`

- [ ] **Step 1: Implement Snapshot**

Add to `/Users/noonoon/voicetype/recorder.go`:

```go
func (r *portaudioRecorder) Snapshot() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := 0
	for _, chunk := range r.chunks {
		total += len(chunk)
	}
	if total == 0 {
		return nil
	}

	audio := make([]float32, 0, total)
	for _, chunk := range r.chunks {
		audio = append(audio, chunk...)
	}
	return audio
}
```

- [ ] **Step 2: Verify build**

```bash
go vet ./...
```

- [ ] **Step 3: Commit**

```bash
git add recorder.go
git commit -m "feat: Recorder.Snapshot() for live audio buffer access"
```

---

### Task 3: CGEvent Stream Typer

**Files:**
- Create: `typer.go`

- [ ] **Step 1: Implement typer**

Create `/Users/noonoon/voicetype/typer.go`:

```go
package main

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

static inline int typeUnichar(unsigned short ch) {
	CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
	if (down == NULL) return 1;
	UniChar c = (UniChar)ch;
	CGEventKeyboardSetUnicodeString(down, 1, &c);
	CGEventPost(kCGHIDEventTap, down);

	CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
	if (up == NULL) {
		CFRelease(down);
		return 2;
	}
	CGEventKeyboardSetUnicodeString(up, 1, &c);
	CGEventPost(kCGHIDEventTap, up);

	CFRelease(down);
	CFRelease(up);
	return 0;
}

static inline int typeBackspace(void) {
	CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0x33, true);
	if (down == NULL) return 1;
	CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0x33, false);
	if (up == NULL) {
		CFRelease(down);
		return 2;
	}
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(down);
	CFRelease(up);
	return 0;
}
*/
import "C"

import (
	"fmt"
	"log/slog"
	"time"
)

type cgEventTyper struct {
	logger     *slog.Logger
	charDelay  time.Duration
}

func NewTyper(logger *slog.Logger) Typer {
	return &cgEventTyper{
		logger:    logger.With("component", "typer"),
		charDelay: 1 * time.Millisecond,
	}
}

func (t *cgEventTyper) Type(text string) error {
	t.logger.Debug("typing", "operation", "Type", "length", len(text))
	for _, r := range text {
		result := C.typeUnichar(C.ushort(r))
		if result != 0 {
			return fmt.Errorf("typer.Type: CGEvent failed for char %q (error %d)", r, int(result))
		}
		if t.charDelay > 0 {
			time.Sleep(t.charDelay)
		}
	}
	return nil
}

func (t *cgEventTyper) Backspace(count int) error {
	t.logger.Debug("backspacing", "operation", "Backspace", "count", count)
	for i := 0; i < count; i++ {
		result := C.typeBackspace()
		if result != 0 {
			return fmt.Errorf("typer.Backspace: CGEvent failed (error %d)", int(result))
		}
		if t.charDelay > 0 {
			time.Sleep(t.charDelay)
		}
	}
	return nil
}

func (t *cgEventTyper) ReplaceAll(oldLen int, newText string) error {
	if oldLen > 0 {
		if err := t.Backspace(oldLen); err != nil {
			return fmt.Errorf("typer.ReplaceAll: %w", err)
		}
	}
	if len(newText) > 0 {
		if err := t.Type(newText); err != nil {
			return fmt.Errorf("typer.ReplaceAll: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 2: Verify build**

```bash
go vet ./...
```

- [ ] **Step 3: Commit**

```bash
git add typer.go
git commit -m "feat: CGEvent stream typer with Unicode support"
```

---

### Task 4: Streamer — Streaming Transcription Loop

**Files:**
- Create: `streamer.go`
- Create: `streamer_test.go`

- [ ] **Step 1: Implement streamer**

Create `/Users/noonoon/voicetype/streamer.go`:

```go
package main

import (
	"log/slog"
	"sync"
	"time"
)

// commonPrefixLen returns the length of the longest common prefix between a and b.
func commonPrefixLen(a, b string) int {
	maxLen := len(a)
	if len(b) < maxLen {
		maxLen = len(b)
	}
	i := 0
	for i < maxLen && a[i] == b[i] {
		i++
	}
	return i
}

// Streamer runs a periodic transcription loop during recording,
// streaming partial results to the cursor via a Typer.
type Streamer struct {
	transcriber Transcriber
	typer       Typer
	recorder    Recorder
	logger      *slog.Logger
	interval    time.Duration

	mu       sync.Mutex
	lastText string
	running  bool
	stopCh   chan struct{}
	done     chan struct{}
}

func NewStreamer(
	transcriber Transcriber,
	typer Typer,
	recorder Recorder,
	logger *slog.Logger,
	interval time.Duration,
) *Streamer {
	return &Streamer{
		transcriber: transcriber,
		typer:       typer,
		recorder:    recorder,
		logger:      logger.With("component", "streamer"),
		interval:    interval,
	}
}

func (s *Streamer) Start() {
	s.mu.Lock()
	s.lastText = ""
	s.running = true
	s.stopCh = make(chan struct{})
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.loop()
	s.logger.Info("streaming started", "operation", "Start", "interval", s.interval)
}

func (s *Streamer) loop() {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Streamer) tick() {
	audio := s.recorder.Snapshot()
	if len(audio) == 0 {
		return
	}

	text, err := s.transcriber.Transcribe(audio)
	if err != nil {
		s.logger.Error("streaming transcription failed", "operation", "tick", "error", err)
		return
	}

	s.mu.Lock()
	prev := s.lastText
	s.mu.Unlock()

	if text == prev {
		return // no change
	}

	// Diff: find common prefix, backspace old suffix, type new suffix
	prefixLen := commonPrefixLen(prev, text)
	oldSuffix := len([]rune(prev)) - len([]rune(prev[:prefixLen]))
	newSuffix := text[prefixLen:]

	// Count runes for backspace (not bytes)
	oldRunes := []rune(prev[prefixLen:])

	s.logger.Debug("streaming update", "operation", "tick",
		"prev_len", len(prev), "new_len", len(text),
		"backspace", len(oldRunes), "type_len", len(newSuffix))

	if err := s.typer.ReplaceAll(len(oldRunes), newSuffix); err != nil {
		s.logger.Error("streaming type failed", "operation", "tick", "error", err)
		return
	}

	s.mu.Lock()
	s.lastText = text
	s.mu.Unlock()
}

// Stop stops the streaming loop. Call Finalize after this.
func (s *Streamer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	<-s.done
	s.logger.Info("streaming stopped", "operation", "Stop")
}

// Finalize runs a final transcription on the complete audio and applies corrections.
// Returns the final text.
func (s *Streamer) Finalize(audio []float32) (string, error) {
	if len(audio) == 0 {
		return s.lastText, nil
	}

	finalText, err := s.transcriber.Transcribe(audio)
	if err != nil {
		return "", fmt.Errorf("streamer.Finalize: %w", err)
	}

	s.mu.Lock()
	prev := s.lastText
	s.mu.Unlock()

	if finalText != prev {
		prefixLen := commonPrefixLen(prev, finalText)
		oldRunes := []rune(prev[prefixLen:])
		newSuffix := finalText[prefixLen:]

		if err := s.typer.ReplaceAll(len(oldRunes), newSuffix); err != nil {
			return "", fmt.Errorf("streamer.Finalize: %w", err)
		}
	}

	s.logger.Info("finalized", "operation", "Finalize", "text_length", len(finalText))
	return finalText, nil
}
```

Add `"fmt"` to imports.

- [ ] **Step 2: Write streamer tests**

Create `/Users/noonoon/voicetype/streamer_test.go`:

```go
package main

import (
	"sync"
	"testing"
	"time"
)

func TestCommonPrefixLen(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"hello", "hello world", 5},
		{"abc", "xyz", 0},
		{"I need to shed", "I need to schedule", 10},
		{"", "anything", 0},
		{"same", "same", 4},
	}
	for _, tt := range tests {
		got := commonPrefixLen(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonPrefixLen(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

type mockTyper struct {
	mu         sync.Mutex
	typed      string
	backspaced int
}

func (m *mockTyper) Type(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typed += text
	return nil
}

func (m *mockTyper) Backspace(count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backspaced += count
	// Remove from typed
	runes := []rune(m.typed)
	if count > len(runes) {
		count = len(runes)
	}
	m.typed = string(runes[:len(runes)-count])
	return nil
}

func (m *mockTyper) ReplaceAll(oldLen int, newText string) error {
	if err := m.Backspace(oldLen); err != nil {
		return err
	}
	return m.Type(newText)
}

func (m *mockTyper) getText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.typed
}

type sequenceTranscriber struct {
	mu       sync.Mutex
	results  []string
	callIdx  int
}

func (s *sequenceTranscriber) Transcribe(audio []float32) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callIdx >= len(s.results) {
		return s.results[len(s.results)-1], nil
	}
	text := s.results[s.callIdx]
	s.callIdx++
	return text, nil
}

func (s *sequenceTranscriber) Close() error { return nil }

func TestStreamer_ProgressiveCorrection(t *testing.T) {
	typer := &mockTyper{}
	trans := &sequenceTranscriber{results: []string{
		"I need to",
		"I need to shed",
		"I need to schedule a meeting",
	}}
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}

	logger := testLogger()
	s := NewStreamer(trans, typer, rec, logger, 50*time.Millisecond)
	s.Start()

	time.Sleep(200 * time.Millisecond)
	s.Stop()

	got := typer.getText()
	if got != "I need to schedule a meeting" {
		t.Errorf("expected final text 'I need to schedule a meeting', got %q", got)
	}
}

func TestStreamer_Finalize(t *testing.T) {
	typer := &mockTyper{}
	trans := &sequenceTranscriber{results: []string{
		"hello worl",
		"hello world and goodbye",
	}}
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}

	logger := testLogger()
	s := NewStreamer(trans, typer, rec, logger, 50*time.Millisecond)
	s.Start()

	time.Sleep(80 * time.Millisecond)
	s.Stop()

	finalText, err := s.Finalize([]float32{0.1, 0.2, 0.3})
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if finalText != "hello world and goodbye" {
		t.Errorf("expected 'hello world and goodbye', got %q", finalText)
	}

	got := typer.getText()
	if got != finalText {
		t.Errorf("typed text %q != final text %q", got, finalText)
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
```

Add `"log/slog"` to imports.

- [ ] **Step 3: Run tests**

```bash
go test -race -count=1 -v -run "TestCommonPrefix|TestStreamer" ./...
```

- [ ] **Step 4: Commit**

```bash
git add streamer.go streamer_test.go
git commit -m "feat: streaming transcription loop with diff and correction"
```

---

### Task 5: Config and App Wiring

**Files:**
- Modify: `config.go`
- Modify: `config_default.yaml`
- Modify: `app.go`
- Modify: `main.go`

- [ ] **Step 1: Add TypeMode to config**

In `config.go`, add field to Config struct:

```go
TypeMode      string   `yaml:"type_mode"`
```

Add validation in `Validate()`:

```go
validTypeModes := map[string]bool{"clipboard": true, "stream": true}
if c.TypeMode != "" && !validTypeModes[c.TypeMode] {
	return fmt.Errorf("config.Validate: invalid type_mode %q (must be clipboard or stream)", c.TypeMode)
}
```

In `LoadConfig`, after unmarshalling, set default if empty:

```go
if cfg.TypeMode == "" {
	cfg.TypeMode = "clipboard"
}
```

- [ ] **Step 2: Update config_default.yaml**

Add to `/Users/noonoon/voicetype/config_default.yaml`:

```yaml
type_mode: "clipboard"  # "clipboard" (paste) or "stream" (live typing)
```

- [ ] **Step 3: Update App to support stream mode**

Modify `/Users/noonoon/voicetype/app.go`. Add fields:

```go
type App struct {
	recorder      Recorder
	transcriber   Transcriber
	paster        Paster
	typer         Typer
	sound         *Sound
	logger        *slog.Logger
	busy          int32
	stateMu       sync.RWMutex
	onStateChange func(AppState)
	typeMode      string
	streamer      *Streamer
}
```

Update `NewApp` signature:

```go
func NewApp(
	recorder Recorder,
	transcriber Transcriber,
	paster Paster,
	typer Typer,
	sound *Sound,
	typeMode string,
	logger *slog.Logger,
) *App {
	return &App{
		recorder:      recorder,
		transcriber:   transcriber,
		paster:        paster,
		typer:         typer,
		sound:         sound,
		logger:        logger.With("component", "app"),
		onStateChange: func(AppState) {},
		typeMode:      typeMode,
	}
}
```

Update `handlePress`:

```go
func (a *App) handlePress() {
	if atomic.LoadInt32(&a.busy) == 1 {
		a.logger.Warn("still transcribing, ignoring press", "operation", "handlePress")
		a.sound.PlayError()
		return
	}
	a.logger.Debug("trigger pressed", "operation", "handlePress")
	a.sound.PlayStart()
	a.emitState(StateRecording)

	if err := a.recorder.Start(); err != nil {
		a.logger.Error("failed to start recording", "operation", "handlePress", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	if a.typeMode == "stream" {
		a.streamer = NewStreamer(a.transcriber, a.typer, a.recorder, a.logger, 1*time.Second)
		a.streamer.Start()
	}
}
```

Update `handleRelease`:

```go
func (a *App) handleRelease() {
	a.logger.Debug("trigger released", "operation", "handleRelease")

	if a.typeMode == "stream" {
		a.handleReleaseStream()
	} else {
		a.handleReleaseClipboard()
	}
}

func (a *App) handleReleaseStream() {
	if a.streamer != nil {
		a.streamer.Stop()
	}

	audio, err := a.recorder.Stop()
	if err != nil {
		a.logger.Error("failed to stop recording", "operation", "handleReleaseStream", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	a.emitState(StateTranscribing)

	if a.streamer != nil && len(audio) > 0 {
		finalText, finalErr := a.streamer.Finalize(audio)
		if finalErr != nil {
			a.logger.Error("finalize failed", "operation", "handleReleaseStream", "error", finalErr)
			a.sound.PlayError()
		} else {
			a.logger.Info("stream complete", "operation", "handleReleaseStream", "text_length", len(finalText))
		}
	}

	a.streamer = nil
	a.emitState(StateReady)
}

func (a *App) handleReleaseClipboard() {
	audio, err := a.recorder.Stop()
	if err != nil {
		a.logger.Error("failed to stop recording", "operation", "handleReleaseClipboard", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	a.sound.PlayStop()
	a.emitState(StateTranscribing)

	if len(audio) == 0 {
		a.logger.Warn("no audio captured", "operation", "handleReleaseClipboard")
		a.emitState(StateReady)
		return
	}

	go a.transcribeAndPaste(audio)
}
```

Add `"time"` to imports.

- [ ] **Step 4: Update main.go to create Typer and pass to App**

In `runTerminalMode` and `runAppMode`, create the Typer and update NewApp call:

```go
var typer Typer
if cfg.TypeMode == "stream" {
	typer = NewTyper(logger)
}

app := NewApp(recorder, transcriber, paster, typer, sound, cfg.TypeMode, logger)
```

- [ ] **Step 5: Update app_test.go for new NewApp signature**

All test calls to `NewApp` need the new `typer` and `typeMode` params:

```go
app := NewApp(rec, trans, paste, nil, snd, "clipboard", logger)
```

Add a stream mode test:

```go
func TestApp_StreamMode(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}
	trans := &mockTranscriber{text: "hello stream"}
	typer := &mockTyper{}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, typer, snd, "stream", logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})
	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(1500 * time.Millisecond) // let streamer tick
	events <- TriggerReleased
	time.Sleep(200 * time.Millisecond)

	close(events)
	<-done

	got := typer.getText()
	if got == "" {
		t.Error("expected streamer to have typed text")
	}
}
```

- [ ] **Step 6: Run all tests**

```bash
go test -race -count=1 -v ./...
```

- [ ] **Step 7: Commit**

```bash
git add config.go config_default.yaml app.go app_test.go main.go
git commit -m "feat: stream type mode wiring — config, app orchestrator, main"
```

---

### Task 6: CLAUDE.md Update and Final Build

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md**

Add to the Technical Context section:

```markdown
- **Type modes**: "clipboard" (v1 paste) or "stream" (v2 live CGEvent keystrokes)
- **Streaming**: 1-second interval sliding window transcription with common-prefix diff correction
```

Update Roadmap:

```markdown
## Roadmap

- **v1** (done): Core push-to-talk, configurable trigger key
- **v1.5** (done): .app bundle, menu bar icon, setup wizard
- **v2** (current): Streaming type mode with live self-correcting transcription
- **v2.5**: Custom dictionary, settings UI for mic/hotkey/type mode
- **v3**: Menu bar UI (Wails) for full settings management
```

- [ ] **Step 2: Full test and build**

```bash
go test -race -count=1 ./...
go vet ./...
CGO_ENABLED=1 go build -o voicetype .
make app
```

- [ ] **Step 3: Test stream mode**

```bash
# Edit ~/.config/voicetype/config.yaml:
# type_mode: "stream"
# Then run:
./voicetype
```

Hold Fn+Shift, speak, watch text appear and self-correct at the cursor. Release for final pass.

- [ ] **Step 4: Commit and tag**

```bash
git add CLAUDE.md
git commit -m "feat: JoiceTyper v2.0 — streaming type mode"
git tag v2.0
git push origin main --tags
```
