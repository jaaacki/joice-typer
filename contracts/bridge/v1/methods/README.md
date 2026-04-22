# Bridge Methods

Initial `v1` method surface:

- `bootstrap.get`
- `config.get`
- `config.save`
- `permissions.get`
- `permissions.open_settings`
- `devices.list`
- `devices.refresh`
- `model.get`
- `model.download`
- `model.delete`
- `model.use`
- `audio_input.monitor_set`
- `runtime.get`
- `options.get`
- `logs.get`
- `logs.copy_all`

Method rules:

1. Methods are namespaced strings.
2. Queries return current state only.
3. Commands may mutate state and may emit follow-up events.
4. Every request must produce exactly one response.

Logs methods:

- `logs.get` returns the current tail view for the shared logs page.
- `logs.copy_all` returns the full log text for copy/export flows.
