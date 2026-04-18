# Versioning Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a single-source-of-truth versioning system rooted in a checked-in `VERSION` file and wire it through Go builds, app packaging, and release validation for `1.0.0`.

**Architecture:** Store the canonical semantic version in a root `VERSION` file, inject it into the Go binary with linker flags, and generate `Info.plist` from a template so the app bundle uses the same value. Add a small validation path that checks `VERSION` against an optional `v<version>` Git tag for release use.

**Tech Stack:** Go, Make, macOS app bundle packaging, plist template substitution, Git.

---

### Task 1: Add Version Source And Failing Tests

**Files:**
- Create: `VERSION`
- Create: `version.go`
- Create: `version_test.go`

**Step 1: Write the failing tests**

Add tests for:

- reading and trimming `VERSION`
- rejecting empty or malformed versions
- validating a release tag like `v1.0.0` against `1.0.0`

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestLoadVersion|TestValidateReleaseTag' -count=1`
Expected: FAIL because version helpers do not exist yet.

**Step 3: Write minimal implementation**

Add:

- `var Version = "dev"` for runtime/build injection
- helper(s) to read `VERSION`
- helper to validate `v<version>` tag format against the loaded version

Set `VERSION` to:

```text
1.0.0
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestLoadVersion|TestValidateReleaseTag' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add VERSION version.go version_test.go
git commit -m "feat: add single-source version helpers"
```

### Task 2: Wire Version Into Go Build And CLI Surface

**Files:**
- Modify: `Makefile`
- Modify: `main.go`
- Test: `version_test.go`

**Step 1: Write the failing test**

Add a focused test for any formatting/helper used to expose version text from Go, keeping logic testable without shelling out.

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestFormatVersion' -count=1`
Expected: FAIL because the helper/output path does not exist yet.

**Step 3: Write minimal implementation**

- update `go build` in `Makefile` to inject the loaded version with `-ldflags`
- add a small visible version surface in Go, such as `--version` handling or a logged startup version value

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestFormatVersion' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add Makefile main.go version_test.go
git commit -m "feat: inject app version into Go builds"
```

### Task 3: Replace Hardcoded Plist Versions With A Template

**Files:**
- Create: `Info.plist.tmpl`
- Modify: `Info.plist`
- Modify: `Makefile`
- Create or Modify: `version_test.go`

**Step 1: Write the failing test**

Add a test for plist rendering/substitution so:

- `CFBundleVersion` equals `1.0.0`
- `CFBundleShortVersionString` equals `1.0.0`

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestRenderInfoPlist' -count=1`
Expected: FAIL because plist rendering is not implemented yet.

**Step 3: Write minimal implementation**

- template the plist version fields
- make the packaging step render `Info.plist` from the template and current `VERSION`
- avoid introducing a second checked-in version constant

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestRenderInfoPlist' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add Info.plist Info.plist.tmpl Makefile version_test.go
git commit -m "feat: generate plist version from VERSION"
```

### Task 4: Add Release Validation And Packaging Verification

**Files:**
- Modify: `Makefile`
- Modify: `README.md`
- Create or Modify: `version_test.go`

**Step 1: Write the failing test**

Add test coverage for:

- matching `v1.0.0` tag succeeds
- mismatched tag fails
- malformed tag fails

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestValidateReleaseTag' -count=1`
Expected: FAIL until validation is complete.

**Step 3: Write minimal implementation**

- add a release-oriented Make target or script that:
  - reads `VERSION`
  - reads a provided tag or current tag
  - fails on mismatch
- document the version bump and release sequence in `README.md`

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestValidateReleaseTag' -count=1`
Expected: PASS

**Step 5: Verify packaging**

Run:

```bash
go test ./... -count=1
make app
make dmg
```

Expected:

- tests pass
- `.app` builds
- `.dmg` builds
- generated app metadata reflects `1.0.0`

**Step 6: Commit**

```bash
git add Makefile README.md version_test.go
git commit -m "feat: add release version validation"
```
