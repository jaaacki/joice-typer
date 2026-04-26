# Build infrastructure

JoiceTyper has separate build paths for local development, verification, packaging, and releases.

## Daily commands

Fresh worktrees need the whisper submodule and build before full cgo tests or app builds:

```bash
git submodule update --init --recursive
make whisper
```

```bash
make verify          # fast confidence check before pushing
make verify-mac      # verify + build a local macOS app bundle
make verify-windows  # checks Windows-facing build policy from the current host
```

## Local development builds

```bash
make build
make app-no-version-bump
```

Local builds do not mutate `VERSION`. The binary receives a dev display version such as:

```text
1.1.112-dev+b18d9d7
1.1.112-dev+b18d9d7.dirty
```

The About pane and startup logs show this full display version so it is clear which checkout produced the app.

## Release builds

Release builds use the clean version from `VERSION`, for example:

```text
1.1.112
```

Release targets are intentionally stricter than dev targets. They should verify the git tag, generated bridge contract, signing/notarization configuration, packaging inputs, and staged runtime artifacts.

## File layout

```text
Makefile                  # top-level entry points
scripts/make/version.mk   # base/dev/release version variables
scripts/make/macos.mk     # macOS build/package/release targets
scripts/make/windows.mk   # Windows build/package/release targets
scripts/make/verify.mk    # shared verification targets
scripts/release/          # platform release helper scripts
packaging/                # platform packaging metadata
docs/build/               # developer build documentation
```

When changing build behavior, update the matching buildinfra tests under `internal/buildinfra/` so the policy stays enforced.
