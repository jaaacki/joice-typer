# Releases

The intended release flow is git-driven:

```text
VERSION file -> git tag -> platform builds -> signed artifacts -> GitHub Release
```

## Manual outline

```bash
# after deciding the next release version
git checkout main
# update VERSION intentionally
git commit -am "chore(release): v1.1.113"
git tag v1.1.113
git push origin main --tags
```

Release Make targets require the matching tag explicitly unless the checkout is already on that exact tag:

```bash
make mac-release-artifacts RELEASE_TAG=v$(cat VERSION)
make package-windows-release RELEASE_TAG=v$(cat VERSION)
```

A future GitHub Actions workflow can build and upload artifacts from the tag.

## macOS release requirements

A proper macOS release needs:

- stable bundle ID: `com.joicetyper.app`
- Developer ID Application signing certificate
- notarization credentials/profile
- Sparkle signing keys if auto-update is enabled
- clean release version matching the git tag

## Windows release requirements

A proper Windows release needs:

- stable Inno Setup `AppId`
- clean `AppVersion` from `VERSION`
- staged runtime DLLs and executable
- Inno Setup compiler
- Authenticode certificate if code signing is enabled

Unsigned Windows builds may trigger SmartScreen warnings.

## Automation direction

The desired endpoint is:

```text
push tag vX.Y.Z -> CI builds macOS + Windows -> GitHub Release artifacts
```

Keep local Make targets aligned with that future CI flow so the same commands can be reused by automation.
