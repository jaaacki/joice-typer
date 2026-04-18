# Bridge Contract V1 Design

Date: 2026-04-18
Branch: `feat/v2-multi-arch-support`

## Goal

Define a versioned, neutral, always-respected contract between:

- the shared web UI
- the Go bridge/router layer
- the platform-native transport and adapters

This contract must be stronger than implementation convenience. The contract is the product boundary. UI work, platform work, and backend work are only valid if they comply with it.

## Chosen Direction

Use a schema-first bridge contract in:

- `contracts/bridge/v1/`

Protocol:

- JSON
- request / response / event
- versioned envelope

Transport:

- WebView/native message bridge on macOS
- WebView/native message bridge on Windows later

The bridge contract is not REST. It is not ad hoc `postMessage`. It is not frontend-owned and not backend-owned. It is a shared protocol.

## Why This Direction

This directly addresses the two architectural risks in the current migration:

1. UI drift
   - one shared contract prevents the macOS UI and Windows UI from growing separate semantics
2. bridge drift
   - a neutral contract prevents the web UI, Go bridge, and native transport from inventing incompatible payloads

It also gives the project a clean major-versioning story. Breaking bridge changes become explicit version bumps, not accidental regressions.

## Layers

### 1. Web UI

Location:

- `ui/`

Responsibilities:

- rendering
- forms
- local view state
- invoking bridge methods through one client
- reacting to bridge events

Forbidden:

- raw `window.webkit`
- raw `postMessage`
- platform-specific conditionals
- native API knowledge

### 2. Bridge Client

Location:

- `ui/src/bridge/`

Responsibilities:

- serialize typed requests
- assign request IDs
- wait for responses
- expose event subscriptions
- hide the native transport details from React code

There must be one bridge client entrypoint for the UI.

### 3. Contract

Location:

- `contracts/bridge/v1/`

Responsibilities:

- define the envelope
- define methods
- define events
- define errors
- define versioning rules

The contract is the single source of truth for cross-layer behavior.

### 4. Bridge Router / Mapper

Location:

- `internal/core/bridge/`

Responsibilities:

- validate incoming messages against the contract
- map contract methods to core services or platform adapters
- shape all outgoing responses and events strictly according to contract

### 5. Core Backend

Location:

- `internal/core/...`

Responsibilities:

- config policy
- runtime state
- model policy
- versioning
- app rules

The core backend is platform-agnostic.

### 6. Platform Adapters

Location:

- `internal/platform/darwin/`
- `internal/platform/windows/`

Responsibilities:

- native webview host
- permissions integration
- device hooks
- hotkeys
- clipboard paste / typing fallback
- notifications

Platform adapters may implement capabilities, but they may not define their own bridge semantics.

## Absolute Rules

These are the never-broken rules for UI manners and bridge behavior:

1. UI code may only call the bridge through `ui/src/bridge/`.
2. No raw `postMessage` calls outside the bridge client.
3. No raw `evaluateJavaScript` payload shaping outside the platform transport layer.
4. Native code may only emit contract-compliant responses and events.
5. Platform adapters may not invent ad hoc payloads.
6. Core code may not import UI code.
7. UI code may not encode platform-specific business behavior.
8. Every request must yield exactly one response.
9. Long-running commands must emit typed progress or terminal events when applicable.
10. Every bridge error must be machine-readable.
11. Additive evolution is allowed inside `v1`; breaking changes require `v2`.
12. Contract changes happen contract-first, not implementation-first.

## Protocol Shape

### Request

```json
{
  "v": 1,
  "kind": "request",
  "id": "req_123",
  "method": "config.save",
  "params": {
    "triggerKey": ["fn", "shift"]
  }
}
```

### Response

```json
{
  "v": 1,
  "kind": "response",
  "id": "req_123",
  "ok": true,
  "result": {
    "saved": true
  }
}
```

### Error Response

```json
{
  "v": 1,
  "kind": "response",
  "id": "req_123",
  "ok": false,
  "error": {
    "code": "config.invalid",
    "message": "decodeMode is invalid",
    "details": {
      "field": "decodeMode"
    },
    "retriable": false
  }
}
```

### Event

```json
{
  "v": 1,
  "kind": "event",
  "event": "runtime.state_changed",
  "payload": {
    "state": "ready"
  }
}
```

## Method Model

The contract supports three categories:

- `query`
- `command`
- `event`

Recommended initial methods:

- `bootstrap.get`
- `config.get`
- `config.save`
- `permissions.get`
- `permissions.open_settings`
- `devices.list`
- `model.get`
- `model.download`
- `runtime.get`

Recommended initial events:

- `runtime.state_changed`
- `permissions.changed`
- `devices.changed`
- `model.download_progress`
- `config.saved`

## Error Model

Every bridge error includes:

- `code`
- `message`
- `details`
- `retriable`

Free-form string-only errors are not valid bridge contract output.

## Versioning Rules

- `v1` is the first frozen major bridge contract
- additive changes only inside `v1`
- breaking changes require a new major directory such as `contracts/bridge/v2/`
- UI and backend must reject unsupported major versions explicitly

## Repo Shape

```text
contracts/
  bridge/
    v1/
      protocol.schema.json
      README.md
      methods/
      events/
      errors/

ui/
  src/
    bridge/

internal/
  core/
    bridge/

  platform/
    darwin/
    windows/
```

## Migration Intent

The current bridge is transitional and not contract-complete. This design freezes the target architecture that future work must migrate toward:

- one versioned contract home
- one UI bridge client
- one backend bridge router
- one envelope family
- no ad hoc save/event message shapes

## Non-Goals

This phase does not:

- implement Windows parity
- migrate all settings methods immediately
- introduce code generation tooling yet
- replace all current bridge code in one step

It defines the contract boundary first so implementation can proceed without drift.
