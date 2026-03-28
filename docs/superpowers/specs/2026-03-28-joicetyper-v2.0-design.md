# JoiceTyper v2.0 — Streaming Type Mode

## Overview

Add a live streaming transcription mode ("stream") alongside the existing clipboard paste mode ("clipboard"). In stream mode, text appears at the cursor as the user speaks and self-corrects as whisper refines its hypothesis. On key release, a final full-audio pass ensures accuracy.

The clipboard path is completely untouched. A config switch (`type_mode`) selects the mode.

## Core Behavior

### Stream Mode Flow

```
Hold Fn+Shift:
├── Recorder starts capturing audio
├── Streaming loop starts (every 1 second):
│   ├── Snapshot current audio buffer (without stopping recorder)
│   ├── Transcribe accumulated audio via whisper.cpp
│   ├── Diff new text against previously typed text (common prefix match)
│   └── Backspace old suffix + type new suffix via CGEvent keystrokes
│
Release Fn+Shift:
├── Stop streaming loop
├── Stop recorder, get full audio buffer
├── Final transcription pass on complete audio
├── Diff final text against last streamed text
├── Backspace + retype corrections
└── Done
```

### Clipboard Mode Flow (unchanged)

Existing v1 behavior. Hold → record → release → transcribe once → clipboard paste.

### Config

```yaml
type_mode: "clipboard"  # default, v1 behavior
# type_mode: "stream"   # v2 live typing
```

## Contracts

### New: Typer Interface

```go
// Typer streams text at the cursor via simulated keystrokes.
type Typer interface {
    Type(text string) error
    Backspace(count int) error
    ReplaceAll(oldLen int, newText string) error
}
```

- `Type` — simulates keystrokes for each character using `CGEventKeyboardSetUnicodeString`
- `Backspace` — simulates N presses of the delete key (keycode 0x33)
- `ReplaceAll` — convenience: backspace `oldLen` characters then type `newText`

Implemented in pure Go + cgo C (CoreGraphics). No Objective-C needed.

### Modified: Recorder Interface

```go
type Recorder interface {
    Start() error
    Stop() ([]float32, error)
    Snapshot() []float32  // NEW: copy of audio captured so far, without stopping
    Close() error
}
```

`Snapshot()` copies the current chunks under mutex and returns a flattened buffer. Recording continues. Used only in stream mode.

### Unchanged

- `Paster` interface — clipboard path untouched
- `Transcriber` interface — same `Transcribe([]float32) (string, error)`, reused by both modes
- `HotkeyListener` interface — unchanged

## Diff Algorithm

Common prefix matching between previous and new transcription text:

```
Previous: "I need to shed"
New:      "I need to schedule a"

Common prefix: "I need to s"  (11 chars)
Backspace: len("hed") = 3
Type: "chedule a"
```

Find the longest common prefix. Backspace everything after the prefix in old text. Type everything after the prefix in new text. O(n) string comparison.

## Streaming Loop (streamer.go)

```go
type Streamer struct {
    transcriber  Transcriber
    typer        Typer
    recorder     Recorder
    logger       *slog.Logger
    interval     time.Duration   // 1 second
    stopCh       chan struct{}
    lastText     string          // what was typed so far
}
```

- `Start()` — launches a goroutine with a 1-second ticker
- Each tick: `recorder.Snapshot()` → `transcriber.Transcribe()` → diff → `typer.ReplaceAll()`
- `StopAndFinalize(audio []float32) (string, error)` — stops the ticker, runs final full-buffer transcription, applies final correction, returns the final text

The Streamer is created fresh for each recording session (press → release). It's not a long-lived component.

## CGEvent Typer (typer.go)

Pure Go + cgo. Uses CoreGraphics C API:

```c
// Type a Unicode character
CGEventRef event = CGEventCreateKeyboardEvent(NULL, 0, true);
UniChar chars[] = { character };
CGEventKeyboardSetUnicodeString(event, 1, chars);
CGEventPost(kCGHIDEventTap, event);
// ... key up event
CFRelease(event);
```

For backspace:
```c
// Delete key = keycode 0x33
CGEventRef event = CGEventCreateKeyboardEvent(NULL, 0x33, true);
CGEventPost(kCGHIDEventTap, event);
// ... key up event
CFRelease(event);
```

Small inter-keystroke delay (~1-2ms) to prevent event coalescing issues in target apps.

## App Orchestrator Changes

`App` gains:
- `typeMode string` field ("clipboard" or "stream")
- `typer Typer` field (nil in clipboard mode)
- `streamer *Streamer` field (created per recording session in stream mode)

### handlePress (stream mode)

```go
if a.typeMode == "stream" {
    recorder.Start()
    a.streamer = NewStreamer(a.transcriber, a.typer, a.recorder, a.logger, 1*time.Second)
    a.streamer.Start()
}
```

### handleRelease (stream mode)

```go
if a.typeMode == "stream" {
    a.streamer.Stop()
    audio, _ := a.recorder.Stop()
    finalText, err := a.streamer.Finalize(audio)
    // final correction applied inside Finalize
}
```

The clipboard path in handlePress/handleRelease is unchanged.

## Config Changes

```go
type Config struct {
    TriggerKey    []string `yaml:"trigger_key"`
    ModelSize     string   `yaml:"model_size"`
    Language      string   `yaml:"language"`
    SampleRate    int      `yaml:"sample_rate"`
    SoundFeedback bool     `yaml:"sound_feedback"`
    InputDevice   string   `yaml:"input_device"`
    TypeMode      string   `yaml:"type_mode"`     // NEW: "clipboard" or "stream"
}
```

Validation: `type_mode` must be `"clipboard"` or `"stream"`. Default: `"clipboard"`.

## File Changes

### New Files
- `typer.go` — Typer interface implementation (Go + cgo C, CoreGraphics)
- `streamer.go` — Streaming loop: ticker, snapshot, transcribe, diff, type
- `streamer_test.go` — Tests with mock typer and transcriber

### Modified Files
- `contracts.go` — Add Typer interface, add Snapshot() to Recorder
- `recorder.go` — Implement Snapshot()
- `config.go` — Add TypeMode field, validation
- `config_default.yaml` — Add type_mode: "clipboard"
- `app.go` — Accept Typer, branch on type mode
- `main.go` — Create Typer, pass to App
- `CLAUDE.md` — Update for v2.0

### Unchanged Files
- `paster.go`, `paster_darwin.h`, `paster_darwin.m` — clipboard path untouched
- `transcriber.go` — same interface, reused
- `hotkey.go`, `hotkey_darwin.m` — unchanged
- `sound.go`, `logger.go` — unchanged
- All v1.5 files (setup, statusbar, notification) — unchanged

## What's NOT in v2.0

- Custom dictionary (v2.5)
- Settings UI for switching type mode (v2.5 — for now, edit YAML)
- Settings UI for mic/hotkey changes (v2.5)
- Wails full UI (v3)
