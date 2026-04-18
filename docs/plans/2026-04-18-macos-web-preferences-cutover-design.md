# macOS Web Preferences Cutover Design

Date: 2026-04-18
Branch: `feat/v2-multi-arch-support`

## Goal

Make the embedded web UI the real Preferences surface on macOS.

The web settings window must own all normal user-facing Preferences behavior. The legacy native Preferences implementation remains in the tree only as a hidden fallback for debugging and recovery during this migration.

## Why This Phase Exists

The current state is structurally correct but still split-brain:

- the contract exists
- the web shell exists
- part of the settings surface is web-backed
- native Preferences logic still owns too much real behavior

That is not a stable architecture. Keeping two real settings surfaces alive will create drift immediately. This phase removes that ambiguity by making the web UI authoritative on macOS.

## Approaches Considered

### 1. Recommended: authoritative web Preferences with hidden native fallback

Behavior:

- opening Preferences always opens the web settings window
- the web UI owns the normal settings experience
- legacy native Preferences remains available only behind a debug/env-only fallback path

Why this is the right choice:

- reaches the checkpoint the project actually wants
- avoids user-facing dual surfaces
- still leaves a recovery path during worktree testing

### 2. Hard cutover with native Preferences removed from runtime completely

Behavior:

- web UI is the only Preferences path
- native Preferences path is not callable

Why not now:

- too brittle for a first full cutover
- gains little over a hidden fallback while making recovery harder during testing

### 3. Long-lived dual surface

Behavior:

- web and native Preferences both remain normal product paths

Why rejected:

- guarantees drift
- weakens the contract boundary
- delays the exact checkpoint we want

## Chosen Direction

Adopt approach 1.

The macOS app will use the embedded web settings window as the only normal Preferences UI. The old native Preferences code will remain compiled and available only through a non-user-facing debug fallback.

## Product Boundary

This phase preserves the earlier bridge rules:

1. UI talks only through `ui/src/bridge/`
2. `internal/core/bridge` owns request/response/event semantics
3. `internal/platform/darwin` implements native capabilities, not bridge policy
4. no user-facing split settings flow
5. no raw native transport access from React code

## Functional Scope

The web Preferences UI must fully cover the current macOS Preferences surface:

- trigger key display and editing
- input device selection
- device refresh and live device updates
- sample rate
- sound feedback
- model size
- language
- decode mode
- punctuation mode
- vocabulary
- permission status
- open system Accessibility settings
- open system Input Monitoring settings
- model state display
- model actions:
  - download
  - delete
  - use/select
- live runtime badge and relevant status updates

## What Stays Native

This is still not a rewrite of the app runtime.

The following remain native/backend responsibilities:

- webview hosting
- config persistence
- permission polling and OS integration
- device enumeration and recorder refresh
- model filesystem actions and validation
- download execution and progress reporting
- runtime restart signaling
- hotkey registration and actual dictation runtime

The web UI only renders and orchestrates these capabilities through the bridge.

## Required Bridge Surface

The current bridge is close, but this phase requires the following complete command/query/event surface.

### Queries

- `bootstrap.get`
- `config.get`
- `permissions.get`
- `devices.list`
- `model.get`
- `runtime.get`
- `options.get`

### Commands

- `config.save`
- `permissions.open_settings`
- `devices.refresh`
- `model.download`
- `model.delete`
- `model.use`

### Events

- `runtime.state_changed`
- `permissions.changed`
- `devices.changed`
- `model.changed`
- `model.download_progress`
- `config.saved`

## UI Contract Expectations

The web Preferences surface must obey a few rules if it is going to replace native Preferences honestly:

1. every action must be bridge-backed, not local-only
2. saves must report real contract success/failure
3. buttons for model and permissions must use command methods, not hidden native side effects
4. state shown in the UI must come from query/event payloads, not inferred guesses
5. select/dropdown options must come from contract-backed option lists, not hard-coded duplicated arrays

## Native Fallback Policy

The fallback exists only for debugging and recovery during the migration period.

Rules:

- fallback is not the default Preferences path
- fallback is not presented in normal UI
- fallback must be opt-in via debug/env behavior
- fallback may be removed later once the web path is proven

## Error Handling

This phase depends on explicit contract errors.

All bridge commands used by the web Preferences UI must return machine-readable errors with stable codes. The UI should surface those failures truthfully in status text and leave the window usable after failure.

Important command-specific failures:

- invalid config save payload
- config load/save failure
- permission target invalid
- permission settings open failure
- device enumeration/refresh failure
- model unavailable
- model download/delete/use failure

## Testing Strategy

This phase should be validated in four layers:

### 1. Contract/backend tests

- router tests for new commands and explicit error codes
- Darwin bridge service tests for new command wiring

### 2. Build-infra/source boundary tests

- generated contract outputs are current
- UI still respects the bridge boundary
- native fallback remains hidden from the normal Preferences path

### 3. Frontend build verification

- the embedded frontend still builds cleanly
- the new settings screen compiles against generated contract outputs

### 4. App packaging verification

- `make app`
- `make build-windows-amd64`

Runtime manual testing will happen after this checkpoint when the user tests the worktree `.app`.

## Success Criteria

This phase is complete when all of the following are true:

1. opening Preferences on macOS goes to the web UI by default
2. all normal Preferences behavior is available from the web UI
3. model/device/permission actions are bridge-backed commands
4. native Preferences is no longer a user-facing parallel path
5. the hidden native fallback still exists for debugging/recovery
6. full automated verification passes in the worktree

## Non-Goals

This phase does not include:

- onboarding rewrite
- Windows implementation
- deleting native Preferences code
- moving dictation runtime into React
- broad UI redesign beyond what is needed for parity and correctness
