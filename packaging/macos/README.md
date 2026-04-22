# macOS Packaging

This directory contains macOS packaging templates and release/update metadata.

Current roles:
- `sparkle-appcast.xml.tmpl`: template for Sparkle appcast generation
- `release.env.example`: example release-only configuration inputs

Normal development remains credential-free:
- `make app`
- `make dmg`

Release/update targets are separate and fail closed when required inputs are missing:
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
- `mac-release-artifacts` produces the archive, dmg, and appcast under `build/macos-release/`
- `mac-publish-github-release` uploads those artifacts to the tagged GitHub release using `gh`

Preflight targets are the quickest readiness check before a real release run:
- `mac-release-preflight` validates codesign access and the Sparkle private key path
- `mac-notarize-preflight` validates that `notarytool` can access the configured keychain profile
- `mac-publish-preflight` validates `gh` authentication and that the tagged GitHub release is reachable
