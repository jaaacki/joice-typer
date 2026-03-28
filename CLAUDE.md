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
- **Type modes**: "clipboard" (v1 paste) or "stream" (v2 live CGEvent keystrokes)
- **Streaming**: 1-second interval sliding window transcription with common-prefix diff correction

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

- **v1** (done): Core push-to-talk, configurable trigger key
- **v1.5** (done): .app bundle, menu bar icon, setup wizard
- **v2** (done): Streaming type mode (experimental, default off)
- **v2.5** (current): Settings UI for mic/hotkey selection
- **v3**: Custom dictionary (whisper prompt + post-processing replacement)
- **v4**: Menu bar UI (Wails) for full settings management
