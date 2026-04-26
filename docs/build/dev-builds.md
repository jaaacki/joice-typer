# Development builds

Use dev builds when testing local changes. Dev builds show commit metadata in the app so you can tell exactly what is running.

## macOS

Build a local app bundle:

```bash
make app-no-version-bump
```

Install it over the current local app only when you intentionally want to test that build:

```bash
backup="/Applications/JoiceTyper.app.backup.$(date +%Y%m%d%H%M%S)"
[ ! -d /Applications/JoiceTyper.app ] || mv /Applications/JoiceTyper.app "$backup"
cp -R JoiceTyper.app /Applications/JoiceTyper.app
```

For safer replacement, keep the timestamped backup until the new app is verified.

## Verification

Dev packages and dry-run update artifacts use a package-safe dev version such as `1.1.112-dev-b18d9d7` in their package/update metadata where possible, while the macOS app bundle keeps stable registration fields.

Before pushing build-related changes:

```bash
make verify
make verify-mac
```

`make verify-windows` validates Windows-facing policy from the current host, but full Windows runtime and installer packaging still require a Windows-capable builder with the required toolchain/artifacts.

## Changing build behavior

When changing Make targets or release scripts:

1. Decide whether the target is dev, verify, package, or release.
2. Dev and verify targets must not mutate `VERSION`.
3. Release targets must use clean release version metadata.
4. Update `internal/buildinfra/` tests for the new policy.
5. Update these docs if developer commands change.
