# Audio Recovery Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make recorder failures self-heal so JoiceTyper recovers automatically from stale PortAudio or warm-stream state instead of appearing ready while audio is still broken.

**Architecture:** Keep warm-start as an optimization, but treat it as disposable. The recorder will detect fatal audio failures, mark itself unhealthy, perform a PortAudio reset, and retry once cold. The app will route start-failure recovery through the recorder instead of only returning to ready UI state.

**Tech Stack:** Go, PortAudio, macOS CoreAudio via PortAudio, existing app event loop and unit tests.

---

### Follow-up Scope

- Add a macOS sleep/wake observer so the recorder is proactively refreshed after wake when idle.
- Move the recording cue/state transition until after recorder start succeeds so failed starts do not look like active recording.

### Task 1: Lock Recovery Behavior With Tests

**Files:**
- Modify: `app_test.go`

**Step 1: Write failing tests**

- Add a test that simulates a recorder start failure on the first attempt, successful `RefreshDevices`, and a successful second start inside the same press.
- Add a test that simulates refresh failure and verifies the app stays responsive for later presses.

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestApp_.*Recovery' -count=1`

Expected: FAIL because the app does not retry or refresh the recorder today.

### Task 2: Implement Recorder Recovery Hooks

**Files:**
- Modify: `contracts.go`
- Modify: `recorder.go`

**Step 1: Extend recorder contract**

- Add a recovery method that can reset PortAudio state without reopening Preferences.

**Step 2: Implement minimal recorder recovery**

- Track unhealthy recorder state after read-loop hang or stream open/start failure.
- Drop stale warm streams during recovery.
- Terminate and re-initialize PortAudio, then re-warm opportunistically.

**Step 3: Run focused tests**

Run: `go test ./... -run 'TestApp_.*Recovery' -count=1`

Expected: PASS

### Task 3: Wire App-Level Retry

**Files:**
- Modify: `app.go`

**Step 1: Retry once after recorder start failure**

- On a dependency-unavailable start failure, trigger recorder recovery and retry once before surfacing an error.

**Step 2: Keep state transitions honest**

- Only fall back to ready after retry paths are exhausted.

**Step 3: Run targeted tests**

Run: `go test ./... -run 'TestApp_(RecorderStartFails_ContinuesListening|.*Recovery)' -count=1`

Expected: PASS

### Task 4: Verify Broader Behavior

**Files:**
- Modify: `app_test.go` if needed

**Step 1: Run broader relevant tests**

Run: `go test ./... -count=1`

Expected: PASS
