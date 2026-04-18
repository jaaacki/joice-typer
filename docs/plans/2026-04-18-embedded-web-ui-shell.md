# Embedded Web UI Shell Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an embedded React/TypeScript UI shell to the desktop app, backed by a narrow Go bridge, without moving dictation runtime logic out of Go/native code.

**Architecture:** Introduce a frontend toolchain under `ui/`, build static assets into `ui/dist/`, and embed those assets into the Go binary with `embed`. Add a small shared bridge package under `internal/core/bridge/`, then prove the shell with a minimal embedded page and one real migrated UI surface: the settings window on macOS.

**Tech Stack:** Go `embed`, React, TypeScript, Vite, macOS native shell/webview hosting, existing `internal/core/*` backend, existing `internal/platform/darwin/*` adapters.

---

### Task 1: Add the frontend toolchain skeleton

**Files:**
- Create: `ui/package.json`
- Create: `ui/tsconfig.json`
- Create: `ui/vite.config.ts`
- Create: `ui/index.html`
- Create: `ui/src/main.tsx`
- Create: `ui/src/App.tsx`
- Create: `ui/src/styles.css`
- Create: `ui/.gitignore`
- Modify: `ui/README.md`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Extend `internal/buildinfra/repo_layout_test.go`:

```go
func TestRepoLayout_FrontendToolchainFilesExist(t *testing.T) {
    root := repoRoot(t)
    for _, path := range []string{
        "ui/package.json",
        "ui/tsconfig.json",
        "ui/vite.config.ts",
        "ui/index.html",
        "ui/src/main.tsx",
        "ui/src/App.tsx",
    } {
        if _, err := os.Stat(filepath.Join(root, path)); err != nil {
            t.Fatalf("expected %s: %v", path, err)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestRepoLayout_FrontendToolchainFilesExist -count=1`

Expected: FAIL because the UI toolchain files do not exist yet.

**Step 3: Write minimal implementation**

Create a minimal Vite + React + TypeScript app:

- `ui/package.json`
  - `react`
  - `react-dom`
  - `typescript`
  - `vite`
  - scripts:
    - `dev`
    - `build`
- `ui/index.html`
- `ui/src/main.tsx`
- `ui/src/App.tsx`
- `ui/src/styles.css`
- `ui/tsconfig.json`
- `ui/vite.config.ts`
- `ui/.gitignore`

The app can render a simple shell like:

```tsx
export default function App() {
  return <div>JoiceTyper UI Shell</div>
}
```

Update `ui/README.md` to describe:

- frontend authoring stack
- build output target
- that the UI is still not wired into the desktop runtime yet

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestRepoLayout_FrontendToolchainFilesExist -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add ui internal/buildinfra/repo_layout_test.go
git commit -m "feat: add frontend toolchain skeleton"
```

### Task 2: Make the frontend build reproducibly

**Files:**
- Modify: `ui/package.json`
- Modify: `ui/vite.config.ts`
- Create: `ui/package-lock.json` or `ui/pnpm-lock.yaml` depending on chosen package manager
- Create: `ui/dist/.gitkeep` only if needed for empty-dir expectations
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing test**

Add a build artifact expectation in `internal/buildinfra/repo_layout_test.go`:

```go
func TestFrontendBuild_ProducesDistIndex(t *testing.T) {
    root := repoRoot(t)
    cmd := exec.Command("npm", "run", "build")
    cmd.Dir = filepath.Join(root, "ui")
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("npm run build: %v\n%s", err, out)
    }
    if _, err := os.Stat(filepath.Join(root, "ui", "dist", "index.html")); err != nil {
        t.Fatalf("expected ui/dist/index.html: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestFrontendBuild_ProducesDistIndex -count=1`

Expected: FAIL because dependencies are not installed and no build has been produced.

**Step 3: Write minimal implementation**

Inside `ui/`:

```bash
npm install
npm run build
```

Commit the lockfile, not `node_modules`.

Adjust `vite.config.ts` only if needed so the build output is deterministic and lands in `ui/dist/`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/buildinfra -run TestFrontendBuild_ProducesDistIndex -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add ui/package*.json ui/vite.config.ts internal/buildinfra/repo_layout_test.go
git commit -m "build: make frontend shell build reproducibly"
```

### Task 3: Add embedded frontend assets to the Go binary

**Files:**
- Create: `internal/core/bridge/assets.go`
- Create: `internal/core/bridge/assets_test.go`
- Modify: `cmd/joicetyper/main.go` only if needed for bootstrap wiring later
- Test: `internal/core/bridge/assets_test.go`

**Step 1: Write the failing test**

Create `internal/core/bridge/assets_test.go`:

```go
func TestEmbeddedAssets_ContainsIndexHTML(t *testing.T) {
    data, err := embeddedUI.ReadFile("dist/index.html")
    if err != nil {
        t.Fatalf("expected embedded dist/index.html: %v", err)
    }
    if len(data) == 0 {
        t.Fatal("embedded index.html is empty")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/bridge -run TestEmbeddedAssets_ContainsIndexHTML -count=1`

Expected: FAIL because the bridge package and embedded assets do not exist yet.

**Step 3: Write minimal implementation**

Create `internal/core/bridge/assets.go`:

```go
package bridge

import "embed"

//go:embed ../../../ui/dist/*
var embeddedUI embed.FS
```

Adjust the exact path if needed for `embed` validity from the package location. If direct relative embedding from `internal/core/bridge` is awkward, add a small package under a more suitable location such as `internal/uiembed/`, but keep the responsibility clear.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/core/bridge -run TestEmbeddedAssets_ContainsIndexHTML -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/bridge
git commit -m "feat: embed frontend assets into Go binary"
```

### Task 4: Define the first typed bridge contract

**Files:**
- Create: `internal/core/bridge/api.go`
- Create: `internal/core/bridge/api_test.go`
- Create: `internal/core/bridge/types.go`
- Test: `internal/core/bridge/api_test.go`

**Step 1: Write the failing test**

Create a minimal API contract test:

```go
func TestBridge_NewServiceExposesConfigMethods(t *testing.T) {
    svc := NewService(nil)
    if svc == nil {
        t.Fatal("expected bridge service")
    }
}
```

Add one narrow typed method expectation:

```go
func TestBridge_ConfigSnapshotTypeIsStable(t *testing.T) {
    snapshot := ConfigSnapshot{}
    _ = snapshot.TriggerKey
    _ = snapshot.ModelSize
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/bridge -run TestBridge_ -count=1`

Expected: FAIL because the bridge API does not exist.

**Step 3: Write minimal implementation**

Create:

- `internal/core/bridge/types.go`
  - `ConfigSnapshot`
  - `PermissionsSnapshot`
  - `ModelSnapshot`
  - `AppStateSnapshot`
- `internal/core/bridge/api.go`
  - `Service`
  - `NewService(...)`
  - only placeholders for:
    - config read
    - config write
    - permissions state
    - device list
    - model state
    - app state

Do not overbuild the transport yet. This task is type and boundary definition only.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/core/bridge -run TestBridge_ -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/bridge
git commit -m "feat: define typed backend bridge surface"
```

### Task 5: Add a minimal embedded shell renderer on macOS

**Files:**
- Modify: `internal/platform/darwin/settings.go`
- Modify: `internal/platform/darwin/types.go` if needed
- Create: `internal/platform/darwin/webview.go`
- Create: `internal/platform/darwin/webview_darwin.h`
- Create: `internal/platform/darwin/webview_darwin.m`
- Test: `internal/platform/darwin/settings_test.go`

**Step 1: Write the failing test**

Add a source-level guard in `internal/platform/darwin/settings_test.go`:

```go
func TestSettingsSource_UsesWebViewHostHook(t *testing.T) {
    data, err := os.ReadFile("settings.go")
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(string(data), "ShowWebSettingsWindow") {
        t.Fatal("expected settings flow to reference web settings host")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/darwin -run TestSettingsSource_UsesWebViewHostHook -count=1`

Expected: FAIL because settings are still native-only.

**Step 3: Write minimal implementation**

Add a minimal macOS webview host entrypoint, but keep it non-invasive:

- `ShowWebSettingsWindow(...)`
- it can initially load a static embedded shell page
- do not remove the old native settings implementation yet
- add an opt-in path or temporary dev hook for the embedded shell

This task is shell proof, not full migration.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/darwin -run TestSettingsSource_UsesWebViewHostHook -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/platform/darwin
git commit -m "feat: add macOS embedded webview shell hook"
```

### Task 6: Route one real settings screen through the bridge

**Files:**
- Modify: `internal/core/bridge/api.go`
- Modify: `internal/core/config/config.go`
- Modify: `internal/platform/darwin/settings.go`
- Modify: `ui/src/App.tsx`
- Create: `ui/src/settings/SettingsScreen.tsx`
- Create: `ui/src/settings/SettingsScreen.test.tsx` if frontend test tooling is present
- Test: `internal/platform/darwin/settings_test.go`
- Test: frontend build/test as available

**Step 1: Write the failing test**

Backend side:

```go
func TestBridge_ConfigSnapshotReflectsCurrentConfig(t *testing.T) {
    // use temp config file or injected config dependency
}
```

Frontend side, if test tooling exists:

```tsx
it("renders trigger key and model size fields", () => {
  render(<SettingsScreen ... />)
  expect(screen.getByText(/trigger/i)).toBeInTheDocument()
})
```

**Step 2: Run tests to verify they fail**

Run the targeted backend and frontend tests.

Expected: FAIL because the real settings UI is not yet wired through the bridge.

**Step 3: Write minimal implementation**

Implement one real UI surface:

- config snapshot load
- display trigger keys
- display model size
- display language
- maybe sound feedback toggle

Start with read-only if needed, then add write-back in the same task only if the tests explicitly require it.

Do not move every settings sub-feature yet.

**Step 4: Run tests to verify they pass**

Run the same targeted backend/frontend tests and `npm run build`.

Expected: PASS

**Step 5: Commit**

```bash
git add ui internal/core/bridge internal/platform/darwin
git commit -m "feat: render settings through embedded bridge UI"
```

### Task 7: Add build integration for the frontend shell

**Files:**
- Modify: `Makefile`
- Modify: `internal/buildinfra/makefile_test.go`
- Modify: `README.md`

**Step 1: Write the failing test**

Extend `internal/buildinfra/makefile_test.go`:

```go
func TestMakeApp_BuildsFrontendBeforePackaging(t *testing.T) {
    root := repoRoot(t)
    cmd := exec.Command("make", "-n", "app")
    cmd.Dir = root
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("make -n app: %v\n%s", err, out)
    }
    text := string(out)
    if !strings.Contains(text, "npm run build") {
        t.Fatal("expected app build to include frontend build step")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/buildinfra -run TestMakeApp_BuildsFrontendBeforePackaging -count=1`

Expected: FAIL because the frontend build is not yet part of the packaging flow.

**Step 3: Write minimal implementation**

Update `Makefile`:

- add a frontend build target
- run it before `app`
- keep the existing app/dmg entrypoints

Update `README.md` with concise frontend build notes.

**Step 4: Run test to verify it passes**

Run:

- `go test ./internal/buildinfra -run TestMakeApp_BuildsFrontendBeforePackaging -count=1`
- `make app`

Expected: PASS

**Step 5: Commit**

```bash
git add Makefile README.md internal/buildinfra/makefile_test.go
git commit -m "build: integrate embedded frontend into app packaging"
```

### Task 8: Full phase checkpoint verification

**Files:**
- Modify: none unless fixes are required

**Step 1: Run backend tests**

Run: `go test ./... -count=1`

Expected: PASS

**Step 2: Run frontend build**

Run:

```bash
cd ui
npm run build
```

Expected: PASS

**Step 3: Run app packaging**

Run:

- `make app`
- `make dmg`

Expected: PASS

**Step 4: Run Windows bootstrap build**

Run: `make build-windows-amd64`

Expected: PASS

**Step 5: Run whitespace and diff checks**

Run:

- `git diff --check`
- `git status --short`

Expected:

- no whitespace errors
- only intended tracked changes remain

**Step 6: Commit**

```bash
git add .
git commit -m "feat: add embedded web UI shell foundation"
```
