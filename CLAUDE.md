# VoiceType

## Goal

A lightweight, local, voice-to-text tool for macOS that replaces VoiceTypr ($50-140).
The ONLY job: press a hotkey, speak, text appears at the cursor. Wherever the cursor is - browser, IDE, chat, notes, anywhere.

## Non-Negotiable Requirements

- **Local/offline only** - no cloud APIs, no network calls, voice never leaves the machine
- **Fast** - near-zero latency between stopping speech and text appearing
- **Accurate** - uses OpenAI Whisper (open-source) via `faster-whisper`
- **Universal** - works in any app where you can type
- **Properly packaged** - NOT a rogue Python script left running in a terminal. Must be a proper macOS service/app that can be installed, started, stopped, and managed cleanly

## What We Do NOT Need

- Smart formatting / rewriting modes
- Audio/video file upload transcription
- History search or export
- Productivity tracking / streaks / statistics
- Anything beyond: hotkey -> speak -> text at cursor

## Technical Context

- **Platform**: macOS (Apple Silicon / arm64)
- **Python**: 3.14.3 (at /opt/homebrew/bin/python3)
- **Speech model**: `faster-whisper` with Whisper `base` model (configurable: tiny/base/small/medium)
- **Audio capture**: `sounddevice`
- **Global hotkey**: `pynput` (requires macOS Accessibility permissions)
- **Paste mechanism**: `pbcopy` + `osascript` simulating Cmd+V
- **Hotkey**: Ctrl+Shift+Space (toggle: press to start recording, press again to stop and transcribe)

## Current State

A working Python script (`voicetype.py`) exists with core functionality:
- Records audio via hotkey toggle
- Transcribes with faster-whisper locally
- Pastes result at cursor position
- Audio feedback (system sounds on start/stop)

**What's missing**: proper packaging. The script currently requires manually running in a terminal and leaving that terminal open. Needs to be turned into a properly managed macOS service or app.

## Packaging Direction (TO BE DECIDED)

Options to discuss with the user:
1. **macOS LaunchAgent** - background service managed via `launchctl`, starts on login, CLI commands to start/stop
2. **Menu bar app** (e.g., using `rumps`) - icon in menu bar showing state, proper Quit
3. **Native Swift app** - full .app bundle, most polished but most work
4. **py2app / PyInstaller** - bundle Python into a .app

The user has strong opinions on packaging quality. Ask before building.

## Project Structure

```
~/voicetype/
  voicetype.py        # Core voice-to-text logic
  requirements.txt    # Python dependencies
  setup.sh           # Initial venv + dependency setup
  .venv/             # Python virtual environment (already created, deps installed)
```

## Dependencies (installed in .venv)

- faster-whisper
- sounddevice
- numpy
- pynput
