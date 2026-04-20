# Bridge Errors

All bridge errors must use this shape:

- `code`
- `message`
- `details`
- `retriable`

Examples:

- `bridge.invalid_request`
- `bridge.unsupported_method`
- `bridge.internal_error`
- `config.invalid`
- `config.load_failed`
- `config.save_failed`
- `permissions.unavailable`
- `permissions.invalid_target`
- `permissions.open_settings_failed`
- `devices.enumeration_failed`
- `devices.refresh_failed`
- `model.unavailable`
- `model.download_failed`
- `model.delete_failed`
- `model.use_failed`
- `logs.unavailable`
- `runtime.unavailable`
- `bridge.unsupported_method`

Rules:

1. String-only errors are invalid bridge output.
2. Error codes are stable identifiers.
3. `message` is user-readable.
4. `details` is for structured context.
5. `retriable` tells the UI whether retry affordances should be offered.

Logs errors:

- `logs.unavailable` reports that the shared log source could not be read or is not available.
