# macOS Packaging

This directory contains macOS packaging templates and release/update metadata.

Current roles:
- `sparkle-appcast.xml.tmpl`: template for Sparkle appcast generation
- `release.env.example`: example release-only configuration inputs

Normal development remains credential-free:
- `make app`
- `make dmg`

Release/update targets are separate and fail closed when required inputs are missing:
- `make mac-stage-sparkle`
- `make mac-release-app`
- `make mac-release-archive`
- `make mac-notarize-release`
- `make mac-appcast`
- `make mac-release-artifacts`

Local secret/config inputs are intentionally untracked:
- `packaging/macos/release.env.local`
- `packaging/macos/*.private`
