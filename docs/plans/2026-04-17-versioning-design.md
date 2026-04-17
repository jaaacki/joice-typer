# Versioning Design

## Goal

Establish a single source of truth for JoiceTyper versioning so local builds, the macOS app bundle, CLI-visible version output, and GitHub-style releases all use the same version value.

## Decision

Use a checked-in `VERSION` file as the canonical version source.

- `VERSION` contains a semantic version such as `1.0.0`
- release tags use `v1.0.0`
- release automation validates that the Git tag matches `VERSION`
- build tooling injects the version into Go binaries and generated app metadata

Git tags remain the release marker, but they are not the primary version source.

## Why This Approach

This repository builds a Go binary, wraps it in a macOS `.app`, and packages a `.dmg`. Those flows should not depend on Git metadata always being present at build time.

A checked-in `VERSION` file gives:

- deterministic local builds
- deterministic CI builds
- version visibility in source archives and detached checkouts
- easy code review for version bumps

Using tags alone would make local packaging and app metadata more brittle, especially in dirty working trees, detached checkouts, or copied source directories.

## Scope

The first versioned release will be `1.0.0`.

The implementation should:

- add a root `VERSION` file with `1.0.0`
- expose the version from the Go binary
- stop hardcoding version strings in `Info.plist`
- make app and dmg packaging consume the same version source
- provide a release-validation path that checks `VERSION` against a tag like `v1.0.0`

## Source Of Truth Model

### Canonical value

`VERSION`

Example:

```text
1.0.0
```

### Derived values

- Git tag: `v1.0.0`
- `CFBundleShortVersionString`: `1.0.0`
- `CFBundleVersion`: `1.0.0`
- Go runtime/build variable: `1.0.0`

## Build Strategy

The build should read `VERSION` once and pass it through to all relevant outputs.

Recommended implementation:

- add a small Go source file with defaults like `var Version = "dev"`
- inject `Version` during `go build` with `-ldflags "-X main.Version=$(VERSION)"`
- generate `Info.plist` for app packaging from a template rather than maintaining a second hardcoded version string

This keeps version data centralized while remaining simple.

## Release Strategy

Release flow should validate:

1. the current checked-in `VERSION`
2. the current Git tag, if present
3. that `v` + `VERSION` equals the release tag

If they differ, the release step should fail loudly.

This avoids “tag says one thing, app says another” drift.

## Testing

Add tests around the versioning helpers rather than relying only on manual packaging:

- version file parsing
- generated plist/version substitution
- release tag validation logic

Also verify packaging commands still produce a working `.app` and `.dmg`.

## Non-Goals

- introducing GoReleaser right now
- adding automatic remote release publishing in this same change
- building a full release pipeline before the local versioning model is correct

## Follow-Up

Once this lands, the next release workflow can safely build on top of it:

- bump `VERSION`
- commit
- tag `vX.Y.Z`
- build artifacts
- publish GitHub release from those validated values
