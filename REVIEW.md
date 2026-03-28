# Hostile Critic Review — VoiceType v1

## Verdict: REJECTED

---

## Critical Issues (MUST FIX)

### 1. [transcriber.go:40] Deprecated API — `whisper_init_from_file` is deprecated
Severity: **REJECT**

```go
ctx := C.whisper_init_from_file(cPath)
```

The project's own `third_party/whisper.cpp/include/whisper.h` (line 216-219) explicitly marks this function as deprecated:

```c
WHISPER_DEPRECATED(
    WHISPER_API struct whisper_context * whisper_init_from_file(const char * path_model),
    "use whisper_init_from_file_with_params instead"
);
```

This will generate compiler warnings on every build and may be removed in a future whisper.cpp release, breaking the build entirely.

**Fix:** Use the non-deprecated API:

```go
cPath := C.CString(modelPath)
defer C.free(unsafe.Pointer(cPath))

cparams := C.whisper_context_default_params()
ctx := C.whisper_init_from_file_with_params(cPath, cparams)
```

---

### 2. [config.go:86] Swallowed Error — `os.UserHomeDir()` error silently discarded
Severity: **REJECT**

```go
func DefaultConfigDir() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".config", "voicetype")
}
```

The error from `os.UserHomeDir()` is silently discarded with `_ =`. If this fails (e.g., `$HOME` is unset, running as a system daemon), `home` is the empty string, producing the path `"/.config/voicetype"` — which is the filesystem root. On the next call, `LoadConfig` will attempt `os.MkdirAll("/.config/voicetype", 0755)` and either fail with a permission error (confusing) or succeed in writing to the root filesystem (dangerous).

The spec says: "no silent failures, no `_ = err`, no swallowed returns." This is a direct violation.

**Fix:** Change `DefaultConfigDir`, `DefaultConfigPath`, and `DefaultModelPath` to return `(string, error)`:

```go
func DefaultConfigDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("config.DefaultConfigDir: %w", err)
    }
    return filepath.Join(home, ".config", "voicetype"), nil
}
```

---

### 3. [hotkey.go:16,20,25,27,68] Race Condition — Package-level `hotkeyEvents` channel accessed without synchronization
Severity: **REJECT**

```go
var hotkeyEvents chan<- HotkeyEvent  // line 16

//export hotkeyCallback
func hotkeyCallback(eventType C.int) {
    if hotkeyEvents == nil {     // line 20 — READ without lock
        return
    }
    switch int(eventType) {
    case 0:
        hotkeyEvents <- TriggerPressed   // line 25 — WRITE without lock
    case 1:
        hotkeyEvents <- TriggerReleased   // line 27 — WRITE without lock
    }
}

func (h *cgEventHotkeyListener) Start(events chan<- HotkeyEvent) error {
    // ...
    hotkeyEvents = events   // line 68 — WRITE without lock
```

`hotkeyCallback` is called from the C event tap callback on the CFRunLoop thread. `hotkeyEvents` is written in `Start()` which runs on the Go main goroutine. There is no synchronization (no mutex, no atomic) protecting this shared variable. This is a data race.

Additionally, using a package-level global means only one HotkeyListener can ever exist. If `Start` is called twice (even with a different listener instance), it silently overwrites the channel — violating the principle of no implicit behavior.

**Fix:** Use `atomic.Value` or a mutex to synchronize access to `hotkeyEvents`. At minimum, use `sync.Mutex`:

```go
var (
    hotkeyMu     sync.Mutex
    hotkeyEvents chan<- HotkeyEvent
)

//export hotkeyCallback
func hotkeyCallback(eventType C.int) {
    hotkeyMu.Lock()
    ch := hotkeyEvents
    hotkeyMu.Unlock()
    if ch == nil {
        return
    }
    // ...
}
```

---

### 4. [transcriber.go:65] Nil Pointer Dereference — `Transcribe` panics on empty audio
Severity: **REJECT**

```go
func (t *whisperTranscriber) Transcribe(audio []float32) (string, error) {
    // ...
    result := C.whisper_full(t.ctx, params, (*C.float)(unsafe.Pointer(&audio[0])), C.int(len(audio)))
```

If `audio` is an empty slice (len 0) or nil, `&audio[0]` will panic with an index-out-of-range error. The orchestrator flow in the spec says:

> if len(audio) == 0 -> log warning, continue

But the Transcriber itself has no guard. If the orchestrator fails to check (or a future caller forgets), this is a crash.

**Fix:** Add a nil/empty guard at the start of `Transcribe`:

```go
if len(audio) == 0 {
    return "", fmt.Errorf("transcriber.Transcribe: empty audio buffer")
}
```

---

### 5. [transcriber.go:114] Security / Spec Violation — `ensureModel` makes network calls; spec says "local/offline only"
Severity: **REJECT**

```go
resp, err := http.Get(url)
```

The CLAUDE.md non-negotiable requirements state: **"Local/offline only — no cloud APIs, no network calls, voice never leaves the machine."** The `ensureModel` function downloads a model from `huggingface.co` over HTTP. While the design spec allows this for first-run model download, the implementation has multiple problems:

1. **No TLS certificate validation or checksum verification.** A MITM attacker could serve a malicious binary as the model file. There is no SHA256 hash check after download.
2. **No timeout on `http.Get`.** The default `http.Client` has no timeout — if the server hangs, VoiceType hangs forever.
3. **No user consent.** The download happens silently. The user never opted in to a network call. At minimum, log a WARN and consider requiring explicit opt-in.

**Fix:**
- Add a configurable timeout: `client := &http.Client{Timeout: 5 * time.Minute}`
- Add SHA256 checksum verification after download.
- Log at WARN level before making any network call.
- Consider making auto-download opt-in rather than implicit.

---

### 6. [transcriber.go:93-150] Inconsistent Error Format — `ensureModel` errors lack `transcriber.` prefix
Severity: **REJECT**

All other files follow the pattern `component.operation: description` for error messages. But `ensureModel` uses a bare prefix:

```go
return fmt.Errorf("ensureModel: create dir: %w", err)       // line 111
return fmt.Errorf("ensureModel: download: %w", err)          // line 116
return fmt.Errorf("ensureModel: download returned status %d") // line 121
return fmt.Errorf("ensureModel: create file: %w", err)       // line 127
return fmt.Errorf("ensureModel: close file: %w", closeErr)   // line 138
return fmt.Errorf("ensureModel: write: %w", err)             // line 142
return fmt.Errorf("ensureModel: rename: %w", err)            // line 146
```

Should be `transcriber.ensureModel: ...` to match the project convention. Additionally, line 121 does not use `%w` wrapping — it creates a terminal error with no chain:

```go
return fmt.Errorf("ensureModel: download returned status %d", resp.StatusCode)
```

This makes it impossible to inspect the error programmatically.

**Fix:** Add `transcriber.` prefix to all `ensureModel` errors. While `status %d` has no underlying error to wrap, it should still follow the naming convention: `transcriber.ensureModel: download returned status %d`.

---

### 7. [recorder.go:78] Race Condition — `r.stream.Read()` called outside mutex while `Stop()` calls `r.stream.Stop()` concurrently
Severity: **REJECT**

```go
func (r *portaudioRecorder) readLoop() {
    defer close(r.done)
    for {
        if err := r.stream.Read(); err != nil {   // line 78 — NO LOCK
```

```go
func (r *portaudioRecorder) Stop() ([]float32, error) {
    r.mu.Lock()
    // ...
    r.recording = false
    r.mu.Unlock()

    if err := r.stream.Stop(); err != nil {   // line 112 — NO LOCK
```

`readLoop` calls `r.stream.Read()` in a tight loop without holding the mutex. Concurrently, `Stop()` sets `r.recording = false` and then calls `r.stream.Stop()`. Between the unlock and the `stream.Stop()` call, `readLoop` may be mid-`Read()`. PortAudio's documentation does not guarantee that `Pa_StopStream` is safe to call while `Pa_ReadStream` is in progress on another thread.

The code relies on `r.stream.Read()` returning an error after `r.stream.Stop()` is called, but this is a hope, not a guarantee — PortAudio may deadlock or corrupt state.

**Fix:** Either:
- Use PortAudio's callback-based API instead of the blocking read API, or
- Signal `readLoop` to stop (via a `done` channel or atomic flag) and wait for it to exit *before* calling `stream.Stop()`.

---

### 8. [recorder.go:112-113] Swallowed Error — `stream.Stop()` failure is logged but not returned
Severity: **REJECT**

```go
if err := r.stream.Stop(); err != nil {
    r.logger.Error("failed to stop stream", "operation", "Stop", "error", err)
}
```

If `stream.Stop()` fails, the error is logged but `Stop()` continues and returns the audio buffer with `nil` error. The caller has no idea the stream failed to stop. Same issue at line 119-120 with `stream.Close()`.

The spec says: "Every error handled — no silent failures." Logging is not handling. The caller deserves to know.

**Fix:** Accumulate errors and return them. At minimum:

```go
var errs []error
if err := r.stream.Stop(); err != nil {
    errs = append(errs, fmt.Errorf("recorder.Stop: stop stream: %w", err))
}
// ... collect audio ...
if len(errs) > 0 {
    return audio, errors.Join(errs...)
}
```

Or return the audio AND the error, letting the caller decide.

---

### 9. [config.go:40] Missing Error Path — `os.Stat` non-NotExist errors silently ignored
Severity: **REJECT**

```go
if _, err := os.Stat(path); os.IsNotExist(err) {
```

This only handles the `os.IsNotExist` case. If `os.Stat` returns a *different* error (e.g., permission denied, broken symlink, I/O error), that error is silently ignored, and the code falls through to `os.ReadFile(path)` which will likely fail with a confusing message.

**Fix:** Handle non-nil, non-NotExist errors explicitly:

```go
_, err := os.Stat(path)
if err != nil && !os.IsNotExist(err) {
    return Config{}, fmt.Errorf("config.LoadConfig: stat: %w", err)
}
if os.IsNotExist(err) {
    // create default...
}
```

---

### 10. [paster.go:28-29] Logging Violation — Sensitive data logged at INFO level
Severity: **REJECT**

```go
p.logger.Info("pasting", "operation", "Paste", "text_length", len(text))
```

While `text_length` itself is fine, the real issue is that this function logs at **INFO** level for a routine operation. Every single paste operation generates two INFO log entries ("pasting" and "pasted"). The spec says INFO is for **lifecycle** events (start, stop, ready). Routine operations at this frequency should be DEBUG.

Additionally, if the text itself were ever added to the log (easy mistake in a future change), this would be a data leak of dictated speech.

**Fix:** Change to DEBUG:

```go
p.logger.Debug("pasting", "operation", "Paste", "text_length", len(text))
```

---

## Warnings (SHOULD FIX)

### 11. [transcriber.go:49-50] Log Level — Routine transcription logged at INFO
Severity: **WARN**

```go
t.logger.Info("transcribing", "operation", "Transcribe", "samples", len(audio))
```

Every single transcription emits two INFO entries. This is a hot path. The spec says INFO is for lifecycle, not per-operation. Should be DEBUG.

---

### 12. [recorder.go:45] Log Level — Every recording start/stop logged at INFO
Severity: **WARN**

```go
r.logger.Info("starting", "operation", "Start", "sample_rate", r.sampleRate)
```

Similar to above — every push-to-talk cycle generates 4+ INFO entries across recorder alone. This floods the log. Start/stop of *recording* is a repeated operation, not a lifecycle event.

**Fix:** Use DEBUG for per-recording events. Reserve INFO for the initial "recorder initialized" lifecycle event.

---

### 13. [logger_test.go:109-120] Code Quality — Reimplemented `strings.Contains`
Severity: **WARN**

```go
func contains(s, substr string) bool {
    return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}
```

This is a manual reimplementation of `strings.Contains` from the standard library. It is used in both `logger_test.go` and `config_test.go` (via the same package). There is zero reason to rewrite standard library functions.

**Fix:** Replace with `strings.Contains`:

```go
import "strings"
// delete contains() and containsSubstring()
// replace all calls with strings.Contains(s, substr)
```

---

### 14. [hotkey.go:90-99] Contract Compliance — `keysToFlags` only supports modifier keys, but `validKeys` in config.go allows regular keys
Severity: **WARN**

```go
var keyToFlag = map[string]uint64{
    "fn": flagFn, "shift": flagShift, "ctrl": flagCtrl, "option": flagOption, "cmd": flagCmd,
}
```

`config.go` validates trigger keys against `validKeys` which includes `"space"`, `"a"`-`"z"`, and `"f1"`-`"f12"`. But `keyToFlag` in `hotkey.go` only maps 5 modifier keys. If a user configures `trigger_key: ["ctrl", "space"]`, config validation passes, but `keysToFlags` returns an error at runtime: `key "space" is not a modifier`.

This is a disconnect between what config allows and what hotkey supports. The user gets a confusing delayed error.

**Fix:** Either:
- Restrict `validKeys` in config.go to only the 5 supported modifiers, or
- Implement support for non-modifier keys in the hotkey listener (requires CGEvent keycode monitoring, not just flag monitoring).

---

### 15. [paster_darwin.m:20-28] Resource Safety — No nil check on `CGEventCreateKeyboardEvent` return value
Severity: **WARN**

```c
CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 0x09, true);
CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);
CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, 0x09, false);
CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

CGEventPost(kCGHIDEventTap, keyDown);
CGEventPost(kCGHIDEventTap, keyUp);

CFRelease(keyDown);
CFRelease(keyUp);
```

`CGEventCreateKeyboardEvent` can return `NULL` if the event source is invalid or the system is under memory pressure. If it returns `NULL`, `CGEventSetFlags(NULL, ...)` and `CGEventPost(kCGHIDEventTap, NULL)` are undefined behavior, and `CFRelease(NULL)` is a crash (unlike `free(NULL)` which is safe).

**Fix:** Add nil checks:

```c
CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 0x09, true);
if (keyDown == NULL) return;
// ... same for keyUp
```

---

### 16. [hotkey_darwin.m:64] Resource Leak — `sRunLoopSource` not removed from run loop before release
Severity: **WARN**

```c
void stopHotkeyListener(void) {
    if (sEventTap != NULL) {
        CGEventTapEnable(sEventTap, false);
        CFRelease(sEventTap);
        sEventTap = NULL;
    }
    if (sRunLoopSource != NULL) {
        CFRelease(sRunLoopSource);   // line 79 — released without removing from run loop
        sRunLoopSource = NULL;
    }
}
```

The run loop source was added to `CFRunLoopGetMain()` in `startHotkeyListener` but is never removed via `CFRunLoopRemoveSource` before being released. This can cause the run loop to reference a freed object.

**Fix:** Remove the source from the run loop before releasing:

```c
if (sRunLoopSource != NULL) {
    CFRunLoopRemoveSource(CFRunLoopGetMain(), sRunLoopSource, kCFRunLoopCommonModes);
    CFRelease(sRunLoopSource);
    sRunLoopSource = NULL;
}
```

---

### 17. [transcriber.go:136-142] Logic Bug — Copy error and close error handling order is wrong
Severity: **WARN**

```go
written, err := io.Copy(f, pr)
if closeErr := f.Close(); closeErr != nil {
    return fmt.Errorf("ensureModel: close file: %w", closeErr)
}
if err != nil {
    os.Remove(tmpPath)
    return fmt.Errorf("ensureModel: write: %w", err)
}
```

If `io.Copy` fails AND `f.Close()` also fails, the close error is returned and the copy error is lost. The close error is typically less informative (e.g., "bad file descriptor") while the copy error tells you what actually went wrong (e.g., "disk full").

Additionally, when `io.Copy` fails but `f.Close()` succeeds, `os.Remove(tmpPath)` runs — but its error is silently discarded.

**Fix:** Check the copy error first, always close, then report:

```go
written, copyErr := io.Copy(f, pr)
closeErr := f.Close()
if copyErr != nil {
    os.Remove(tmpPath)
    return fmt.Errorf("transcriber.ensureModel: write: %w", copyErr)
}
if closeErr != nil {
    os.Remove(tmpPath)
    return fmt.Errorf("transcriber.ensureModel: close file: %w", closeErr)
}
```

---

### 18. [sound.go:27] Error Context — Sound play error logged without path context
Severity: **WARN**

```go
if err := cmd.Run(); err != nil {
    s.logger.Error("failed to play sound",
        "operation", "Play",
        "sound", name,
        "error", err,
    )
}
```

The error log includes `sound` (just the name like "Tink") but not the full `path`. If the sound file is missing or the path is wrong, the user has to mentally reconstruct the full path to debug.

**Fix:** Add `"path", path` to the log entry.

---

### 19. [contracts.go:3] Unused Import in Contract — `fmt` imported only for `String()` method
Severity: **WARN**

The `contracts.go` file is described as containing "ABSOLUTE" interfaces. The `fmt.Sprintf` in the `String()` method's default case is the only reason `fmt` is imported. This is not a bug, but the `HotkeyEvent.String()` method and its `fmt` import are implementation details polluting the contracts file. The default case (`HotkeyEvent(%d)`) should never be reached in correct code. If it matters, move `String()` to `hotkey.go`.

---

### 20. [recorder.go:135-136] Defensive Check — `Stop()` returns `nil, nil` for zero audio
Severity: **WARN**

```go
if total == 0 {
    r.logger.Warn("no audio captured", "operation", "Stop")
    return nil, nil
}
```

Returning `nil, nil` (no audio, no error) forces the caller to handle a nil slice as a non-error condition. This is ambiguous. The spec's orchestrator flow says `if len(audio) == 0 -> log warning, continue`. But `nil` and empty slice have different semantics in Go. The recorder should return an empty non-nil slice, or an explicit error, to be unambiguous.

---

## Summary

| Metric | Count |
|---|---|
| Total files reviewed | 15 (8 Go, 4 ObjC/C headers, 1 YAML, 1 spec, 1 test) |
| Critical issues (REJECT) | 10 |
| Warnings (SHOULD FIX) | 10 |
| **VERDICT** | **REJECTED** |

### Breakdown by Category

- **Deprecated API**: 1 critical (whisper_init_from_file)
- **Swallowed errors**: 2 critical (UserHomeDir, stream.Stop)
- **Race conditions**: 2 critical (hotkeyEvents global, stream.Read vs stream.Stop)
- **Nil pointer / crash risk**: 1 critical (empty audio to Transcribe)
- **Security**: 1 critical (unvalidated HTTP download, no timeout, no checksum)
- **Error format inconsistency**: 1 critical (ensureModel prefix)
- **Missing error path**: 1 critical (os.Stat non-NotExist)
- **Logging violations**: 1 critical + 2 warnings (INFO on hot paths)
- **Resource safety**: 2 warnings (CGEvent nil, run loop source leak)
- **Code quality**: 2 warnings (reimplemented strings.Contains, ambiguous nil return)
- **Contract drift**: 1 warning (config allows keys hotkey cannot handle)

### VERDICT: REJECTED

All 10 critical issues must be fixed before this code is mergeable. The race conditions (#3, #7), the deprecated API (#1), and the nil-pointer crash (#4) are the most dangerous. The swallowed `UserHomeDir` error (#2) is the most embarrassing — it is the exact pattern the spec explicitly forbids.
