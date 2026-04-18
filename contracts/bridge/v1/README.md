# Bridge Contract V1

This directory is the source of truth for the embedded web UI bridge contract.

Rules:

1. All cross-layer UI/native/backend behavior must be defined here first.
2. `v1` changes must be additive only.
3. Breaking changes require a new major version directory.
4. UI code, backend bridge code, and platform transport code must conform to this contract.

Contents:

- `protocol.schema.json`: shared envelope schema
- `methods/`: request/response method documentation
- `events/`: async event documentation
- `errors/`: machine-readable error catalog
