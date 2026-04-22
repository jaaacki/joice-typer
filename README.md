# JoiceTyper

Hold a key, speak, release — text appears at your cursor. Anywhere.

A lightweight, fully local voice-to-text tool for macOS. No cloud APIs, no data leaves your machine (one-time model download from Hugging Face on first run). Powered by [whisper.cpp](https://github.com/ggerganov/whisper.cpp) with Metal GPU acceleration.

## How It Works

1. Hold **Fn+Shift** (configurable)
2. Speak
3. Release — transcribed text is pasted at your cursor

Works in any app where you can type: editors, browsers, terminals, chat apps.

## Requirements

- macOS (Apple Silicon recommended)
- Homebrew
- Node.js + npm
- ~500MB disk for the whisper `small` model (downloaded on first run)

## Install

```bash
git clone --recurse-submodules https://github.com/jaaacki/joice-typer.git
cd joice-typer

make setup          # install portaudio, cmake via Homebrew
make whisper        # build whisper.cpp with Metal GPU support
make build          # build the embedded frontend, then the Go binary
```

## Run

### Terminal (development)

```bash
./build/<os>-<arch>/voicetype
```

### App bundle

```bash
make app
open JoiceTyper.app
```

Windows portable shell build:

```bash
make build-windows-amd64
```

Windows installer packaging:

```bash
make package-windows
```

Each build target now bumps the checked-in patch version in `VERSION` once per invocation before producing artifacts, so version numbers advance monotonically across macOS and Windows builds.

Windows runtime build:

```bash
make build-windows-runtime-amd64
make package-windows-runtime
```

This produces a bootstrap Windows build at `build/windows-amd64/joicetyper.exe`.
The installer path packages only the runtime build:

```bash
make build-windows-runtime-amd64
make package-windows
```

`build-windows-amd64` is the non-CGO Windows shell build.
`build-windows-runtime-amd64` is the native Windows runtime build path for whisper-backed transcription and recorder support, and requires a Windows CGO toolchain plus a local `third_party/portaudio-windows-src` checkout.
That target now builds/stages the Windows whisper runtime automatically (AVX2-enabled ggml/whisper DLLs), generates static Windows PortAudio metadata, and bundles the extra MinGW support DLLs needed at runtime.
Missing runtime DLLs, the PortAudio source checkout, or the Windows CGO toolchain now fail that target immediately instead of producing a partial package.

Frontend-only rebuild:

```bash
make frontend-build
```

Repository structure note:

- shared backend code is moving under `internal/core/`
- platform adapters live under `internal/platform/`
- future shared frontend code will live under `ui/`
- packaging resources are being organized under `assets/` and `packaging/`

## Versioning

`VERSION` is the single source of truth for releases.

- checked-in version: `VERSION`
- release tag format: `vX.Y.Z`
- app bundle and Go binary both derive their version from `VERSION`

Typical release flow:

```bash
printf '1.0.0\n' > VERSION
git add VERSION
git commit -m "release: bump version to 1.0.0"
git tag v1.0.0
make release-check
make dmg
```

macOS self-update release flow:

```bash
cp packaging/macos/release.env.example packaging/macos/release.env.local
# fill in your real signing, notarytool, Sparkle, and GitHub values

make mac-release-preflight
make mac-notarize-preflight
make mac-publish-preflight RELEASE_TAG=v$(cat VERSION)
make mac-release-artifacts
make mac-publish-github-release RELEASE_TAG=v$(cat VERSION)
```

This keeps normal development builds credential-free while generating Sparkle-ready release artifacts under `build/macos-release/` for GitHub Releases hosting.

There is also a GitHub Actions path for this release flow:
- `.github/workflows/macos-release.yml`
- it expects the mac signing/notary/Sparkle secrets documented in `packaging/macos/README.md`

On first launch, a setup wizard guides you through granting Accessibility permission, selecting a microphone, and downloading the speech model.

## Configuration

Config lives at `~/Library/Application Support/JoiceTyper/config.yaml`:

```yaml
trigger_key:
  - fn
  - shift
model_size: small        # tiny, base, small, or medium
language: "en"           # recommended default; set another explicit code if needed
sample_rate: 16000
sound_feedback: true
input_device: ""         # empty = system default
decode_mode: "beam"      # "beam" or "greedy"
punctuation_mode: "conservative" # "off", "conservative", or "opinionated"
```

### Trigger keys

Any combination of: `fn`, `shift`, `ctrl`, `option`, `cmd`

### Listing audio devices

```bash
./build/<os>-<arch>/voicetype --list-devices
```

## Model Integrity

Downloaded models are verified against pinned SHA-256 hashes (from HuggingFace Git LFS). On every startup, cached models are re-verified against the manifest. Corrupt or tampered models are quarantined and re-downloaded.

## Roadmap

- **v1** - Core push-to-talk, configurable trigger key
- **v1.5** - .app bundle, menu bar icon, setup wizard
- **v2** - Faster local transcription pipeline
- **v2.5** - Settings UI for mic/hotkey selection *(current)*
- **v3** - Custom dictionary and punctuation controls
- **v4** - Menu bar UI

## License

MIT
