# Releases

The intended release flow is git-driven:

```text
VERSION file -> git tag -> platform builds -> signed artifacts -> GitHub Release
```

## Manual outline

```bash
# after deciding the next release version
git checkout main
# update VERSION intentionally
git commit -am "chore(release): v1.1.113"
git tag v1.1.113
git push origin main --tags
```

Release Make targets require the matching tag explicitly unless the checkout is already on that exact tag:

```bash
make mac-release-artifacts RELEASE_TAG=v$(cat VERSION)
make package-windows-release RELEASE_TAG=v$(cat VERSION)
```

A future GitHub Actions workflow can build and upload artifacts from the tag.

## macOS release requirements

A proper macOS release needs:

- stable bundle ID: `com.joicetyper.app`
- Developer ID Application signing certificate
- notarization credentials/profile
- Sparkle signing keys if auto-update is enabled
- clean release version matching the git tag

## Windows release requirements

A proper Windows release needs:

- stable Inno Setup `AppId`
- clean `AppVersion` from `VERSION`
- staged runtime DLLs and executable
- Inno Setup compiler
- Authenticode certificate if code signing is enabled

Unsigned Windows builds may trigger SmartScreen warnings.

## Microsoft Store (MSIX) submission

The Store path is additive to the Inno Setup installer — both can ship
in parallel. See `docs/plans/2026-04-26-microsoft-store-release-design.md`
for design context.

### One-time setup

1. Reserve the app name in Partner Center
   (Apps and games → New product → MSIX or PWA app).
2. From Product identity, copy four values into a new file
   `packaging/windows/msix/identity.local.env` (gitignored). Use
   `packaging/windows/msix/identity.example.env` as the template.
3. (Optional) Generate a self-signed `.pfx` for sideload testing:
   ```powershell
   New-SelfSignedCertificate -Type CodeSigningCert `
       -Subject "<EXACT Publisher value from identity.local.env>" `
       -KeyUsage DigitalSignature -FriendlyName "JoiceTyper Test Sign" `
       -CertStoreLocation Cert:\CurrentUser\My `
       -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3","2.5.29.19={text}")
   # then export to .pfx via certmgr.msc and set:
   #   $env:WINDOWS_MSIX_TEST_PFX = '...path...\test.pfx'
   #   $env:WINDOWS_MSIX_TEST_PFX_PASSWORD = '...'
   ```
   The Subject MUST match the `Publisher` field in
   `identity.local.env` exactly, otherwise sideload install fails with
   `0x800B0109` (root cert not trusted) or `0x80073CF3` (signature
   mismatch).

### Local sideload test

```bash
make build-windows-runtime-amd64-release RELEASE_TAG=v$(cat VERSION)
make package-windows-msix-test-sign RELEASE_TAG=v$(cat VERSION)
# Output: build/windows-amd64/JoiceTyper-X.Y.Z.0.msix
```

Install the resulting `.msix` via `Add-AppxPackage` (PowerShell) or by
double-clicking. The self-signed cert's public key must be installed
into `Trusted People` on the test machine first.

### Store submission build

```bash
make package-windows-msix-release RELEASE_TAG=v$(cat VERSION)
```

This produces an **unsigned** `.msix`. Microsoft re-signs Store
packages — do not pass `-TestSign` for submission builds.

### Upload checklist

- Run the Windows App Certification Kit (WACK) against the `.msix`
  with the *Microsoft Store app* profile; fix any errors.
- Upload the `.msix` to Partner Center → Submission → Packages.
- Provide a privacy policy URL (mandatory because we declare the
  `microphone` capability).
- In *Notes for certification*, justify `runFullTrust`: the global
  push-to-talk hotkey and synthetic paste injection require Win32
  APIs that are not available to sandboxed UWP apps.
- Set age rating, pricing/availability, and store listing
  (screenshots, description, keywords).

### CI workflow

`.github/workflows/windows-release.yml` triggers on tag-push (`v*`) and
on manual `workflow_dispatch`. It runs two jobs:

1. **`toolchain-smoke`** — installs MinGW (msys2), Inno Setup,
   pkg-config, and verifies `MakeAppx.exe` from a Windows SDK is
   reachable. Completes in 3-5 min and gates the release job, so a
   broken runner setup fails fast instead of after a 30-min whisper
   build.
2. **`windows-release`** — full build: caches Vulkan SDK + portaudio +
   whisper, produces the Inno Setup `.exe` (published to GitHub
   Release) and the unsigned `.msix` (uploaded as a workflow artifact
   for manual Partner Center submission).

#### Required GitHub secrets

For MSIX (set under Settings → Secrets and variables → Actions):

- `WINDOWS_MSIX_IDENTITY_NAME`
- `WINDOWS_MSIX_PUBLISHER`
- `WINDOWS_MSIX_PUBLISHER_DISPLAY_NAME`
- `WINDOWS_MSIX_DISPLAY_NAME`

If any of those is missing and the MSIX step runs, the workflow fails
fast. Set `build_msix=false` on a manual dispatch to skip MSIX (e.g.
before Partner Center reservation is complete). Tag-push triggers
build MSIX by default.

For `.exe` Authenticode signing (optional — when absent, the installer
ships unsigned and users hit SmartScreen warnings):

- `WINDOWS_AUTHENTICODE_PFX_BASE64` — base64-encoded `.pfx` file.
  Generate with `base64 -w0 cert.pfx | pbcopy` (macOS) or
  `[Convert]::ToBase64String([IO.File]::ReadAllBytes('cert.pfx'))` in
  PowerShell.
- `WINDOWS_AUTHENTICODE_PFX_PASSWORD` — `.pfx` password.

Both the staged `joicetyper.exe` and the final installer `.exe` are
signed when these are present. The MSIX is intentionally not signed in
CI — Microsoft re-signs Store packages.

### Why no auto-update wiring

The Store updates apps natively from each uploaded package version —
Sparkle/Velopack are not used on this distribution channel. The
direct-download installer's auto-update path is tracked separately.

## Automation direction

The desired endpoint is:

```text
push tag vX.Y.Z -> CI builds macOS + Windows -> GitHub Release artifacts
```

Keep local Make targets aligned with that future CI flow so the same commands can be reused by automation.
