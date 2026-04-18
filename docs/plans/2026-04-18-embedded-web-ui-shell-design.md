# Embedded Web UI Shell Design

Date: 2026-04-18
Branch: `feat/v2-multi-arch-support`

## Goal

Introduce a shared desktop UI layer authored in React and TypeScript, built into static frontend assets, and embedded into the Go binary.

This phase is about the shell and the integration seam, not a full product rewrite. The dictation engine, hotkey runtime, audio capture, transcription flow, and native integrations remain in Go and platform-specific code. The frontend is responsible for UI only.

## Chosen Direction

Use:

- React + TypeScript for frontend authoring
- a static frontend build output (`html/css/js`)
- `embed` in Go to bundle built assets into the desktop binary
- a narrow backend/frontend bridge for communication
- platform-native integrations only below the bridge

## Why This Direction

This directly addresses the two problems driving the change:

1. **UI drift**
   - one shared frontend reduces divergence between macOS and Windows
2. **version drift**
   - one Go backend + one embedded frontend build keeps versioning centralized

At the same time, it avoids the main failure mode of webview desktop apps: pushing core runtime behavior into the frontend. The frontend does not own dictation state machines, audio, hotkeys, paste, or transcription execution.

## High-Level Architecture

### Layers

#### 1. Frontend layer

Location:

- `ui/`

Responsibilities:

- settings UI
- onboarding/setup UI
- status and progress views
- future tray/popover-facing content

Technology:

- React
- TypeScript
- Vite or equivalent

Shipped form:

- compiled static assets only
- no `.tsx` source shipped to users

#### 2. Bridge layer

Location:

- likely `internal/core/bridge/`

Responsibilities:

- define the frontend/backend API contract
- map UI requests to shared backend operations
- deliver status/events back to the UI

This must be typed and intentionally small. It is the most important control point for preventing architectural drift.

#### 3. Shared backend layer

Location:

- `internal/core/...`

Responsibilities:

- runtime orchestration
- config read/write behavior
- logging
- versioning
- transcription coordination
- shared application state and policy

#### 4. Platform layer

Location:

- `internal/platform/darwin/`
- `internal/platform/windows/`

Responsibilities:

- global hotkeys
- native windows/webview hosting
- clipboard paste
- typing fallback
- notifications
- permission/state checks
- device enumeration hooks where platform-specific
- packaging/runtime integration

## Core Boundary Rules

These rules are non-negotiable if the web UI approach is going to stay clean:

1. `ui/` never talks directly to platform adapters
2. `ui/` talks only through the bridge
3. `internal/core` must not depend on `ui/`
4. `internal/platform/*` may depend on `internal/core`, never the other way around
5. platform code must not become the owner of application state
6. the frontend must not own hotkey/audio/transcription runtime logic

## Packaging Strategy

### Authoring vs shipping

The frontend is authored in React/TypeScript, but shipped as static built assets:

- `index.html`
- compiled CSS
- compiled JS

These assets are then embedded into the Go binary using `embed`.

### Why embed instead of shipping separate assets

Embedding is preferred because:

- distribution stays simpler
- the product remains closer to a single desktop binary/app artifact
- frontend and backend version alignment is easier to enforce
- packaging logic stays more controlled

Tradeoff:

- bigger binary
- frontend asset debugging in packaged builds is a little less transparent

That tradeoff is acceptable for this project.

## Recommended Migration Sequence

### Phase 1: Add the frontend toolchain

Introduce:

- `ui/package.json`
- TypeScript
- React
- Vite
- a minimal project build that outputs static assets

No runtime adoption yet beyond proving the build pipeline.

### Phase 2: Add embedded asset serving in Go

Add:

- a small embedded asset loader in Go
- a native shell path that can render a trivial embedded frontend page

This is a shell proof, not a feature migration.

### Phase 3: Define the bridge

Initial bridge candidates:

- config load/save
- permissions state
- current app status/state
- device list
- model presence/progress
- trigger-key settings

The bridge should support:

- request/response methods
- event subscription or state push

### Phase 4: Migrate one real UI surface

Best first candidate:

- settings window

Reason:

- high UI value
- low risk compared to the dictation runtime
- exercises config, devices, model state, permissions, and event updates

### Phase 5: Use the same frontend shell for Windows

Once the shell and bridge are proven on macOS, bring the Windows native shell online around the same embedded frontend.

## What Should Not Move Into the Frontend

The following should remain backend/native responsibilities:

- trigger key capture
- recorder lifecycle
- transcription execution
- bulkhead/timeout logic
- clipboard and typing fallback execution
- OS permission handling
- model validation/integrity logic

The frontend may display and configure these systems, but it must not own them.

## Recommended First Windows Strategy

Windows should not start with a separate native UI.

Instead:

- implement the Windows native shell/webview host
- implement the Windows platform adapters
- use the same embedded frontend as macOS

This is how the project avoids recreating the same settings and onboarding work twice.

## Risks

### Bridge sprawl

Risk:
- too many unstructured backend calls

Mitigation:
- define a typed bridge package early
- keep bridge additions reviewable and intentional

### Hidden platform drift

Risk:
- one shared UI but inconsistent backend semantics

Mitigation:
- shared bridge semantics first
- platform-specific behavior only where unavoidable

### Build complexity

Risk:
- frontend build, Go build, embed, and packaging become messy

Mitigation:
- keep one authoritative root build path
- do not create parallel packaging flows

### Startup ambiguity

Risk:
- frontend load failures look like native runtime failures

Mitigation:
- explicit logging around:
  - embedded asset load
  - webview host startup
  - bridge registration

### Over-migration

Risk:
- pushing runtime logic into the frontend because it feels convenient

Mitigation:
- enforce the UI-only role of the frontend

## Success Criteria For The Next Phase

The next implementation phase is successful if:

- a React/TypeScript frontend builds reproducibly
- the built assets are embedded into the Go binary
- the desktop app can render a trivial embedded UI shell
- one real UI surface can talk through the bridge
- current macOS runtime behavior remains intact
- the direction clearly supports later Windows parity

## Recommendation

Proceed with an embedded React/TypeScript shell, but keep the migration narrow:

1. toolchain
2. embedded shell proof
3. typed bridge
4. settings window migration

Do not move the dictation engine into the frontend.
