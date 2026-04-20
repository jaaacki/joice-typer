# Windows Multi-Arch Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a real Windows adapter and installer under the existing shared web UI and bridge contract, while also introducing a shared Logs page for both macOS and Windows.

**Architecture:** Keep `contracts/bridge/v1`, `ui/`, and `internal/core/...` as the authority. Add Windows only as a sibling adapter under `internal/platform/windows` plus Windows packaging under `packaging/windows`. Any capability required for Windows that is missing from the current architecture must be added to the shared contract/core first, then implemented by both Darwin and Windows as needed.

**Tech Stack:** Go, CGO/platform shims, React/TypeScript/Vite, shared generated bridge contract, `WKWebView` on macOS, `WebView2` on Windows, Windows installer tooling under `packaging/windows`.

---

### Task 1: Freeze the shared contract additions for logs and Windows parity

**Files:**
- Modify: `contracts/bridge/v1/catalog.json`
- Modify: `contracts/bridge/v1/README.md`
- Modify: `contracts/bridge/v1/methods/README.md`
- Modify: `contracts/bridge/v1/events/README.md`
- Modify: `contracts/bridge/v1/errors/README.md`
- Modify: `internal/core/bridge/generated/protocol_gen.go`
- Modify: `ui/src/bridge/generated/protocol.ts`
- Modify: `scripts/generate_bridge_contract/main.go` or `scripts/generate_bridge_contract/*.go`
- Test: `internal/core/bridge/router_test.go`
- Test: `internal/core/bridge/contract_error_test.go`

**Step 1: Write the failing tests**

- Add failing assertions in `internal/core/bridge/router_test.go` for:
  - `logs.get`
  - `logs.copy_all`
- Add failing assertions in `internal/core/bridge/contract_error_test.go` for any new shared error codes needed by logs or Windows adapter parity.

**Step 2: Run the focused tests to verify failure**

Run:

```bash
go test ./internal/core/bridge -count=1
```

Expected: FAIL because the methods/error codes are not yet defined.

**Step 3: Update the catalog**

- Add additive `v1` contract entries for:
  - `logs.get`
  - `logs.copy_all`
  - `logs.updated`
- Add any missing shared error codes only if truly needed for shared semantics.

**Step 4: Regenerate protocol outputs**

Run:

```bash
go run ./scripts/generate_bridge_contract
```

**Step 5: Update human-readable contract docs**

- Document the new logs methods/events and any new shared errors.

**Step 6: Re-run focused tests**

Run:

```bash
go test ./internal/core/bridge -count=1
go run ./scripts/generate_bridge_contract -check
```

Expected: PASS.

**Step 7: Commit**

```bash
git add contracts/bridge/v1 scripts/generate_bridge_contract internal/core/bridge ui/src/bridge/generated
git commit -m "feat: extend bridge contract for logs and windows parity"
```

### Task 2: Add shared core log-reading helpers

**Files:**
- Modify: `internal/core/logging/logger.go`
- Create: `internal/core/logging/log_reader.go`
- Create: `internal/core/logging/log_reader_test.go`

**Step 1: Write the failing tests**

Add tests for:
- reading the last `500` lines from a log file
- returning `truncated=true` when lines are omitted
- returning full content for copy-all
- handling missing log file cleanly

**Step 2: Run the tests to verify failure**

Run:

```bash
go test ./internal/core/logging -count=1
```

Expected: FAIL because the helper does not exist yet.

**Step 3: Implement the minimal shared helpers**

Add a shared helper shape such as:
- `ReadLogTail(path string, maxLines int) (...)`
- `ReadFullLog(path string) (...)`

Keep all semantics in core, not in platform adapters.

**Step 4: Re-run logging tests**

Run:

```bash
go test ./internal/core/logging -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/core/logging
git commit -m "feat: add shared log readers"
```

### Task 3: Wire logs into the shared bridge service and router

**Files:**
- Modify: `internal/core/bridge/api.go`
- Modify: `internal/core/bridge/protocol.go`
- Modify: `internal/core/bridge/types.go`
- Modify: `internal/core/bridge/router.go`
- Modify: `internal/core/bridge/api_test.go`
- Modify: `internal/core/bridge/router_test.go`

**Step 1: Write the failing tests**

Add tests covering:
- `logs.get` returns tail payload with `truncated`, `byteSize`, `updatedAt`, and text
- `logs.copy_all` returns the full file text
- missing log dependencies return typed contract errors rather than silent success

**Step 2: Run the focused tests**

Run:

```bash
go test ./internal/core/bridge -count=1
```

Expected: FAIL.

**Step 3: Extend bridge dependency wiring**

Add shared dependency hooks for:
- reading the tail log payload
- reading the full log payload

Do not add platform-specific payload shapes.

**Step 4: Extend router handling**

- Route `logs.get`
- Route `logs.copy_all`
- keep error handling contract-shaped

**Step 5: Re-run focused tests**

Run:

```bash
go test ./internal/core/bridge -count=1
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/core/bridge
git commit -m "feat: bridge logs through shared service"
```

### Task 4: Add the shared Logs pane to the web Preferences UI

**Files:**
- Modify: `ui/src/settings/SettingsScreen.tsx`
- Create: `ui/src/settings/panes/LogsPane.tsx`
- Modify: `ui/src/settings/shared.tsx`
- Modify: `ui/src/bridge/client.ts`
- Modify: `ui/src/bridge/index.ts`
- Modify: `ui/src/styles.css`
- Test: `internal/buildinfra/repo_layout_test.go`

**Step 1: Write the failing guard/test**

- Extend `internal/buildinfra/repo_layout_test.go` to assert the new Logs pane still goes through `ui/src/bridge/` rather than raw native calls.

**Step 2: Run the focused test**

Run:

```bash
go test ./internal/buildinfra -count=1
```

Expected: FAIL after adding the new guard.

**Step 3: Implement the UI pane**

Add:
- `Logs` nav item
- read-only multiline text surface showing the tail
- metadata row
- `Copy Full Log` button
- live refresh through `logs.updated`

**Step 4: Add bridge client methods**

Add:
- `fetchLogs()`
- `copyFullLog()`
- `subscribeLogsUpdated(...)`

Use generated/shared protocol names only.

**Step 5: Build the frontend**

Run:

```bash
npm run build
```

Expected: PASS.

**Step 6: Re-run the build-infra test**

Run:

```bash
go test ./internal/buildinfra -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
git add ui internal/buildinfra
git commit -m "feat: add shared logs preferences pane"
```

### Task 5: Add Darwin log support through the shared contract

**Files:**
- Modify: `internal/platform/darwin/settings.go`
- Modify: `internal/platform/darwin/webview.go`
- Modify: `internal/platform/darwin/settings_test.go`
- Modify: `internal/platform/darwin/webview_test.go`

**Step 1: Write the failing tests**

Add tests covering:
- Darwin bridge service provides log tail/full handlers
- Darwin publishes `logs.updated`
- missing or unreadable log file surfaces a contract error or empty-safe result according to the chosen contract rules

**Step 2: Run the focused tests**

Run:

```bash
go test ./internal/platform/darwin -count=1
```

Expected: FAIL.

**Step 3: Implement Darwin adapter hooks**

- map current log file path into shared bridge dependencies
- emit `logs.updated` from the existing Preferences runtime plumbing
- keep event/log transport explicit, not silent

**Step 4: Re-run the Darwin tests**

Run:

```bash
go test ./internal/platform/darwin -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/platform/darwin
git commit -m "feat: bridge logs through darwin preferences"
```

### Task 6: Create the Windows platform package skeleton under the shared contracts

**Files:**
- Create: `internal/platform/windows/doc.go`
- Create: `internal/platform/windows/types.go`
- Create: `internal/platform/windows/runtime_state.go`
- Create: `internal/platform/windows/webview.go`
- Create: `internal/platform/windows/tray.go`
- Create: `internal/platform/windows/notification.go`
- Create: `internal/platform/windows/hotkey.go`
- Create: `internal/platform/windows/paster.go`
- Create: `internal/platform/windows/settings.go`
- Create: `internal/platform/windows/power.go`
- Create: `internal/platform/windows/*_test.go`
- Modify: `internal/platform/platform_windows.go`
- Modify: `cmd/joicetyper/main.go` or platform selection files as needed

**Step 1: Write the failing compile/test skeleton**

Add minimal tests proving:
- the Windows package builds
- the shared bridge service can be constructed from a Windows dependency bundle

**Step 2: Run the focused Windows build/test**

Run:

```bash
GOOS=windows GOARCH=amd64 go test ./internal/platform/windows -count=1
```

Expected: FAIL because the package does not exist yet.

**Step 3: Add the Windows package skeleton**

Implement only enough structure to:
- compile
- expose the same adapter-shaped seams Darwin uses
- avoid inventing Windows-only bridge entrypoints

**Step 4: Re-run the focused Windows package test**

Run:

```bash
GOOS=windows GOARCH=amd64 go test ./internal/platform/windows -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/platform/windows internal/platform cmd
git commit -m "feat: scaffold windows platform adapter"
```

### Task 7: Implement the Windows webview host and shared Preferences bridge

**Files:**
- Modify: `internal/platform/windows/webview.go`
- Modify: `internal/platform/windows/settings.go`
- Modify: `internal/platform/windows/*_test.go`
- Modify: `internal/core/bridge/api_test.go` if shared assumptions change

**Step 1: Write the failing tests**

Add tests for:
- building the Windows bridge service
- bootstrap payload creation
- request routing through the shared bridge
- `logs.updated` event publication path

**Step 2: Run the focused tests**

Run:

```bash
GOOS=windows GOARCH=amd64 go test ./internal/platform/windows -count=1
```

Expected: FAIL.

**Step 3: Implement the Windows webview host**

- host the shared embedded UI in `WebView2`
- inject the same shared bootstrap
- route bridge traffic through `internal/core/bridge`
- log native transport failures explicitly

**Step 4: Re-run focused tests**

Run:

```bash
GOOS=windows GOARCH=amd64 go test ./internal/platform/windows -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/platform/windows
git commit -m "feat: add windows web preferences host"
```

### Task 8: Implement Windows runtime adapters for hotkey, insertion, tray, notifications, and devices

**Files:**
- Modify: `internal/platform/windows/hotkey.go`
- Modify: `internal/platform/windows/paster.go`
- Modify: `internal/platform/windows/notification.go`
- Modify: `internal/platform/windows/tray.go`
- Modify: `internal/platform/windows/power.go`
- Modify: `internal/platform/windows/settings.go`
- Modify: `internal/platform/windows/*_test.go`
- Modify: shared runtime tests only if contract additions are truly needed

**Step 1: Write failing adapter tests**

Cover:
- hotkey capture/start/cancel/confirm contract hooks
- paste-first insertion with typing fallback path selection
- device listing/refresh hooks
- notification hook wiring

**Step 2: Run the focused tests**

Run:

```bash
GOOS=windows GOARCH=amd64 go test ./internal/platform/windows -count=1
```

Expected: FAIL.

**Step 3: Implement adapter behavior**

Keep these rules:
- use shared bridge methods/events only
- if a missing concept appears, stop and add it to the shared contract first
- do not add a Windows-only bridge shortcut

**Step 4: Re-run focused tests**

Run:

```bash
GOOS=windows GOARCH=amd64 go test ./internal/platform/windows -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/platform/windows
git commit -m "feat: add windows runtime integrations"
```

### Task 9: Add Windows packaging and installer flow

**Files:**
- Create: `packaging/windows/...`
- Modify: `Makefile`
- Modify: `README.md`
- Create/Modify: installer build scripts and assets under `packaging/windows`
- Test: `internal/buildinfra/repo_layout_test.go` if packaging layout rules are needed

**Step 1: Write the failing packaging/build guard**

- Add a test or build-infra guard that the Windows packaging home exists and the Makefile exposes the intended installer target.

**Step 2: Run the focused test**

Run:

```bash
go test ./internal/buildinfra -count=1
```

Expected: FAIL after adding the guard.

**Step 3: Implement installer packaging**

- add Windows build output pathing
- add installer build target
- feed app version from `VERSION`
- include the normal embedded UI bundle and runtime assets

**Step 4: Build the Windows artifacts**

Run:

```bash
make build-windows-amd64
make package-windows
```

Expected: PASS.

**Step 5: Re-run build-infra tests**

Run:

```bash
go test ./internal/buildinfra -count=1
```

Expected: PASS.

**Step 6: Commit**

```bash
git add packaging/windows Makefile README.md internal/buildinfra
git commit -m "build: add windows installer packaging"
```

### Task 10: Full verification and checkpoint

**Files:**
- Modify: `docs/plans/2026-04-20-windows-arch-support.md` only if verification notes are needed

**Step 1: Run shared contract verification**

Run:

```bash
go run ./scripts/generate_bridge_contract -check
```

Expected: PASS.

**Step 2: Run full repo tests**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

**Step 3: Run frontend build**

Run:

```bash
(cd ui && npm run build)
```

Expected: PASS.

**Step 4: Run packaging builds**

Run:

```bash
make app
make build-windows-amd64
make package-windows
```

Expected: PASS.

**Step 5: Inspect git diff hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected:
- `git diff --check` clean
- only intended tracked changes

**Step 6: Commit final checkpoint**

```bash
git add .
git commit -m "feat: add windows desktop support"
```

