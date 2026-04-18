# JoiceTyper Web Preferences UI-SPEC

Date: 2026-04-18
Status: Approved working spec for the plain-function-first phase
Scope: macOS web Preferences cutover in the `v2-multi-arch-support` worktree

## Purpose

This spec freezes the functional UI contract for the embedded web Preferences window.

This phase is intentionally plain. The goal is to make the web Preferences path correct, truthful, and stable before pushing visual polish. The UI should behave like a compact desktop utility, not a styled marketing surface.

## Phase Priorities

1. Correctness
2. Truthful status and errors
3. Stable information hierarchy
4. Desktop-friendly interaction behavior
5. Visual polish later

## Non-Goals

This phase does not attempt to define:

- final brand language
- polished motion design
- premium desktop styling
- custom illustration or decorative visuals
- Windows-specific visual adaptation
- onboarding UI redesign

## Design Principles

### 1. Functional First

Every visible control must map to a real bridge-backed capability.

No placeholder interaction.
No fake success.
No decorative UI that makes operational state harder to read.

### 2. Truthful At All Times

Status text, badges, progress, and errors must reflect real backend/native state.

The UI must not:

- claim save success before native confirmation
- claim a model action started or completed when it has not
- hide a failure behind stale optimistic text

### 3. Predictable Desktop Utility Layout

The window should be easy to scan in repeated use.

Users should quickly find:

- runtime state
- capture settings
- transcription settings
- permission blockers
- model actions
- save/apply status

### 4. Keep The Surface Small

Only expose controls the user needs to operate the app.

Do not introduce settings taxonomy, tabs, advanced drawers, or visual complexity unless the current structure becomes insufficient.

## Window Structure

The Preferences window is a single-screen utility view with these regions:

1. Header
2. Main settings grid
3. Footer action/status strip

### Header

Purpose:

- identify the window
- show current runtime state
- show app version

Contents:

- product label / section eyebrow
- title: `JoiceTyper Preferences`
- runtime state badge
- version chip

Rules:

- runtime badge must always be visible
- version must be visible but visually secondary

### Main Grid

Use a simple card/grid layout with clearly separated sections.

Sections:

1. Capture
2. Transcription
3. Vocabulary
4. Permissions
5. Model Cache

Rules:

- each section owns one domain only
- controls should stay close to the status they affect
- destructive actions stay in the section they belong to
- wide text content like vocabulary spans full width

### Footer

Purpose:

- expose the primary commit action
- show the latest truthful status line

Contents:

- status text
- primary button: `Save and Reload`

Rules:

- footer status must summarize current action/failure/success
- primary button must remain visually prominent
- footer must not contain unrelated actions

## Component Inventory

## Runtime Badge

Shows current runtime state from bridge events.

Allowed states:

- `ready`
- `recording`
- `transcribing`
- `no_permission`
- `dependency_stuck`
- any future contract-backed state

Rules:

- text comes from contract state, not inferred UI state
- badge color may vary later, but wording must stay explicit

## Field Rows

Used for normal settings inputs.

Each field row includes:

- uppercase/small label
- one primary control
- optional adjacent action button

Examples:

- trigger keys
- input device
- sample rate
- model size
- language
- decode mode
- punctuation mode

Rules:

- labels are stable and terse
- controls should not shift layout significantly when values change

## Hotkey Capture Control

The trigger key field is a stateful control, not a plain text input.

States:

- idle
- recording
- confirmable
- failed

UI behavior:

- idle shows current hotkey plus `Change Hotkey`
- recording shows captured display plus `Cancel` and `Confirm Hotkey`
- confirm button disabled until a valid capture exists

Rules:

- no free-form text editing for hotkeys
- capture state must reset truthfully on cancel and close

## Device Selector

Uses a select control when device list data exists.

Fallback:

- plain input only when the device list is unavailable

Adjacent action:

- `Refresh Devices`

Rules:

- `System default` must be the explicit empty/default option
- device refresh must update the visible list and status text

## Enum Selects

Use typed selects for:

- model size
- language
- decode mode
- punctuation mode

Rules:

- option lists come from the contract, not hardcoded component-local arrays
- selected values must always remain contract-valid

## Permissions Panel

Shows two explicit permission statuses:

- Accessibility
- Input Monitoring

Actions:

- `Open Accessibility`
- `Open Input Monitoring`

Rules:

- permission status must show current truth, not assumed truth
- buttons must trigger explicit bridge commands

## Model Cache Panel

Purpose:

- show current cached model state
- separate cache truth from action target

Contents:

- action target
- cached snapshot
- readiness
- model path or unavailability text

Actions:

- `Download Model`
- `Delete Model`
- `Use Model`

Rules:

- displayed target and acted-on target must never drift
- deleting the active model must be blocked
- delete requires a visible two-step confirmation
- model actions must update status text and contract-backed state

## Vocabulary Editor

Single large textarea.

Rules:

- spans full width
- no rich text
- no token chip UI in this phase
- placeholder should explain expected content briefly

## Status and Feedback Rules

The UI must have one primary status channel:

- footer status text

Optional local emphasis inside a section is allowed later, but this phase uses one global operational status line.

### Success Rules

Do not show success until native confirmation exists.

Examples:

- save: only after a real success response or `config.saved`
- model action: only after command success or matching event

### Error Rules

Bridge errors must surface as clear operational text.

Use:

- contract error message
- retry hint only when the error is marked retriable

Do not:

- swallow errors
- replace explicit backend messages with vague generic text
- rely on native notifications as the only visible error surface

### Progress Rules

Long-running work uses the footer status line with real progress text.

Current example:

- model download progress

Rules:

- progress must come from contract events
- do not simulate progress in the UI

## Save Behavior

`Save and Reload` is the only primary commit action in this phase.

Rules:

- sends the full config snapshot through the bridge
- enters a pending state immediately
- only reports success after native confirmation
- may close the window only after a successful native save flow

## Desktop Behavior Rules

### Reentrancy

Only one Preferences window should be active at a time.

Rules:

- opening Preferences again should focus or reuse the existing window
- in-progress edits must not be silently discarded by accidental reentry

### Close Semantics

Closing the window should:

- clear transient capture state
- release webview temp assets
- clear the Preferences-open guard

### Responsiveness

The window must remain usable on narrower widths, but it is desktop-first.

Rules:

- multi-column layout may collapse to one column
- footer may stack vertically on smaller widths
- no mobile-specific redesign in this phase

## Content Tone

Use plain operational language.

Preferred:

- `Input devices refreshed.`
- `Config saved. JoiceTyper is reloading the runtime.`
- `Confirm deleting medium model.`

Avoid:

- cheerleading
- conversational filler
- marketing phrasing
- vague statements like `Something went wrong`

## What Stays Intentionally Undecided

These are explicitly deferred to later visual passes:

- final typography system
- brand color system
- dark mode strategy
- animation and motion language
- premium surface styling
- iconography system
- advanced visual hierarchy refinements

## Acceptance Criteria For This UI Phase

The web Preferences UI is acceptable for this phase when:

1. Every visible control maps to a real bridge-backed capability.
2. Save, permission, device, model, and hotkey flows are truthful.
3. There is no user-facing dependency on the old native Preferences UI.
4. The layout is easy to scan without visual redesign work.
5. The UI remains intentionally plain and stable enough for real app testing.

