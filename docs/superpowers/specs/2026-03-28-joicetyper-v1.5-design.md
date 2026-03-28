# JoiceTyper v1.5 — App Bundle + Onboarding

## Overview

Wrap the existing VoiceType Go engine in a proper macOS .app bundle with a menu bar icon and first-run setup wizard. No full UI — just enough native AppKit to eliminate the need for Terminal.

Two modes from the same binary:
- **App mode** (launched from `JoiceTyper.app`) — menu bar icon, setup wizard, no terminal output
- **Terminal mode** (launched as `./voicetype`) — current dev behavior, slog JSON to stderr

## What's New in v1.5

### 1. macOS .app Bundle

```
JoiceTyper.app/
└── Contents/
    ├── Info.plist
    ├── MacOS/
    │   └── JoiceTyper
    └── Resources/
        └── icon.icns
```

- `Info.plist` sets `LSUIElement = true` (no Dock icon), declares bundle ID `com.joicetyper.app`
- Ad-hoc code signed (`codesign --force --sign -`) so Accessibility permission survives rebuilds
- `make app` target produces the bundle

### 2. Menu Bar Icon (NSStatusItem)

Bubble J icon — a "J" inside a speech bubble. Color changes for state:

| State | Color | Dropdown Menu |
|-------|-------|---------------|
| Loading | Grey | "Loading model..." (disabled) |
| Ready | Green | "Ready — Fn+Shift to dictate", separator, "Quit JoiceTyper" |
| Recording | Red | "Recording...", separator, "Quit JoiceTyper" |
| Transcribing | Blue | "Transcribing...", separator, "Quit JoiceTyper" |

Implemented as `NSStatusItem` with `NSMenu`. Icon is an `NSImage` drawn programmatically (Bubble J SVG path rendered via Core Graphics), tinted for each state.

### 3. First-Run Setup Wizard

A single `NSWindow` (480x400px, fixed, centered) shown on first launch. All 4 steps visible top to bottom, with status indicators.

**Step 1 — Accessibility Permission:**
- Calls `AXIsProcessTrustedWithOptions` with prompt
- Polls every 2 seconds to detect when user grants permission
- Shows "Granted" checkmark or "Open System Settings" button
- Continue button disabled until granted

**Step 2 — Select Microphone:**
- `NSPopUpButton` dropdown populated from PortAudio device list (input devices only)
- Pre-selects system default device
- User picks one, saved to config

**Step 3 — Download Speech Model:**
- `NSProgressIndicator` (determinate) showing download progress
- Label shows "289 MB / 466 MB — 62%"
- Uses existing `ensureModel` function with a progress callback
- If model already exists, shows "Already downloaded" instantly

**Step 4 — Ready:**
- Checkmark appears
- "Continue" button becomes "Start JoiceTyper"
- Click closes wizard, writes config, activates menu bar icon

**First-run detection:** config file `~/.config/voicetype/config.yaml` does not exist.

### 4. macOS Notification

On first launch after setup completes: "JoiceTyper is ready. Hold Fn+Shift to dictate." via `NSUserNotification` (or `UNUserNotificationCenter` on modern macOS). Only on first launch — subsequent launches just show the green icon.

### 5. Whisper.cpp Stderr Suppression (App Mode)

In app mode, redirect file descriptors 1 and 2 to the log file before loading the whisper model. This captures all whisper.cpp C-level output that bypasses slog. Terminal mode leaves stderr untouched.

## Architecture

### State Callback

The engine (`App`) emits state changes via a callback. The status bar subscribes in app mode. Terminal mode ignores it.

```go
type AppState int

const (
    StateLoading AppState = iota
    StateReady
    StateRecording
    StateTranscribing
)

type App struct {
    // ... existing fields ...
    onStateChange func(AppState)
}
```

`handlePress` calls `onStateChange(StateRecording)`. `handleRelease` calls `onStateChange(StateTranscribing)`. `transcribeAndPaste` calls `onStateChange(StateReady)` when done. The status bar Objective-C code receives these and updates the icon.

### Mode Detection

```go
func isAppMode() bool {
    exe, _ := os.Executable()
    return strings.Contains(exe, ".app/Contents/MacOS")
}
```

In `main.go`:
```
if isAppMode() {
    runAppMode(cfg, logger)    // setup wizard + menu bar
} else {
    runTerminalMode(cfg, logger) // current behavior
}
```

Both modes call the same engine — `NewApp`, `app.Run`, etc. The difference is the UI surface.

### File Structure (New Files)

```
~/voicetype/
├── statusbar.go              # Go: AppState type, state callback wiring
├── statusbar_darwin.h        # C: status bar function declarations
├── statusbar_darwin.m        # ObjC: NSStatusItem, NSMenu, icon rendering
├── setup.go                  # Go: first-run detection, setup orchestration
├── setup_darwin.h            # C: setup window function declarations
├── setup_darwin.m            # ObjC: NSWindow with 4-step setup UI
├── notification_darwin.h     # C: notification function declaration
├── notification_darwin.m     # ObjC: NSUserNotification post
├── Info.plist                # App bundle metadata
├── icon.icns                 # Bubble J app icon
└── ... (existing files unchanged)
```

### Modified Files

- `main.go` — add `isAppMode()`, branch to `runAppMode` or `runTerminalMode`
- `app.go` — add `AppState` type, `onStateChange` callback, emit state in handlePress/handleRelease/transcribeAndPaste
- `Makefile` — add `app` target that builds the .app bundle with code signing
- `CLAUDE.md` — update for v1.5

### Unchanged Files

All engine components: `contracts.go`, `config.go`, `recorder.go`, `transcriber.go`, `paster.go`, `hotkey.go`, `sound.go`, `logger.go`

## Build

```bash
# Development (terminal mode)
make build         # produces ./voicetype

# Production (app bundle)
make app           # produces JoiceTyper.app/
```

The `app` target:
1. Runs `make build`
2. Creates the .app directory structure
3. Copies binary as `JoiceTyper.app/Contents/MacOS/JoiceTyper`
4. Copies `Info.plist` and `icon.icns`
5. Generates `icon.icns` from the Bubble J SVG via `iconutil` (or embeds a pre-built .icns)
6. Code signs with `codesign --force --sign -`

## What's NOT in v1.5

- No preferences window — edit YAML for trigger key, language, model size
- No auto-update mechanism
- No Homebrew formula
- No streaming/live transcription (v2)
- No custom dictionary (v2)
- No Wails UI (v3)
