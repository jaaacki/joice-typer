# Versioning

`VERSION` is the committed base release version. Normal development builds must not mutate it.

## Display versions

Release builds display the clean version:

```text
1.1.112
```

Development builds derive a display version from `VERSION` and git state:

```text
1.1.112-dev+b18d9d7
1.1.112-dev+b18d9d7.dirty
```

`dirty` means the build was produced with uncommitted changes in the working tree.

## Build-time injection

Make computes the display version in `scripts/make/version.mk` and passes it into Go with ldflags:

```make
-X 'voicetype/internal/core/version.Version=$(DISPLAY_VERSION)'
```

The app exposes that value through the bridge, About pane, CLI version output, and startup logs.

## macOS bundle metadata

The macOS app bundle `Info.plist` version fields stay release-safe and SemVer-like for normal app registration. Dev commit metadata is shown inside the app, logs, and dev update artifacts rather than used as the app bundle identity.

## Release rule

A production release should satisfy:

```text
git tag == v$(VERSION)
app display version == $(VERSION)
installer/package version == $(VERSION)
```
