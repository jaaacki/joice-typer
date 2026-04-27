# macOS Packaging

This directory contains macOS packaging templates and release/update metadata.

Current roles:
- `sparkle-appcast.xml.tmpl`: template for Sparkle appcast generation
- `release.env.example`: example release-only configuration inputs
- `../../assets/macos/JoiceTyper.entitlements`: hardened-runtime entitlements used for Developer ID release signing

Normal development remains credential-free:
- `make app`
- `make dmg`
- `make mac-dev-update-artifacts`
- `make mac-local-release-candidate`

`mac-local-release-candidate` builds a release-versioned app with local ad-hoc hardened-runtime signing, then produces and validates accountless release-candidate artifacts under `build/macos-local-rc/`:
- `JoiceTyper-<version>-macos.zip`
- `JoiceTyper-<version>.dmg`
- `JoiceTyper-<version>-macos.env`
- `SHA256SUMS`

The local RC validator checks bundle structure, bundled PortAudio linkage, arm64 architecture, app metadata, icon presence, hardened-runtime ad-hoc code-signature validity, empty release entitlements, ZIP contents, DMG readability, checksum consistency, metadata consistency, and package extraction/install-copy smoke. Set `MACOS_LOCAL_RC_SMOKE_LAUNCH=1 make mac-local-release-candidate` to also launch the app from a temporary DMG install copy and verify the process starts.

Accountless artifacts are intentionally not Developer ID signed or notarized. They may be valid on disk but still be rejected by Gatekeeper after download/quarantine; `spctl --assess` rejection is expected until the Developer ID notarization path is run with an Apple Developer account.

The official release signing path uses `assets/macos/JoiceTyper.entitlements` with hardened runtime when Developer ID credentials are available.

Release/update targets are separate and fail closed when required inputs are missing. The official GitHub Actions release workflow is manual-only until Apple Developer credentials and release secrets are configured:
- `make mac-release-preflight`
- `make mac-notarize-preflight`
- `make mac-publish-preflight RELEASE_TAG=vX.Y.Z`
- `make mac-stage-sparkle`
- `make mac-release-app`
- `make mac-release-archive`
- `make mac-notarize-release`
- `make mac-appcast`
- `make mac-release-artifacts`
- `make mac-publish-github-release`

Local secret/config inputs are intentionally untracked:
- `packaging/macos/release.env.local`
- `packaging/macos/*.private`

GitHub Releases remains the first hosting target:
- `mac-release-artifacts` produces the archive, dmg, appcast, and checksum manifest under `build/macos-release/`
- `mac-publish-github-release` uploads those artifacts to the tagged GitHub release using `gh`
- use a stable `MACOS_APPCAST_URL` for the embedded feed, for example `.../releases/latest/download/appcast.xml`
- use `MACOS_RELEASE_DOWNLOAD_BASE_URL` for the versioned archive URLs referenced by each generated appcast item
- when downloading Sparkle in release automation, pin `MACOS_SPARKLE_DOWNLOAD_SHA256` with the expected archive hash

Preflight targets are the quickest readiness check before a real release run:
- `mac-release-preflight` validates codesign access and the Sparkle private key path
- `mac-notarize-preflight` validates that `notarytool` can access the configured keychain profile
- `mac-publish-preflight` validates `gh` authentication and that the tagged GitHub release is reachable

Local dry-run updater validation:
- `mac-dev-update-artifacts` produces an unsigned Sparkle-style archive and appcast under `build/macos-dryrun-update/`
- this is for validating artifact shape and metadata only
- it deliberately uses placeholder URLs and `EDDSA_SIGNATURE=UNSIGNED`

GitHub Actions release automation now lives at:
- `.github/workflows/macos-accountless-rc.yml` for credential-free PR/workflow-dispatch validation of local RC artifacts
- `.github/workflows/macos-release.yml` for manual Developer ID signing, notarization, stapler validation, Gatekeeper assessment, Sparkle appcast generation, artifact validation, and GitHub Release publishing

Expected GitHub Actions secrets:
- `MACOS_DEVELOPER_ID_P12_BASE64`
- `MACOS_DEVELOPER_ID_P12_PASSWORD`
- `MACOS_CODESIGN_IDENTITY`
- `MACOS_NOTARY_APPLE_ID`
- `MACOS_NOTARY_TEAM_ID`
- `MACOS_NOTARY_PASSWORD`
- `MACOS_SPARKLE_PUBLIC_ED_KEY`
- `MACOS_SPARKLE_PRIVATE_KEY`

The workflow:
- imports the Developer ID certificate into a temporary keychain
- creates the `notarytool` profile on the runner
- writes `packaging/macos/release.env.local`
- runs the release preflight checks
- notarizes and staples the release app before the Sparkle archive is generated
- validates stapling and Gatekeeper assessment for notarized app and DMG artifacts
- signs, notarizes, and staples the release DMG before publishing
- validates app, archive, DMG, appcast, metadata, and checksum consistency before publishing
- publishes the archive, dmg, appcast, and checksum manifest to GitHub Releases

Manual accountless install smoke checklist:
- run `make mac-local-release-candidate`
- open the generated DMG and copy `JoiceTyper.app` to a temporary folder or `/Applications`
- launch the copied app and confirm the menu bar item appears
- verify onboarding can request Microphone, Accessibility, and Input Monitoring permissions; TCC grants made to an ad-hoc local build may not carry over to the future Developer ID identity
- expect Gatekeeper rejection/warnings for downloaded accountless artifacts until notarization is available
