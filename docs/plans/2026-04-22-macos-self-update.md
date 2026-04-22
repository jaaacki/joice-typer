# macOS Self-Update Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a real Sparkle-ready macOS self-update backend and release pipeline for JoiceTyper without blocking normal local development when Apple signing and notarization credentials are absent.

**Architecture:** Keep normal dev packaging (`make app`, `make dmg`) working as they do today, but add separate mac release/update targets for signed archives and appcast generation. Wire Sparkle into the mac app/backend in a release-gated way, with release credentials provided externally and fail-closed checks on release-only targets.

**Tech Stack:** Go, Make, macOS app packaging, Sparkle, XML appcast generation, codesign, notarization tooling, GitHub Releases-oriented artifact layout

---

### Task 1: Inspect current mac packaging and define release target boundaries

**Files:**
- Modify: `scripts/make/macos.mk`
- Test: `internal/buildinfra/makefile_test.go`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Add buildinfra assertions that mac packaging now exposes explicit release/update targets distinct from `app` and `dmg`.

Examples to assert:
- `mac-release-archive`
- `mac-appcast`
- release targets do not replace dev targets

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMakefileHasMacReleaseTargets -count=1`

Expected: FAIL because the new targets do not exist yet.

**Step 3: Write minimal implementation**

Add placeholder mac release target names in `scripts/make/macos.mk` without changing current dev target behavior.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestMakefileHasMacReleaseTargets -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/make/macos.mk internal/buildinfra/makefile_test.go internal/buildinfra/repo_layout_test.go
git commit -m "build: define mac release target boundaries"
```

### Task 2: Add mac release config templates and ignored secret/config inputs

**Files:**
- Create: `packaging/macos/README.md`
- Create: `packaging/macos/sparkle-appcast.xml.tmpl`
- Create: `packaging/macos/release.env.example`
- Modify: `.gitignore`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Add a repo-layout test requiring:
- `packaging/macos/sparkle-appcast.xml.tmpl`
- `packaging/macos/release.env.example`
- ignored local mac release secret/config file path

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMacReleasePackagingHomeExists -count=1`

Expected: FAIL because files do not exist yet.

**Step 3: Write minimal implementation**

Create:
- a mac packaging README explaining dev vs release targets
- a release env example with placeholders for signing/notarization/feed values
- an appcast template
- ignored local release config/secret paths in `.gitignore`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestMacReleasePackagingHomeExists -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add packaging/macos .gitignore internal/buildinfra/repo_layout_test.go
git commit -m "build: add mac release config templates"
```

### Task 3: Add Sparkle integration placeholders to the mac app bundle path

**Files:**
- Modify: `assets/macos/Info.plist.tmpl`
- Modify: `scripts/make/macos.mk`
- Test: `internal/core/version/version_test.go`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Add tests/assertions that the mac Info.plist template includes release-updater metadata placeholders needed for Sparkle-backed builds, while still remaining compatible with dev builds.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/version -run TestResolveVersionTemplateSupportsUpdaterPlaceholders -count=1`

Expected: FAIL because updater placeholders are absent.

**Step 3: Write minimal implementation**

Update `assets/macos/Info.plist.tmpl` with Sparkle-related placeholders and adjust packaging/template substitution so unset release values do not break dev builds.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/core/version -run TestResolveVersionTemplateSupportsUpdaterPlaceholders -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add assets/macos/Info.plist.tmpl scripts/make/macos.mk internal/core/version/version_test.go internal/buildinfra/repo_layout_test.go
git commit -m "build: prepare mac app metadata for updater"
```

### Task 4: Add fail-closed mac release credential validation

**Files:**
- Create: `scripts/release/macos_release_env.sh`
- Modify: `scripts/make/macos.mk`
- Test: `internal/buildinfra/makefile_test.go`

**Step 1: Write the failing test**

Add buildinfra tests asserting:
- release targets require explicit signing/notarization/feed inputs
- `make app` does not require them
- release targets fail closed with clear fatal messages

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMacReleaseTargetsFailClosedWithoutCredentials -count=1`

Expected: FAIL because validation logic does not exist yet.

**Step 3: Write minimal implementation**

Create a small shell helper that validates required env/config for release targets and wire it into release-only make targets.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestMacReleaseTargetsFailClosedWithoutCredentials -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/release/macos_release_env.sh scripts/make/macos.mk internal/buildinfra/makefile_test.go
git commit -m "build: validate mac release credentials explicitly"
```

### Task 5: Add signed archive generation target for Sparkle updates

**Files:**
- Create: `scripts/release/macos_archive.sh`
- Modify: `scripts/make/macos.mk`
- Test: `internal/buildinfra/makefile_test.go`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Add tests that assert:
- a mac release archive target exists
- it consumes the built `.app`
- it produces a deterministic archive path tied to `VERSION`

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMacReleaseArchiveTargetUsesVersionedArtifactNames -count=1`

Expected: FAIL because no archive target exists yet.

**Step 3: Write minimal implementation**

Create a helper script and make target to generate the Sparkle-ready update archive from the built/signed app bundle.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestMacReleaseArchiveTargetUsesVersionedArtifactNames -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/release/macos_archive.sh scripts/make/macos.mk internal/buildinfra/makefile_test.go internal/buildinfra/repo_layout_test.go
git commit -m "build: add mac release archive target"
```

### Task 6: Add appcast generation target and template rendering

**Files:**
- Create: `scripts/release/macos_appcast.py`
- Modify: `scripts/make/macos.mk`
- Modify: `packaging/macos/sparkle-appcast.xml.tmpl`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Add source/layout tests asserting:
- there is an appcast generation script
- the appcast template includes release version, enclosure URL, length, and publication date placeholders

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMacAppcastGenerationFilesExist -count=1`

Expected: FAIL because the script and concrete template contents do not exist yet.

**Step 3: Write minimal implementation**

Add a small appcast renderer that takes release metadata inputs and writes appcast XML from the template.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestMacAppcastGenerationFilesExist -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/release/macos_appcast.py scripts/make/macos.mk packaging/macos/sparkle-appcast.xml.tmpl internal/buildinfra/repo_layout_test.go
git commit -m "build: add mac appcast generation"
```

### Task 7: Add Sparkle framework acquisition/staging path

**Files:**
- Create: `scripts/release/macos_stage_sparkle.sh`
- Modify: `scripts/make/macos.mk`
- Modify: `packaging/macos/README.md`
- Test: `internal/buildinfra/makefile_test.go`

**Step 1: Write the failing test**

Add tests that assert:
- the mac release path includes explicit Sparkle staging
- dev `make app` does not fail just because Sparkle release staging inputs are absent

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMacReleasePathStagesSparkleSeparatelyFromDevBuilds -count=1`

Expected: FAIL because no Sparkle staging split exists yet.

**Step 3: Write minimal implementation**

Add a release-only Sparkle staging helper and keep it out of normal dev packaging.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestMacReleasePathStagesSparkleSeparatelyFromDevBuilds -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/release/macos_stage_sparkle.sh scripts/make/macos.mk packaging/macos/README.md internal/buildinfra/makefile_test.go
git commit -m "build: stage sparkle only for mac release path"
```

### Task 8: Add GitHub Releases-oriented output layout

**Files:**
- Modify: `scripts/make/macos.mk`
- Modify: `README.md`
- Test: `internal/buildinfra/makefile_test.go`

**Step 1: Write the failing test**

Add tests asserting mac release targets generate a stable output layout suitable for GitHub Releases, including:
- archive artifact
- optional dmg artifact
- appcast output

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMacReleaseTargetsProduceGitHubReleaseFriendlyOutputs -count=1`

Expected: FAIL because the layout is not fully defined yet.

**Step 3: Write minimal implementation**

Define the output paths and update `README.md` with the mac release/update flow.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestMacReleaseTargetsProduceGitHubReleaseFriendlyOutputs -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/make/macos.mk README.md internal/buildinfra/makefile_test.go
git commit -m "docs: define mac release artifact layout"
```

### Task 9: Add updater-disabled-by-default runtime behavior for dev builds

**Files:**
- Create: `internal/platform/darwin/updater.go`
- Modify: `internal/platform/darwin/webview.go`
- Modify: `internal/platform/darwin/settings.go`
- Test: `internal/platform/darwin/settings_test.go`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Add tests asserting:
- updater support is compiled/wired as a backend integration point
- unsigned/dev builds do not expose or invoke live update behavior
- no visible updater UI is required in this phase

**Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/darwin -run TestUpdaterIsDisabledForDevBuilds -count=1`

Expected: FAIL because the updater backend boundary does not exist yet.

**Step 3: Write minimal implementation**

Add a Darwin updater boundary that is disabled by default until release/update configuration is available.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/darwin -run TestUpdaterIsDisabledForDevBuilds -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/platform/darwin/updater.go internal/platform/darwin/webview.go internal/platform/darwin/settings.go internal/platform/darwin/settings_test.go internal/buildinfra/repo_layout_test.go
git commit -m "feat: add mac updater backend boundary"
```

### Task 10: Verify dev and release paths explicitly

**Files:**
- Modify: `README.md`

**Step 1: Run targeted verification**

Run:

```bash
go test ./internal/buildinfra -count=1
go test ./internal/platform/darwin -count=1
git diff --check
```

Expected:
- PASS
- PASS
- no diff check issues

**Step 2: Verify dev path remains unblocked**

Run:

```bash
make -n app
```

Expected:
- no mandatory Sparkle/signing/notarization credential requirement in the normal dev app target

**Step 3: Verify release path fails closed without credentials**

Run:

```bash
make mac-release-archive
```

Expected:
- explicit failure describing the missing release credentials/config

**Step 4: Commit final docs cleanup**

```bash
git add README.md
git commit -m "docs: verify mac self-update release flow"
```
