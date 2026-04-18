# Web UI Structural Reorg Design

Date: 2026-04-18
Branch: `feat/v2-multi-arch-support`

## Goal

Prepare the repository for a shared web UI desktop shell without changing runtime behavior yet.

This phase is structural only. It does not introduce a webview runtime, does not change the macOS product behavior, and does not make the Windows desktop runtime real yet. Its purpose is to create a responsibility-oriented repository shape that supports:

- a shared frontend UI layer
- native platform adapters underneath
- a single shared Go backend/core
- lower version drift and UI drift across macOS and Windows

## Chosen Direction

Use a responsibility-oriented structure:

- `cmd/joicetyper/`
- `internal/core/`
- `internal/platform/`
- `ui/`
- `assets/`
- `packaging/`
- `third_party/`

This was chosen over deliverable-oriented layouts because it keeps Go package boundaries cleaner and makes later backend/frontend separation more explicit.

## Why This Direction

The project is moving toward:

- a shared rich web UI
- native method mapping underneath
- platform-specific adapters only for OS integrations

That means the repository should optimize for separation of responsibilities, not for the current macOS-only implementation layout.

The existing structure is cleaner than before, but it still reflects a native-shell-first organization. If a shared UI is added without another structural pass, the frontend and packaging concerns will be bolted onto the repo instead of fitting naturally into it.

## Target Repository Shape

### Top-level

- `cmd/joicetyper/`
  - desktop application entrypoint
- `internal/core/`
  - shared backend logic
- `internal/platform/`
  - OS adapters and build-tag façades
- `ui/`
  - future shared frontend source and tooling
- `assets/`
  - icons, plist/manifests, static packaged resources
- `packaging/`
  - platform packaging scripts/templates
- `third_party/`
  - external vendored code such as `whisper.cpp`

### Core packages

Planned mapping from the current structure:

- `internal/app` -> `internal/core/runtime`
- `internal/config` -> `internal/core/config`
- `internal/logging` -> `internal/core/logging`
- `internal/version` -> `internal/core/version`
- `internal/transcription` -> `internal/core/transcription`
- `internal/audio`
  - either remains temporarily as-is or moves under `internal/core/audio`

Recommendation:
- move audio only if the shared backend/policy boundary stays clear
- otherwise keep it where it is temporarily and finish the UI/platform split first

### Platform packages

- `internal/platform/darwin/`
  - macOS integrations only
- `internal/platform/windows/`
  - future Windows integrations only
- `internal/platform/`
  - thin façade layer selected by build tags

Platform packages should contain native integration code only:

- hotkeys
- notifications
- paste and typing fallback
- power events
- native settings/window bridges
- packaging/runtime glue where required

They should not become a second copy of backend logic.

### UI and packaging homes

Create these now, even if they are mostly scaffolding:

- `ui/`
  - reserved for shared frontend source later
- `assets/icons/`
- `assets/macos/`
  - `Info.plist.tmpl`
  - macOS static resources
- `assets/windows/`
  - reserved for manifests/icons/resources later
- `packaging/macos/`
- `packaging/windows/`

## Boundary Rules

These rules are the main reason for the reorg:

1. `internal/core` must not depend on `ui/`
2. `ui/` does not import Go packages directly
3. `ui/` communicates through a narrow bridge surface
4. `internal/platform/*` may depend on `internal/core`
5. `internal/core` must not depend on `internal/platform/*`
6. `internal/platform/*` should remain integration-focused, not orchestration-heavy

These rules are what will prevent the future shared web UI from tangling with the backend and platform layers.

## Migration Strategy

The reorg should happen in 3 structural phases.

### Phase 1: Boundary-first reorg

- move packages into the responsibility-oriented layout
- keep runtime behavior identical
- preserve current macOS build/runtime
- minimize wrappers and compatibility shims

### Phase 2: Frontend slotting

- create `ui/`, `assets/`, and `packaging/`
- move root assets and templates into their permanent homes
- keep the current build flow working
- do not introduce any web runtime yet

### Phase 3: Bridge preparation

- define the backend/frontend bridge seam in Go
- separate desktop runtime state/events from platform glue
- leave actual webview runtime introduction for a later phase

## Explicit Non-Goals

This phase does not:

- introduce Wails or another webview runtime
- replace the existing macOS runtime
- implement the Windows desktop runtime
- change end-user features
- redesign the app UI

## Risks

Main risks:

- import-path churn
- breaking build tags or cgo assumptions
- packaging path breakage while moving assets
- letting structural changes accidentally alter behavior

Mitigations:

- move in vertical slices
- keep root `Makefile` initially and repoint paths
- verify after the structural checkpoint with:
  - `go test ./...`
  - `go build ./cmd/joicetyper`
  - `make app`
  - `make dmg`
  - `make build-windows-amd64`

## Success Criteria

The structural checkpoint is successful if:

- the repository builds and tests as before
- macOS app packaging still works
- Windows bootstrap build still works
- the repo root is materially cleaner
- `ui/`, `assets/`, and `packaging/` exist with meaningful purpose
- there is no user-facing behavior change

## Recommendation

Proceed with the responsibility-oriented reorg first, stop at a clean structural checkpoint, then plan the shared web UI bridge and runtime migration separately.
