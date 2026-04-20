# Windows Multi-Arch Support Design

Date: 2026-04-20
Branch: `feat/windows-arch-support`

## Goal

Add a real Windows desktop implementation with installer support while preserving the shared web UI and shared bridge contract as the architectural authority.

This phase must not create a Windows silo. If Windows needs a capability that does not exist in the current architecture, that capability is introduced into the shared contract and shared core first, and only then implemented by the Darwin and Windows adapters as appropriate.

## Non-Negotiable Constraint

The bridge contract is the authority.

Rules:

1. `contracts/bridge/v1/` remains the source of truth for UI/native/backend interaction.
2. The shared web UI in `ui/` is the only Preferences UI package.
3. `internal/core/...` owns shared behavior and data shaping.
4. `internal/platform/darwin` and `internal/platform/windows` are adapter implementations under the same shared interfaces.
5. No Windows-only bridge methods, payloads, or hidden side channels are allowed.
6. If Windows requires new concepts, those concepts are added to the shared architecture first, not buried inside the Windows adapter.

## What This Phase Includes

- a real Windows adapter under `internal/platform/windows`
- a Windows webview host using the shared embedded UI
- Windows implementations for:
  - global hotkey capture and registration
  - audio input/device enumeration and refresh support through the existing shared surface
  - paste-first dictation insertion with synthetic typing fallback
  - notifications
  - system tray/menu integration
  - settings/open-preferences hosting
  - relevant power/runtime hooks
- a proper Windows installer
- a new shared Logs page in Preferences for both macOS and Windows

## What This Phase Does Not Include

- a second Windows-only Preferences UI
- migration away from the shared web UI
- speculative `v2` bridge redesign
- deleting the Darwin implementation
- onboarding redesign
- visual refinement beyond what is needed to support parity and correctness

## Existing Architecture Baseline

The current application already has the right broad shape:

- shared bridge contract in `contracts/bridge/v1/`
- shared bridge router/service in `internal/core/bridge`
- shared core packages under `internal/core/...`
- Darwin adapter in `internal/platform/darwin`
- shared embedded frontend in `ui/`

That is the correct foundation for Windows. The Windows work should extend this architecture, not bypass it.

## Approaches Considered

### 1. Recommended: shared contract + shared web UI + real Windows adapter

Behavior:

- same embedded frontend on both platforms
- same request/response/event bridge contract on both platforms
- Darwin uses `WKWebView`
- Windows uses `WebView2`
- each platform adapter implements OS-native behavior under the same shared interfaces
- installer is part of the Windows packaging flow from the start

Why this is the right choice:

- preserves the architecture already paid for
- keeps parity pressure on the contract instead of hiding differences
- gives the cleanest path to long-term multi-platform support

### 2. Bootstrap Windows host first, runtime parity later

Behavior:

- add Windows webview host and installer first
- defer hotkey/paste/runtime features

Why not chosen:

- creates a misleading partial checkpoint
- delays the hardest integration work
- increases the chance of a shallow Windows shell drifting from the real product

### 3. Windows-specific bridge and shell optimized for speed

Behavior:

- create a Windows-tailored bridge and host path
- retrofit contract alignment later

Why rejected:

- directly violates the no-drift rule
- would create exactly the architecture split we are trying to avoid

## Chosen Direction

Adopt approach 1.

The shared contract and shared web UI remain the product surface. Windows support is implemented as a new adapter and packaging target under that contract.

## Architecture

### Shared layers

- `contracts/bridge/v1/`
  - authoritative bridge methods, events, and error codes
- `ui/`
  - one embedded React/TypeScript Preferences UI
- `internal/core/bridge`
  - shared bridge protocol, router, service dependencies, contract errors
- `internal/core/config`
  - shared config and path policy where possible
- `internal/core/runtime`
  - shared runtime-facing contracts
- `internal/core/logging`
  - shared logger setup and log access helpers

### Platform adapters

- `internal/platform/darwin`
  - macOS host and OS integrations
- `internal/platform/windows`
  - Windows host and OS integrations

### Packaging

- `packaging/windows`
  - installer definition, assets, and Windows packaging scripts

## Webview Host Model

There is one shared UI package, not one shared native webview implementation.

- macOS host: `WKWebView`
- Windows host: `WebView2`

Both hosts must:

- render the same embedded HTML/CSS/JS UI
- inject the same bridge bootstrap payload shape
- deliver the same contract messages
- surface transport warnings explicitly instead of silently swallowing them

## Windows Adapter Responsibilities

The Windows adapter must implement the shared product capabilities without changing the contract shape.

### Required capabilities

- settings window host using `WebView2`
- system tray/menu integration
- notifications
- global hotkey registration
- hotkey capture support for Preferences
- paste-first dictation insertion
- synthetic typing fallback when paste is not possible
- permission/status integration as required by Windows behavior
- device enumeration and refresh
- log file location and bridge-backed log updates
- installer-facing app packaging hooks

### Allowed Windows-specific code

Windows-specific code is allowed only inside the adapter or packaging layer when implementing already-shared capabilities.

Examples:

- Win32/WebView2 hosting
- Windows clipboard interaction
- Windows input simulation
- installer scripts/assets

### Not allowed

- Windows-only Preferences payloads
- Windows-only hidden bridge messages
- UI code branching into Windows-specific transport semantics
- contract bypasses from the Windows host into React

## Shared Logs Page

This phase also adds a new Logs page for both macOS and Windows.

### Product behavior

- new `Logs` navigation item in Preferences
- live read-only tail view of the last `500` lines
- `Copy Full Log` action for the complete log file
- no editing

### Shared contract additions

Queries/commands:

- `logs.get`
- `logs.copy_all`

Events:

- `logs.updated`

### Payload expectations

`logs.get` returns:

- tail text for the last `500` lines
- `truncated`
- `byteSize`
- `updatedAt`

`logs.copy_all` returns:

- full log text

Why shared:

- log viewing is a product feature, not a Windows workaround
- the same UI and same contract should work on both platforms

## Bridge Contract Changes Expected

The current contract is close but not complete for this phase.

Shared additions likely needed:

- logs methods/events described above
- any runtime/permission/paste/hotkey concepts that prove necessary for Windows parity

Rule:

- additions inside `v1` must be additive only
- breaking changes require a new contract version

## Error Handling

Windows support must preserve the explicit contract error discipline already established for macOS web Preferences.

Requirements:

- all bridge command failures must return machine-readable contract errors
- no silent adapter fallback that reports success
- transport failures must be logged explicitly
- installer/runtime prerequisites that are missing must surface as real product errors, not obscure host failures

Expected Windows-specific failure classes that still use shared error shape:

- webview host unavailable
- permission/settings open failure
- hotkey registration/capture failure
- paste/typing insertion failure
- notification failure
- installer/bootstrap prerequisite failure where relevant

These may need new shared error codes if the current catalog is not expressive enough.

## Installer Requirement

Windows is not shipping as an ad hoc loose binary in this phase.

Goal:

- produce a real installer-backed Windows distribution

Packaging design principles:

- install the application and required assets cleanly
- integrate the shared embedded UI bundle through the normal build path
- fit the current versioning model driven by `VERSION`
- keep packaging artifacts in `packaging/windows`

The exact installer technology can be finalized during implementation, but it must not require a second product architecture.

## Testing Strategy

### 1. Shared contract/core tests

- generated contract outputs remain current
- router/service tests cover any new logs or Windows-driven shared additions
- log helper tests cover tail/full-copy behavior

### 2. Adapter tests

- Windows adapter unit tests for contract dependency wiring
- Darwin regression tests where shared contract additions affect both adapters

### 3. Build/package verification

- current macOS app packaging still passes
- Windows build target compiles
- installer build succeeds

### 4. Manual verification targets

For Windows:

- Preferences opens
- logs page renders and copies full log
- hotkey capture works
- dictation insertion prefers paste and falls back to typing
- device refresh and model actions work
- installer installs and launches cleanly

## Success Criteria

This phase is complete when:

1. Windows runs the same embedded Preferences UI as macOS
2. Windows functionality is delivered through the shared contract, not a side channel
3. missing Windows concepts, if any, were added to the shared architecture first
4. a shared Logs page exists and works on both platforms
5. Windows supports the core product behavior expected from JoiceTyper
6. a proper Windows installer is produced
7. macOS remains compatible with the updated shared contract

