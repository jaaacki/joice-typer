# macOS Web Preferences Cutover Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the embedded web UI the authoritative macOS Preferences surface while keeping the old native Preferences path available only as a hidden fallback.

**Architecture:** Extend the frozen bridge contract with the remaining device/model command surface, complete the web settings screen so it can perform every normal Preferences action, and reroute the macOS Preferences entrypoint to the web window by default. Native code remains the host and capability layer, but stops owning the normal settings UX.

**Tech Stack:** Go, AppKit/WebKit bridge, React, TypeScript, Vite, generated contract outputs from `contracts/bridge/v1/`

---

### Task 1: Freeze the contract catalog for the full Preferences surface

**Files:**
- Modify: `contracts/bridge/v1/catalog.json`
- Modify: `contracts/bridge/v1/methods/README.md`
- Modify: `contracts/bridge/v1/errors/README.md`
- Modify: `scripts/generate_bridge_contract/main.go`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Add the missing command names and error codes**

Add command entries for:

- `devices.refresh`
- `model.download`
- `model.delete`
- `model.use`

Add explicit error codes for:

- `devices.refresh_failed`
- `model.download_failed`
- `model.delete_failed`
- `model.use_failed`

**Step 2: Regenerate the shared outputs**

Run:

```bash
go run ./scripts/generate_bridge_contract
```

Expected: generated Go and TS protocol outputs update with the new constants.

**Step 3: Lock the generator with tests**

Ensure `internal/buildinfra/repo_layout_test.go` still checks:

- generator outputs exist
- `go run ./scripts/generate_bridge_contract -check` passes

**Step 4: Verify**

Run:

```bash
go test ./internal/buildinfra -count=1
```

Expected: PASS

**Step 5: Commit**

```bash
git add contracts/bridge/v1 scripts/generate_bridge_contract internal/buildinfra/repo_layout_test.go internal/core/bridge/generated ui/src/bridge/generated
git commit -m "build: extend bridge contract catalog for web preferences cutover"
```

### Task 2: Add router and service support for device/model commands

**Files:**
- Modify: `internal/core/bridge/api.go`
- Modify: `internal/core/bridge/protocol.go`
- Modify: `internal/core/bridge/router.go`
- Modify: `internal/core/bridge/types.go`
- Test: `internal/core/bridge/router_test.go`
- Test: `internal/core/bridge/contract_error_test.go`

**Step 1: Write failing router tests**

Add tests for:

- `devices.refresh`
- `model.download`
- `model.delete`
- `model.use`

Each test should assert:

- success response shape on success
- explicit bridge error code on failure

**Step 2: Run the new router tests to confirm failure**

Run:

```bash
go test ./internal/core/bridge -run 'TestRouterHandleRequest_(DevicesRefresh|ModelDownload|ModelDelete|ModelUse)' -count=1
```

Expected: FAIL because the methods are not routed yet.

**Step 3: Add minimal dependency hooks and router cases**

Extend `Dependencies` and `Service` with:

- `RefreshDevices`
- `DownloadModel`
- `DeleteModel`
- `UseModel`

Add request param structs where needed and route the new methods with explicit contract errors.

**Step 4: Re-run router tests**

Run:

```bash
go test ./internal/core/bridge -count=1
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/bridge/api.go internal/core/bridge/protocol.go internal/core/bridge/router.go internal/core/bridge/types.go internal/core/bridge/router_test.go internal/core/bridge/contract_error_test.go
git commit -m "feat: add bridge router support for device and model commands"
```

### Task 3: Implement the Darwin bridge service for device/model commands

**Files:**
- Modify: `internal/platform/darwin/webview.go`
- Modify: `internal/platform/darwin/settings.go`
- Modify: `internal/platform/darwin/power.go`
- Test: `internal/platform/darwin/webview_test.go`

**Step 1: Write failing Darwin bridge tests**

Add tests covering:

- `devices.refresh` dispatch path
- `model.download`
- `model.delete`
- `model.use`

The tests should use injected function variables and assert:

- correct native/backend function is called
- explicit contract error code is preserved on failure

**Step 2: Run the focused Darwin tests**

Run:

```bash
go test ./internal/platform/darwin -run 'TestBuildSettingsBridgeService_(RefreshDevices|ModelCommands)' -count=1
```

Expected: FAIL before implementation.

**Step 3: Implement the command hooks**

In `buildSettingsBridgeService(...)`, wire the new dependency callbacks to Darwin-side helpers that:

- refresh recorder devices
- start model download
- delete cached model
- switch active model

Use contract errors instead of plain `fmt.Errorf(...)` for the command boundary.

**Step 4: Ensure events remain consistent**

After successful command execution, publish the right event:

- `devices.changed`
- `model.changed`
- `model.download_progress` as applicable

**Step 5: Re-run the focused Darwin tests**

Run:

```bash
go test ./internal/platform/darwin -count=1
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/platform/darwin/webview.go internal/platform/darwin/settings.go internal/platform/darwin/power.go internal/platform/darwin/webview_test.go
git commit -m "feat: wire darwin bridge service for web preferences commands"
```

### Task 4: Complete the React settings UI for authoritative model/device actions

**Files:**
- Modify: `ui/src/bridge/client.ts`
- Modify: `ui/src/bridge/index.ts`
- Modify: `ui/src/settings/SettingsScreen.tsx`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Add the failing source-guard expectations**

Extend `internal/buildinfra/repo_layout_test.go` to assert the settings screen contains bridge-backed handlers for:

- device refresh
- model download
- model delete
- model use

and does not rely on hidden native-only UI behavior for those actions.

**Step 2: Run the source-guard test**

Run:

```bash
go test ./internal/buildinfra -run 'Test(SettingsScreenSource_UsesRuntimeStateSubscription|BridgeSource_ExposesRuntimeStateEventSubscription)' -count=1
```

Expected: FAIL until the new handlers exist.

**Step 3: Add bridge client methods**

In `ui/src/bridge/client.ts`, add:

- `refreshDevices()`
- `downloadModel(size)`
- `deleteModel(size)`
- `useModel(size)`

Use generated contract method names and keep all failures as `BridgeRequestError`.

**Step 4: Make the settings screen own the actions**

In `SettingsScreen.tsx`:

- add buttons for refresh/download/delete/use
- connect them to the bridge methods
- update status text truthfully from command outcomes and event flow
- keep the current runtime/model/device state synchronized through queries and events

**Step 5: Verify the frontend build**

Run:

```bash
cd ui && npm run build
```

Expected: PASS

**Step 6: Re-run the source-guard tests**

Run:

```bash
go test ./internal/buildinfra -count=1
```

Expected: PASS

**Step 7: Commit**

```bash
git add ui/src/bridge ui/src/settings/SettingsScreen.tsx internal/buildinfra/repo_layout_test.go
git commit -m "feat: complete web settings actions for macos preferences"
```

### Task 5: Cut the default macOS Preferences entrypoint over to the web UI

**Files:**
- Modify: `internal/platform/darwin/settings.go`
- Modify: `internal/platform/darwin/settings_darwin.h`
- Modify: `internal/platform/darwin/settings_darwin.m`
- Test: `internal/platform/darwin/webview_test.go`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Add a failing guard for the default path**

Add tests or source guards asserting:

- normal Preferences open path goes to the web settings window
- native fallback is no longer the normal path

**Step 2: Run the focused tests**

Run:

```bash
go test ./internal/platform/darwin ./internal/buildinfra -count=1
```

Expected: FAIL until the entrypoint is updated.

**Step 3: Change the default path**

Update the Preferences entrypoint so:

- the normal user path always opens the web settings window
- the old native Preferences flow remains available only through a debug/env-controlled fallback

Do not delete the native code.

**Step 4: Keep the fallback hidden**

Ensure the fallback:

- is not visible in normal menus or flows
- is only reachable through a deliberate debug escape hatch

**Step 5: Re-run focused tests**

Run:

```bash
go test ./internal/platform/darwin ./internal/buildinfra -count=1
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/platform/darwin/settings.go internal/platform/darwin/settings_darwin.h internal/platform/darwin/settings_darwin.m internal/platform/darwin/webview_test.go internal/buildinfra/repo_layout_test.go
git commit -m "refactor: make web ui the default macos preferences path"
```

### Task 6: Verify the full cutover checkpoint

**Files:**
- Verify only

**Step 1: Run the full Go test matrix**

Run:

```bash
go test ./... -count=1
```

Expected: PASS

**Step 2: Verify generated contract outputs**

Run:

```bash
go run ./scripts/generate_bridge_contract -check
```

Expected: PASS

**Step 3: Verify the frontend build**

Run:

```bash
cd ui && npm run build
```

Expected: PASS

**Step 4: Verify Windows bootstrap build**

Run:

```bash
make build-windows-amd64
```

Expected: PASS

**Step 5: Verify macOS app packaging**

Run:

```bash
make app
```

Expected: PASS and produce `JoiceTyper.app` in the worktree.

**Step 6: Verify diff hygiene**

Run:

```bash
git diff --check
```

Expected: PASS

**Step 7: Commit the final checkpoint**

```bash
git add .
git commit -m "feat: cut macos preferences over to the embedded web ui"
```
