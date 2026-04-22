# macOS Self-Update Design

## Goal

Add a real macOS self-update path for JoiceTyper using Sparkle, while keeping normal local development unblocked when Apple signing, notarization, and release credentials are not yet available.

## Decision

Use Sparkle as the macOS updater framework and GitHub Releases as the first release-hosting target.

The first phase will wire the backend and release pipeline only:

- Sparkle integration points exist in the macOS app
- release artifacts and appcast generation exist
- signing and notarization hooks exist
- updater remains disabled in normal unsigned/local builds
- no visible `Check for Updates` UI is exposed yet

## Why This Approach

This is the simplest dependable first mac updater path for this repo:

- avoids inventing a custom updater
- keeps the app identity stable
- fits the current `.app` / `.dmg` packaging model
- does not block day-to-day development
- gives a clean path to later enable visible updater UI once Apple credentials are ready

## Non-Goals

This phase does not include:

- final Apple Developer credentials
- final notarization secrets
- automatic publishing to GitHub Releases
- enabling the updater UI in Preferences/About
- Windows updater work

## Architecture

### 1. App-Side Integration

The macOS app will include Sparkle integration points, but updater behavior will be gated behind release configuration.

Unsigned or development builds must not attempt live updates. Release-capable builds will embed the metadata Sparkle needs, but will still be safe to run without a feed when the release pipeline has not yet produced one.

### 2. Release Artifact Model

The current `app` and `dmg` targets are not sufficient for Sparkle-based updates by themselves. We need a release archive path that is:

- signed
- stable in naming
- compatible with appcast generation
- separate from local dev packaging

The canonical mac release outputs should become:

- signed `JoiceTyper.app`
- Sparkle-ready release archive
- optional `.dmg` for manual install/distribution
- generated appcast XML entry

### 3. Signing / Notarization Boundary

Development builds must stay credential-free. Release targets must fail closed if required signing or notarization inputs are missing.

That means:

- local `make app` continues to work for development
- dedicated release/update targets require env-provided credentials
- ad-hoc signing remains acceptable for local dev
- release signing, notarization, and appcast signing are explicit release-only operations

### 4. Hosting

GitHub Releases will be the initial hosting target for:

- the Sparkle update archive
- appcast XML
- manual install artifacts

The release metadata model should be generic enough that a later CDN/object-store move does not require changing app-side updater behavior.

## Files and Structure

### Packaging

Extend `packaging/macos/` to hold:

- Sparkle-related templates/config
- appcast template(s)
- signing/notarization helper inputs
- release notes / metadata placeholders if needed

### Build Scripts

Extend `scripts/make/macos.mk` with dedicated release/update targets instead of overloading normal `app` / `dmg`.

Expected direction:

- dev targets:
  - `make app`
  - `make dmg`
- release targets:
  - signed release app/archive target
  - appcast generation target
  - notarization-ready target(s)

### Scripts

Add release helper scripts under `scripts/release/` for:

- archive generation
- appcast generation
- release metadata assembly
- signing/notarization checks

## Config and Secrets Model

Use environment variables or a local ignored release config file for:

- Apple signing identity
- Apple notarization credentials
- Sparkle signing key material if needed
- GitHub release base URL / feed URL inputs

Nothing secret should be committed.

The repo should include:

- checked-in examples/templates
- ignored local secret/config files
- fail-closed validation when a release target is invoked without required inputs

## Versioning

`VERSION` remains the single source of truth.

Release artifact names, app metadata, and appcast versions must all derive from the same checked-in version to avoid drift.

## Failure Handling

### Dev Builds

- updater disabled
- no hard dependency on Sparkle release metadata
- no signing/notarization requirement

### Release Targets

- fail immediately if signing credentials are missing
- fail immediately if notarization inputs are missing for notarized targets
- fail immediately if release archive or appcast generation inputs are incomplete

## Testing Strategy

### Static / Source-Level

- buildinfra tests for mac release targets and fail-closed behavior
- source guards so dev targets do not accidentally require release credentials
- source guards so release targets derive version and artifact names consistently

### Local Functional

- local dev `make app` still works without updater credentials
- release target dry-run / metadata generation path works when required inputs are provided

### Manual Release Validation Later

Once Apple credentials exist:

- signed app builds
- notarization succeeds
- Sparkle archive/appcast artifacts are generated correctly
- installed app sees update metadata correctly

## Rollout

Phase 1 in this branch:

- backend/release plumbing only
- no visible updater UI

Phase 2 later:

- enable `Check for Updates`
- enable automatic/background update behavior
- publish real update feeds

## Recommendation Summary

Implement a real Sparkle-ready mac release/update backend now, but keep it release-gated and invisible in the product UI until the Apple registration/signing pieces are ready.
