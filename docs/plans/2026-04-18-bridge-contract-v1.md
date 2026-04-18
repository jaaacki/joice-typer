# Bridge Contract V1 Implementation Plan

Date: 2026-04-18
Branch: `feat/v2-multi-arch-support`
Design: [2026-04-18-bridge-contract-v1-design.md](./2026-04-18-bridge-contract-v1-design.md)

## Objective

Move the current embedded web UI bridge from ad hoc request handling to the frozen `contracts/bridge/v1/` contract without breaking the existing macOS web settings flow.

## Phase 1: Contract Skeleton

Deliverables:

- `contracts/bridge/v1/README.md`
- `contracts/bridge/v1/protocol.schema.json`
- method, event, and error directories with documented initial surface

Verification:

- schema parses as valid JSON
- contract files clearly define the envelope and required fields

## Phase 2: Backend Contract Adoption

Deliverables:

- envelope structs and router shape in `internal/core/bridge/`
- typed contract error model
- migration of current settings save/bootstrap behavior to contract-compliant request/response handling

Verification:

- backend tests for request decode, response encode, error encode
- no ad hoc message structs left on the bridge hot path

## Phase 3: Frontend Contract Adoption

Deliverables:

- single client in `ui/src/bridge/`
- request ID management centralized there
- event subscription API centralized there
- settings screen migrated to the client instead of bespoke message handling

Verification:

- source-level guard that UI does not call raw native handlers directly
- tests for response and error acknowledgement handling

## Phase 4: Platform Transport Cleanup

Deliverables:

- Darwin transport emits only contract-compliant responses/events
- no bespoke save event names or one-off response scripts outside the transport boundary

Verification:

- transport tests for request/response/event wrapping
- no raw UI payload shaping outside transport files

## Phase 5: Surface Expansion

Deliverables:

- `permissions.get`
- `devices.list`
- `model.get`
- `runtime.get`
- typed events for runtime/model updates

Verification:

- bridge parity tests for each new method
- settings UI consumes contract payloads only

## Guardrails

1. No new bridge feature is allowed unless it is first added to `contracts/bridge/v1/`.
2. No new UI/native messaging shortcut is allowed outside the contract path.
3. No platform-specific payload shape is allowed to leak into the UI.
4. Existing behavior may be staged behind the old path only until the contract path replaces it, not alongside it permanently.

## Exit Criteria

This plan is complete when:

- current web settings bridge traffic uses the `v1` envelope
- the UI uses one bridge client only
- the backend uses one router only
- Darwin transport emits only contract-compliant messages
- the contract files are the accepted source of truth for new UI/native work
