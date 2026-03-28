# VoiceType Go v1 — Design Spec

## Overview

A local, offline voice-to-text tool for macOS. Single Go binary. Hold trigger key, speak, release, text appears at cursor. That's it.

Replaces the existing Python prototype with a properly engineered Go application using whisper.cpp for transcription with Metal GPU acceleration.

## Non-Negotiable Engineering Standards

- **Every error handled** — no silent failures, no `_ = err`, no swallowed returns
- **Contracts are absolute** — interfaces define exact inputs, outputs, and error conditions. No implicit behavior.
- **One logging standard** — `log/slog` structured JSON everywhere. `component` + `operation` on every entry. No `fmt.Println`, no `log.Printf`. Ever.

## Core Behavior

- **Push-to-talk**: hold Fn+Shift (configurable) to record, release to stop and transcribe
- **Transcription**: whisper.cpp `small` model, Metal GPU, local only
- **Text insertion**: clipboard paste (pbcopy + Cmd+V via CGEvent)
- **Audio feedback**: macOS system sounds played async (non-blocking)
- **Config**: `~/.config/voicetype/config.yaml`, created with defaults on first run
- **Model storage**: `~/.config/voicetype/models/ggml-small.bin`, downloaded on first run from Hugging Face (`huggingface.co/ggerganov/whisper.cpp`) via HTTP GET with progress logged to stderr

## Component Architecture

```
┌──────────┐    ┌──────────┐    ┌────────────┐    ┌────────────┐
│  Config   │    │  Hotkey   │───>│  Recorder  │───>│ Transcriber│───> Paster
│  Loader   │    │ Listener │    │  (Audio)   │    │ (Whisper)  │    (Clipboard)
└──────────┘    └──────────┘    └────────────┘    └────────────┘
```

Orchestrator (`App`) wires components and owns the event loop.

## Contracts

### Config

```go
type Config struct {
    TriggerKey    []string `yaml:"trigger_key"`    // e.g. ["fn", "shift"]
    ModelSize     string   `yaml:"model_size"`     // tiny|base|small|medium
    Language      string   `yaml:"language"`       // "" = auto-detect, or "en", "fr", etc.
    SampleRate    int      `yaml:"sample_rate"`    // 16000
    SoundFeedback bool     `yaml:"sound_feedback"` // true
}

func LoadConfig(path string) (Config, error)
func (c Config) Validate() error
```

Validation at startup. Invalid config → log specific field that failed, exit.

### HotkeyListener

```go
type HotkeyEvent int
const (
    TriggerPressed HotkeyEvent = iota
    TriggerReleased
)

type HotkeyListener interface {
    Start(events chan<- HotkeyEvent) error
    Stop() error
}
```

Implemented via macOS Carbon API `RegisterEventHotKey`. Pushes events into a channel.

### Recorder

```go
type Recorder interface {
    Start() error              // begin capturing audio
    Stop() ([]float32, error)  // stop and return audio buffer
    Close() error              // release audio device
}
```

PortAudio implementation. 16kHz mono float32 PCM. No shared mutable state — `Stop()` returns the buffer.

### Transcriber

```go
type Transcriber interface {
    Transcribe(audio []float32) (string, error)
    Close() error  // free whisper model
}
```

whisper.cpp Go bindings with Metal acceleration. Model loaded once at startup.

### Paster

```go
type Paster interface {
    Paste(text string) error
}
```

Writes to NSPasteboard, simulates Cmd+V via CGEvent.

## Orchestrator Flow

```
App.Run()
├── Load config (fail fast if invalid)
├── Initialize transcriber (load whisper model — slow, done once at startup)
├── Initialize recorder (open audio device)
├── Initialize hotkey listener
├── Log "ready" with config summary
│
└── Event loop:
    ├── TriggerPressed:
    │   ├── recorder.Start()
    │   ├── play start sound (async goroutine)
    │   └── log "recording started"
    │
    ├── TriggerReleased:
    │   ├── audio, err := recorder.Stop()
    │   ├── play stop sound (async goroutine)
    │   ├── if len(audio) == 0 → log warning, continue
    │   ├── text, err := transcriber.Transcribe(audio)
    │   ├── if text == "" → log "no speech detected", continue
    │   ├── paster.Paste(text)
    │   └── log "pasted" with text length
    │
    └── OS signal (SIGINT/SIGTERM):
        ├── hotkey.Stop()
        ├── recorder.Close()
        ├── transcriber.Close()
        └── clean exit
```

**Error handling at orchestrator level:**
- Config/init errors → log and exit immediately
- Recording/transcription/paste errors → log error, play error sound, continue listening. Never crash on a single failed transcription.

## Logging Standard

`log/slog` with JSON output. Every entry includes `component` and `operation`.

```json
{
    "time": "2026-03-28T10:15:03.123Z",
    "level": "INFO",
    "msg": "recording started",
    "component": "recorder",
    "operation": "Start"
}
```

Error entries add `error` field:

```json
{
    "level": "ERROR",
    "msg": "failed to capture audio",
    "component": "recorder",
    "operation": "Stop",
    "error": "portaudio: device not available"
}
```

**Rules:**
- `component` + `operation` on every entry
- No `fmt.Println`, no `log.Printf`, no bare prints
- Levels: `DEBUG` (internal state), `INFO` (lifecycle), `WARN` (empty audio, no speech), `ERROR` (failures)
- Log file: `~/.config/voicetype/voicetype.log` (keep last 5MB). Also stderr when interactive.
- Each component gets a child logger with `component` pre-set

## Config File

Default `~/.config/voicetype/config.yaml`:

```yaml
trigger_key:
  - fn
  - shift
model_size: small
language: ""
sample_rate: 16000
sound_feedback: true
```

**Trigger key names:**
- Modifiers: `fn`, `shift`, `ctrl`, `option`, `cmd`
- Regular keys: `space`, `a`-`z`, `f1`-`f12`, etc.

## Project Structure

```
~/voicetype/
├── go.mod
├── go.sum
├── main.go                  # entry point — parse flags, init logger, run App
├── app.go                   # App struct, orchestrator, event loop
├── config.go                # Config struct, LoadConfig, Validate, defaults
├── hotkey.go                # HotkeyListener interface + Carbon API implementation
├── recorder.go              # Recorder interface + PortAudio implementation
├── transcriber.go           # Transcriber interface + whisper.cpp implementation
├── paster.go                # Paster interface + clipboard/CGEvent implementation
├── sound.go                 # async system sound playback
├── logger.go                # slog setup, log file rotation, child logger factory
├── config_default.yaml      # embedded default config (via go:embed)
└── CLAUDE.md                # project instructions
```

Flat structure, `package main`. Single binary tool, not a library.

## Build

```bash
CGO_ENABLED=1 go build -o voicetype .
```

**System dependencies:**
- PortAudio: `brew install portaudio`
- whisper.cpp: built with Metal support

**Go dependencies:**
- `github.com/ggerganov/whisper.cpp/bindings/go` — whisper.cpp bindings
- `github.com/gordonklaus/portaudio` — PortAudio bindings
- `gopkg.in/yaml.v3` — config parsing

## Future (Not v1)

- **v2**: Custom dictionary (names, jargon, phrases) for improved recognition
- **v3**: Menu bar UI (Wails — Go + web frontend) for settings, dictionary management, visual feedback
