# Microsoft Store Release — Design

Date: 2026-04-26
Status: Proposal

## Goal

Ship JoiceTyper to the Microsoft Store as a second Windows distribution
channel alongside the existing direct-download Inno Setup installer. The
Store path must coexist with — not replace — the existing
`packaging/windows/joicetyper.iss` flow.

## Packaging type: Desktop Bridge MSIX with `runFullTrust`

JoiceTyper requires capabilities that a fully sandboxed UWP container
cannot provide:

- Global low-level keyboard hook (Fn+Shift hotkey)
- Synthetic keystroke injection (`SendInput` / `keybd_event` for paste)
- Microphone capture (PortAudio over WASAPI)
- WebView2 host for the preferences UI

The standard Microsoft-supported answer is to ship the existing Win32
binary inside an MSIX package that declares the restricted capability
`runFullTrust`. This is the same packaging pattern used by 1Password,
ShareX, and most desktop utilities on the Store. Mic access is declared
as a `DeviceCapability`. WebView2 Evergreen Runtime is pre-installed on
Windows 11 and pushed by Microsoft to Windows 10, so we do not bundle
the Fixed Version Runtime — we rely on Evergreen, matching the Inno
Setup installer's behaviour after the WebView2 bootstrapper runs.

`runFullTrust` is a restricted capability, but voice-to-text utilities
that need global keyboard hooks are an established and accepted Store
use case. Microsoft's certification will scrutinise the justification
during first submission.

## Identity values from Partner Center

After reserving the app name in Partner Center
(Apps and games → New product → MSIX or PWA app), four identity values
must be plugged into the manifest. They are visible under
*Product identity* on the product page:

| Manifest field            | Source in Partner Center        | Example                               |
|---------------------------|----------------------------------|---------------------------------------|
| `Identity/@Name`          | Package/Identity/Name            | `12345JoiceTyper.JoiceTyper`          |
| `Identity/@Publisher`     | Package/Identity/Publisher       | `CN=ABCDEF12-3456-7890-ABCD-EF1234567890` |
| `Properties/PublisherDisplayName` | Publisher Display Name   | `JoiceTyper`                          |
| `Properties/DisplayName`  | Reserved app name                | `JoiceTyper`                          |

These values are stable across versions and never need to change after
the first submission.

## Package layout

```
packaging/windows/msix/
├── AppxManifest.xml.tmpl     # template with $VERSION$ + identity placeholders
└── Assets/
    ├── Square44x44Logo.png       (44x44, plus scale-100/-150/-200/-400)
    ├── Square150x150Logo.png     (150x150)
    ├── Wide310x150Logo.png       (310x150)
    ├── StoreLogo.png             (50x50)
    └── SplashScreen.png          (620x300)
```

For first iteration we generate placeholder PNGs from the existing
`assets/windows/joicetyper.ico` so the package builds end-to-end. Final
Store-quality artwork is expected before submission — see
`packaging/windows/msix/Assets/README.md` for the full required size
matrix (including `targetsize-*` variants for the start menu icon).

## Build flow

```
build/windows-amd64/                # existing staged binary + DLLs
  + packaging/windows/msix/         # AppxManifest.xml + Assets/
  -> build/windows-amd64/msix/      # staging directory MakeAppx packs from
  -> build/windows-amd64/JoiceTyper-X.Y.Z.0.msix
```

`scripts/release/windows_package_msix.ps1`:

1. Verifies `MakeAppx.exe` is on PATH (Windows SDK 10.0.22000 or newer).
2. Cleans and recreates `build/windows-amd64/msix/`.
3. Copies the staged Win32 payload (`joicetyper.exe` + every DLL listed
   in `WINDOWS_RUNTIME_STAGE_FILES`) into the staging root.
4. Copies `Assets/` verbatim.
5. Renders `AppxManifest.xml.tmpl` with the runtime identity values:
   - `$VERSION$` from `PACKAGE_VERSION`, normalised to a 4-part Store
     version (`1.1.112` → `1.1.112.0`; suffixes like `-dev+abc` are
     stripped — Store rejects non-numeric versions).
   - `$IDENTITY_NAME$`, `$PUBLISHER$`, `$PUBLISHER_DISPLAY_NAME$`,
     `$DISPLAY_NAME$` from environment variables (or
     `packaging/windows/msix/identity.local.env` if present).
6. Runs `MakeAppx.exe pack /d <staging> /p <out.msix> /o`.
7. Optionally signs with a local self-signed cert
   (`-TestSign -PfxPath <path> -PfxPassword <pwd>`) so the result can be
   sideloaded for end-to-end testing without a Store submission. Store
   submission itself requires the package to be **unsigned** — Microsoft
   re-signs with the Store certificate.

## Makefile targets

Added to `scripts/make/windows.mk`:

```
package-windows-msix
    # builds the MSIX from the existing build/windows-amd64 staging dir.
    # Requires WINDOWS_MSIX_IDENTITY_NAME, WINDOWS_MSIX_PUBLISHER,
    # WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME, WINDOWS_MSIX_DISPLAY_NAME
    # from env or packaging/windows/msix/identity.local.env.

package-windows-msix-test-sign
    # same, plus signs with a local self-signed cert for sideload
    # testing (WINDOWS_MSIX_TEST_PFX, WINDOWS_MSIX_TEST_PFX_PASSWORD).

package-windows-msix-release
    # release-check-gated variant for CI use, depends on
    # build-windows-runtime-amd64-release first.
```

## Submission flow

1. **Reserve name** in Partner Center; record the four identity values
   into `packaging/windows/msix/identity.local.env` (gitignored).
2. **Build unsigned MSIX** locally:
   `make package-windows-msix-release RELEASE_TAG=v$(cat VERSION)`.
3. **Validate locally** with the Windows App Certification Kit (WACK)
   — `Microsoft Store app` profile.
4. **Upload** the `.msix` to Partner Center → Submission → Packages.
5. **Set age rating, pricing, store listing** (screenshots, description,
   keywords, privacy policy URL). Privacy policy URL is mandatory
   because we declare the microphone capability.
6. **Justify `runFullTrust`** in the Notes for certification — paste
   the block under *runFullTrust certification notes* below verbatim.

## runFullTrust certification notes

Paste this into Partner Center → Submission → *Notes for certification*
when uploading. Microsoft reviewers read this to confirm the
restricted capability is being used as intended.

> JoiceTyper is a local push-to-talk voice-to-text utility. The user
> holds a configurable hotkey, speaks, releases, and the transcribed
> text is inserted at the cursor in whatever app currently has focus
> (e.g. Word, Outlook, the address bar of a browser). Speech
> recognition runs entirely on-device using the bundled whisper.cpp
> small model — no audio or text leaves the machine and the app makes
> no network requests at runtime.
>
> `runFullTrust` is required for three Win32 APIs that have no
> equivalent in the sandboxed UWP surface:
>
> 1. **Global low-level keyboard hook** (`SetWindowsHookEx` with
>    `WH_KEYBOARD_LL`) — needed to observe the push-to-talk hotkey
>    while another application has focus. UWP key event APIs only
>    fire while the app itself is in the foreground, which would
>    defeat the entire push-to-talk model.
> 2. **Synthetic input injection** (`SendInput` to issue Ctrl+V into
>    the foreground window) — needed to deliver the transcribed text
>    into the application that had focus when the user spoke. UWP has
>    no API to send keystrokes to a different app.
> 3. **WASAPI loopback / shared-mode microphone capture via PortAudio**
>    — declared additionally as the `microphone` `DeviceCapability`
>    so the user is prompted on first run.
>
> The app does not write anywhere outside its package container or
> `LocalAppData\JoiceTyper`. It does not load arbitrary DLLs at
> runtime, does not spawn elevated processes, and does not modify
> system settings. The whisper model is downloaded once on first run
> over HTTPS from huggingface.co (verified against a SHA-256
> manifest) into `LocalAppData\JoiceTyper\models`.
>
> Comparable utilities approved on the Microsoft Store with
> `runFullTrust` for similar reasons include ShareX (global
> screenshot hotkey), PowerToys (system-wide keybinds), and
> AutoHotkey (input injection).

## Out of scope for this design

- Auto-update on the Store path — Microsoft Store handles updates
  natively from the uploaded package version. No Sparkle/Velopack work
  required. Direct-download installer auto-update is tracked separately.
- MSIX bundle (`.msixbundle`) for multi-architecture — JoiceTyper is
  amd64-only on Windows. Single-arch `.msix` is sufficient.
- CI workflow — a dedicated `windows-store-release.yml` follows after
  the local flow is proven. The PowerShell script is written so CI can
  reuse it without rework.

## Risks

- **Certification rejection on `runFullTrust`** — primary risk. If
  rejected, fallback is direct download only on Windows; the
  microphone-capability + paste-injection combination is not feasible
  in a sandboxed package. Mitigated by citing comparable approved apps
  (e.g. ShareX) and providing a clear functional justification.
- **WebView2 Evergreen Runtime missing on stripped Windows 10
  installs** — first run would fail. The Inno Setup installer handles
  this with a bootstrapper; the Store app cannot run a bootstrapper.
  If we hit this, the option is to bundle the Fixed Version Runtime
  (~120 MB) as part of the MSIX, dramatically increasing package size.
  Decision deferred until we observe real-world failures.
- **Version normalisation** — Store rejects pre-release suffixes. The
  packaging script strips `-dev+commit` suffixes; if `BUILD_TYPE=dev`
  is ever passed to `package-windows-msix-release`, the resulting
  4-part version will be the base release version, which is wrong.
  Release path is gated by `release-check`, which guarantees the tag
  matches the clean `VERSION`, so this only matters if someone
  bypasses the gate.
