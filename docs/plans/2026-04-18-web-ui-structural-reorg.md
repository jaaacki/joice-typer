# Web UI Structural Reorg Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reorganize the repository around responsibilities so a shared web UI layer can be introduced later without another large repo surgery.

**Architecture:** Keep runtime behavior unchanged while moving shared backend packages under `internal/core`, preserving `internal/platform` as OS adapters, and creating first-class `ui/`, `assets/`, and `packaging/` homes. Keep the root `Makefile` as the entrypoint for now and repoint paths instead of replacing the build flow.

**Tech Stack:** Go, build tags, cgo, Make, macOS app packaging, Windows bootstrap build, git file moves.

---

### Task 1: Create the new top-level homes

**Files:**
- Create: `ui/README.md`
- Create: `assets/README.md`
- Create: `assets/icons/.gitkeep`
- Create: `assets/macos/.gitkeep`
- Create: `assets/windows/.gitkeep`
- Create: `packaging/README.md`
- Create: `packaging/macos/.gitkeep`
- Create: `packaging/windows/.gitkeep`
- Test: `README.md`

**Step 1: Write the failing test**

Add a Go-free structure test in:

- Create: `internal/buildinfra/repo_layout_test.go`

Test exact expected paths:

```go
func TestRepoLayout_FutureHomesExist(t *testing.T) {
    for _, path := range []string{
        "ui",
        "assets",
        "assets/icons",
        "assets/macos",
        "assets/windows",
        "packaging",
        "packaging/macos",
        "packaging/windows",
    } {
        if _, err := os.Stat(filepath.Join(repoRoot(t), path)); err != nil {
            t.Fatalf("expected %s to exist: %v", path, err)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestRepoLayout_FutureHomesExist -count=1`

Expected: FAIL because the directories do not exist yet.

**Step 3: Write minimal implementation**

Create the directories and minimal README placeholders:

- `ui/README.md`
- `assets/README.md`
- `packaging/README.md`

Use short text that states each directory’s purpose and explicitly says no runtime migration happens in this phase.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestRepoLayout_FutureHomesExist -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/buildinfra/repo_layout_test.go ui assets packaging
git commit -m "refactor: add frontend and packaging top-level homes"
```

### Task 2: Move shared backend packages under `internal/core`

**Files:**
- Move: `internal/app/` -> `internal/core/runtime/`
- Move: `internal/config/` -> `internal/core/config/`
- Move: `internal/logging/` -> `internal/core/logging/`
- Move: `internal/version/` -> `internal/core/version/`
- Move: `internal/transcription/` -> `internal/core/transcription/`
- Move: `internal/audio/` -> `internal/core/audio/`
- Modify: all import sites under `cmd/` and `internal/`
- Test: moved package tests in new locations

**Step 1: Write the failing test**

Add a source-level architecture test:

- Modify: `internal/launcher/architecture_test.go`

Extend it to assert the old package paths are gone and the new ones are present:

```go
func TestArchitecture_UsesCorePackages(t *testing.T) {
    data, err := os.ReadFile("launcher.go")
    if err != nil {
        t.Fatal(err)
    }
    text := string(data)
    forbidden := []string{
        "voicetype/internal/app",
        "voicetype/internal/config",
        "voicetype/internal/logging",
        "voicetype/internal/version",
        "voicetype/internal/transcription",
        "voicetype/internal/audio",
    }
    required := []string{
        "voicetype/internal/core/runtime",
        "voicetype/internal/core/config",
        "voicetype/internal/core/logging",
        "voicetype/internal/core/version",
        "voicetype/internal/core/transcription",
        "voicetype/internal/core/audio",
    }
    for _, needle := range forbidden {
        if strings.Contains(text, needle) {
            t.Fatalf("found old import %q", needle)
        }
    }
    for _, needle := range required {
        if !strings.Contains(text, needle) {
            t.Fatalf("missing new import %q", needle)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/launcher -run TestArchitecture_UsesCorePackages -count=1`

Expected: FAIL because the launcher still imports the old package paths.

**Step 3: Write minimal implementation**

Move the directories with `git mv` and update imports everywhere:

```bash
git mv internal/app internal/core/runtime
git mv internal/config internal/core/config
git mv internal/logging internal/core/logging
git mv internal/version internal/core/version
git mv internal/transcription internal/core/transcription
git mv internal/audio internal/core/audio
```

Then update imports in:

- `cmd/joicetyper/main.go`
- `internal/launcher/*.go`
- `internal/platform/**/*.go`
- any tests referencing the old paths

Do not change behavior. This is path churn only.

**Step 4: Run tests to verify it passes**

Run:

- `go test ./internal/launcher -run TestArchitecture_UsesCorePackages -count=1`
- `go test ./internal/core/... -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd internal
git commit -m "refactor: move shared packages under internal/core"
```

### Task 3: Repoint platform and launcher imports without changing behavior

**Files:**
- Modify: `internal/launcher/launcher.go`
- Modify: `internal/launcher/reload.go`
- Modify: `internal/platform/platform_darwin.go`
- Modify: Darwin platform package imports as needed
- Test: `internal/launcher/*.go`, `internal/platform/darwin/*`

**Step 1: Write the failing test**

Add one more architecture guard:

- Modify: `internal/launcher/architecture_test.go`

Add:

```go
func TestArchitecture_LauncherDoesNotImportUI(t *testing.T) {
    data, err := os.ReadFile("launcher.go")
    if err != nil {
        t.Fatal(err)
    }
    if strings.Contains(string(data), "voicetype/ui") {
        t.Fatal("launcher must not depend on ui package")
    }
}
```

**Step 2: Run test to verify current behavior**

Run: `go test ./internal/launcher -run 'TestArchitecture_' -count=1`

Expected: PASS or stable pass after Task 2. This step is a guard before edits.

**Step 3: Write minimal implementation**

Update any remaining imports to use the new `internal/core/...` paths. Keep:

- `internal/platform/*` depending on `internal/core/*`
- `internal/core/*` not depending on `internal/platform/*`

Do not introduce wrappers unless needed to satisfy build tags.

**Step 4: Run package tests**

Run:

- `go test ./internal/launcher -count=1`
- `go test ./internal/platform/darwin -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/launcher internal/platform
git commit -m "refactor: repoint launcher and platform imports to core"
```

### Task 4: Move root assets into `assets/`

**Files:**
- Move: `Info.plist.tmpl` -> `assets/macos/Info.plist.tmpl`
- Move: `icon.icns` -> `assets/icons/icon.icns`
- Move: `icon_1024.png` -> `assets/icons/icon_1024.png`
- Modify: `Makefile`
- Test: `internal/buildinfra/makefile_test.go`

**Step 1: Write the failing test**

Extend `internal/buildinfra/makefile_test.go` with exact path expectations:

```go
func TestMakeApp_UsesAssetPaths(t *testing.T) {
    root := repoRoot(t)
    cmd := exec.Command("make", "-n", "app")
    cmd.Dir = root
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("make -n app: %v\n%s", err, out)
    }
    text := string(out)
    if !strings.Contains(text, "assets/macos/Info.plist.tmpl") {
        t.Fatal("expected app build to use assets/macos/Info.plist.tmpl")
    }
    if !strings.Contains(text, "assets/icons/icon.icns") {
        t.Fatal("expected app build to use assets/icons/icon.icns")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMakeApp_UsesAssetPaths -count=1`

Expected: FAIL because `Makefile` still points at the root assets.

**Step 3: Write minimal implementation**

Move the files with `git mv` and update `Makefile` variables:

```bash
mkdir -p assets/macos assets/icons
git mv Info.plist.tmpl assets/macos/Info.plist.tmpl
git mv icon.icns assets/icons/icon.icns
git mv icon_1024.png assets/icons/icon_1024.png
```

Update `Makefile` to use variables such as:

- `PLIST_TEMPLATE := assets/macos/Info.plist.tmpl`
- `APP_ICON := assets/icons/icon.icns`

Keep the build commands the same otherwise.

**Step 4: Run tests**

Run:

- `go test ./internal/buildinfra -run TestMakeApp_UsesAssetPaths -count=1`
- `make app`

Expected: PASS

**Step 5: Commit**

```bash
git add assets Makefile internal/buildinfra/makefile_test.go
git commit -m "refactor: move app resources into assets"
```

### Task 5: Create `packaging/` homes and repoint build scripts conservatively

**Files:**
- Create: `packaging/macos/README.md`
- Create: `packaging/windows/README.md`
- Modify: `Makefile`
- Modify: `README.md`
- Test: `internal/buildinfra/makefile_test.go`

**Step 1: Write the failing test**

Add a documentation/build test:

```go
func TestRepoLayout_PackagingHomesDocumented(t *testing.T) {
    for _, path := range []string{
        "packaging/macos/README.md",
        "packaging/windows/README.md",
    } {
        if _, err := os.Stat(filepath.Join(repoRoot(t), path)); err != nil {
            t.Fatalf("expected %s: %v", path, err)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestRepoLayout_PackagingHomesDocumented -count=1`

Expected: FAIL because those docs do not exist yet.

**Step 3: Write minimal implementation**

Create:

- `packaging/macos/README.md`
- `packaging/windows/README.md`

Each should state what packaging assets/scripts belong there later and explicitly note that the root `Makefile` remains the current entrypoint in this phase.

Update `README.md` to reflect the new asset/package directory intent, but do not rewrite the product docs more than necessary.

**Step 4: Run tests**

Run:

- `go test ./internal/buildinfra -run TestRepoLayout_PackagingHomesDocumented -count=1`
- `go test ./internal/buildinfra -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add packaging README.md internal/buildinfra/makefile_test.go
git commit -m "docs: establish packaging directory homes"
```

### Task 6: Full structural checkpoint verification

**Files:**
- Modify: none unless fixes are required
- Test: whole repository

**Step 1: Run the full Go test suite**

Run: `go test ./... -count=1`

Expected: PASS

**Step 2: Run the binary build**

Run: `go build ./cmd/joicetyper`

Expected: PASS

**Step 3: Run the macOS packaging builds**

Run:

- `make app`
- `make dmg`

Expected: PASS

**Step 4: Run the Windows bootstrap build**

Run: `make build-windows-amd64`

Expected: PASS

**Step 5: Run whitespace and diff checks**

Run:

- `git diff --check`
- `git status --short`

Expected:

- no diff whitespace errors
- only intended tracked changes remain

**Step 6: Commit the checkpoint**

```bash
git add .
git commit -m "refactor: complete web UI structural reorg checkpoint"
```

### Task 7: Final documentation checkpoint

**Files:**
- Modify: `docs/plans/2026-04-18-web-ui-structural-reorg-design.md`
- Modify: `docs/plans/2026-04-18-web-ui-structural-reorg.md`

**Step 1: Update the design doc with any path adjustments discovered during implementation**

Keep changes factual only.

**Step 2: Re-read the plan and mark any implementation-specific deltas**

If a path or task changed, update the plan so it remains a truthful record.

**Step 3: Run docs-only diff review**

Run: `git diff -- docs/plans`

Expected: only intentional documentation changes.

**Step 4: Commit**

```bash
git add docs/plans
git commit -m "docs: finalize structural reorg checkpoint notes"
```
