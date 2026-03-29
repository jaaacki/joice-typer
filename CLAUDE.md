# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Goal

A lightweight, local, voice-to-text tool for macOS. Single Go binary.
The ONLY job: hold a hotkey, speak, release, text appears at the cursor. Wherever the cursor is.

## Non-Negotiable Requirements

- **Local/offline only** — no cloud APIs, no network calls (one-time model download excepted)
- **Fast** — near-zero latency between releasing key and text appearing
- **Every error handled** — zero silent failures, no swallowed errors
- **One logging standard** — structured JSON via slog, component+operation on every entry
- **Contracts are absolute** — interfaces define exact behavior, no ambiguity

## Build & Test

```bash
make setup          # install brew deps (portaudio, cmake)
make whisper        # build whisper.cpp with Metal (must run before build)
make download-model # download whisper small model (~466MB)
make build          # CGO_ENABLED=1 go build -o voicetype .
make test           # go test -v -count=1 ./...
make app            # build .app bundle with bundled dylibs
make clean          # remove binary, whisper build artifacts, app bundle
```

Run a single test:
```bash
go test -v -count=1 -run TestFunctionName ./...
```

The binary requires Accessibility permission on macOS (System Settings > Privacy & Security > Accessibility).

## Architecture

Flat `package main`. All Go files in root. No sub-packages.

### Core pipeline

`HotkeyListener` -> `App` -> `Recorder` -> `Transcriber` -> `Paster`/`Typer`

- **contracts.go** — All component interfaces (`HotkeyListener`, `Recorder`, `Transcriber`, `Paster`, `Typer`). This is the source of truth for component boundaries.
- **app.go** — Orchestrator. Wires hotkey events to the record->transcribe->paste pipeline. Manages state transitions (`StateLoading`/`StateReady`/`StateRecording`/`StateTranscribing`). Two code paths: `handleReleaseClipboard()` (async transcribe+paste) and `handleReleaseStream()` (live streaming).
- **main.go** — Entry point. Two modes: `runAppMode()` (inside .app bundle, with status bar + setup wizard) and `runTerminalMode()` (CLI). Main goroutine is locked to OS thread for macOS CFRunLoop.

### Components

- **hotkey.go** — CGEvent tap via cgo. The `Start()` call blocks on CFRunLoop (must be main thread). Uses package-level `hotkeyEvents` channel for C->Go callback bridge. Release events are never dropped; press events are dropped if channel full.
- **recorder.go** — PortAudio capture. Each session gets a unique `sessionID` to prevent zombie goroutine interference. `readLoop` owns its stream by value and closes on exit. Max 30s recording. `Snapshot()` returns a copy of audio captured so far (used by streaming mode).
- **transcriber.go** — whisper.cpp via cgo. Thread-safe (mutex). Also handles model download, SHA-256 verification against pinned manifest, and quarantine of corrupt models.
- **streamer.go** — Periodic (1s interval) transcription loop for stream mode. Takes audio snapshots, transcribes, and uses Typer to update cursor position via backspace+retype.
- **paster.go** — NSPasteboard + simulated Cmd+V (clipboard mode).
- **typer.go** — CGEvent keyboard simulation, rune-by-rune with 1ms delay. Handles BMP and supplementary Unicode via UTF-16 surrogate pairs. Clears modifier flags to prevent Fn/Shift leaking into typed text.
- **config.go** — YAML config at `~/Library/Application Support/JoiceTyper/config.yaml`. Auto-creates default on first run. Migrates from old `~/.config/voicetype/` path.
- **statusbar.go** / **statusbar_appkit.go** — Menu bar icon (app mode only).
- **settings.go** / **settings_darwin.m** — Unified onboarding + preferences window. Language dropdown, hotkey recorder, mic selection. Opens as modal (onboarding) or from menu bar (preferences).
- **sound.go** — Audio feedback via macOS system sounds (`afplay`). Max 3 concurrent.

### cgo / Objective-C

macOS platform code lives in `*_darwin.m` files with corresponding `*_darwin.h` headers. Multiple cgo frameworks are linked: CoreGraphics, Carbon, Cocoa, AppKit, Metal, Accelerate. The whisper.cpp library is statically linked from `third_party/whisper.cpp/build/`.

### Threading model

- Main goroutine: locked to OS thread, runs CFRunLoop (hotkey listener blocks here)
- App.Run goroutine: processes HotkeyEvent channel
- readLoop goroutine: one per recording session, reads PortAudio buffers
- Streamer goroutine: periodic transcription tick (stream mode only)
- Transcription goroutine: async transcribe+paste in clipboard mode

## Technical Context

- **Platform**: macOS Apple Silicon (arm64) only
- **Language**: Go 1.26+ with cgo
- **Speech model**: whisper.cpp `small` (~466MB) with Metal GPU
- **Audio capture**: PortAudio via `gordonklaus/portaudio`
- **Submodule**: `third_party/whisper.cpp` (clone with `--recurse-submodules`)
- **Config**: `~/Library/Application Support/JoiceTyper/config.yaml`
- **Default hotkey**: Fn+Shift (push-to-talk)
- **Type modes**: `clipboard` (paste via Cmd+V) or `stream` (live CGEvent keystrokes)

## Engineering Standards

- Every Go error checked and handled with `component.operation` context format
- Structured logging: `log/slog` JSON, always `component` + `operation` fields
- No `fmt.Println` (except `ListInputDevices` CLI output), no `log.Printf`, no silent error swallowing
- Interfaces at every component boundary (defined in contracts.go)
- Tests use mocks at interface boundaries
- Error format: `"component.operation: description: %w"` (e.g., `"recorder.Start: open stream: %w"`)

## Roadmap

- **v1** (done): Core push-to-talk, configurable trigger key
- **v1.5** (done): .app bundle, menu bar icon, setup wizard
- **v2** (done): Streaming type mode (experimental, default off)
- **v2.5** (done): Settings UI — language, hotkey recorder, preferences menu
- **v3**: Custom dictionary (whisper prompt + post-processing replacement)
- **v4**: Menu bar UI (Wails) for full settings management
