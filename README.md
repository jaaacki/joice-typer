# JoiceTyper

Hold a key, speak, release — text appears at your cursor. Anywhere.

A lightweight, fully local voice-to-text tool for macOS. No cloud APIs, no network calls, no data leaves your machine. Powered by [whisper.cpp](https://github.com/ggerganov/whisper.cpp) with Metal GPU acceleration.

## How It Works

1. Hold **Fn+Shift** (configurable)
2. Speak
3. Release — transcribed text is pasted at your cursor

Works in any app where you can type: editors, browsers, terminals, chat apps.

## Requirements

- macOS (Apple Silicon / arm64)
- Homebrew
- ~500MB disk for the whisper `small` model (downloaded on first run)

## Install

```bash
git clone --recurse-submodules https://github.com/jaaacki/joice-typer.git
cd joice-typer

make setup          # install portaudio, cmake via Homebrew
make whisper        # build whisper.cpp with Metal GPU support
make build          # build the Go binary
```

## Run

### Terminal (development)

```bash
./voicetype
```

### App bundle

```bash
make app
open JoiceTyper.app
```

On first launch, a setup wizard guides you through granting Accessibility permission, selecting a microphone, and downloading the speech model.

## Configuration

Config lives at `~/.config/voicetype/config.yaml`:

```yaml
trigger_key:
  - fn
  - shift
model_size: small        # tiny, base, small, or medium
language: ""             # empty = auto-detect, or "en", "zh", etc.
sample_rate: 16000
sound_feedback: true
input_device: ""         # empty = system default
type_mode: "clipboard"   # "clipboard" (paste) or "stream" (experimental)
```

### Trigger keys

Any combination of: `fn`, `shift`, `ctrl`, `option`, `cmd`

### Listing audio devices

```bash
./voicetype --list-devices
```

## Model Integrity

Downloaded models are verified against pinned SHA-256 hashes (from HuggingFace Git LFS). On every startup, cached models are re-verified against the manifest. Corrupt or tampered models are quarantined and re-downloaded.

## Roadmap

- **v1** - Core push-to-talk, configurable trigger key
- **v1.5** - .app bundle, menu bar icon, setup wizard
- **v2** - Streaming type mode (experimental, default off)
- **v2.5** - Settings UI for mic/hotkey selection *(current)*
- **v3** - Custom dictionary
- **v4** - Menu bar UI

## License

MIT
