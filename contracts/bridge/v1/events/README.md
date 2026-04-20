# Bridge Events

Initial `v1` event surface:

- `runtime.state_changed`
- `permissions.changed`
- `devices.changed`
- `model.download_progress`
- `config.saved`
- `logs.updated`

Event rules:

1. Events are async notifications, never replacements for request responses.
2. Events must use the shared event envelope.
3. Event payloads must remain backward-compatible within `v1`.

Logs event:

- `logs.updated` notifies the UI that the shared log view should refresh.
