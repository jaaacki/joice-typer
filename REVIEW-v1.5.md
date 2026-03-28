# Hostile Critic Review: JoiceTyper v1.5

**Reviewer**: Hostile Critic
**Date**: 2026-03-28
**Scope**: All new and modified files for v1.5 (app bundle, setup wizard, status bar, notifications)

---

## REJECT-Level Issues

### R1. runAppMode is a stub -- setup wizard and status bar are dead code
**File**: `main.go:66-75`
**Severity**: REJECT

`runAppMode()` contains a `TODO` comment and immediately falls back to `runTerminalMode`. This means:
- `RunSetupWizard()` in `setup.go` is never called.
- `InitStatusBar()` and `UpdateStatusBar()` in `statusbar_appkit.go` are never called.
- `PostNotification()` in `notification.go` is never called.
- The `IsFirstRun()` check is never performed.

The entire v1.5 feature set -- setup wizard, status bar, notifications -- is unreachable. Building `JoiceTyper.app` via `make app` produces a binary that behaves identically to the terminal binary. The spec explicitly requires app mode to show the setup wizard on first run and the menu bar icon on subsequent runs.

```go
func runAppMode() {
    // TODO: Will be implemented when setup wizard and status bar are integrated.
    // For now, fall back to terminal mode.
    cfgPath, err := DefaultConfigPath()
    ...
    runTerminalMode(cfgPath)
}
```

**Fix**: Implement `runAppMode` to: (1) call `IsFirstRun()`, (2) if first run, call `RunSetupWizard()`, (3) wire `SetStateCallback` to `UpdateStatusBar`, (4) call `InitStatusBar()`, (5) call `PostNotification()` on first launch after setup, (6) call `suppressStderr()`.

---

### R2. Setup wizard never writes the config file
**File**: `setup.go:33-99`
**Severity**: REJECT

`RunSetupWizard` returns `("", nil)` unconditionally on line 99. It never:
1. Calls `C.getSelectedDevice()` to read the user's mic selection.
2. Calls `C.isSetupComplete()` to check if the user clicked "Start JoiceTyper" vs closing the window.
3. Writes a config file with the selected device name.

The spec says: "Click closes wizard, writes config, activates menu bar icon." The setup wizard collects a microphone selection but throws it away. Even if `runAppMode` were implemented, the wizard would finish and produce no config file, so the next launch would show the wizard again -- infinite first-run loop.

```go
// Return -- the caller runs [NSApp run] which processes events until Continue is clicked
return "", nil
```

**Fix**: After `[NSApp run]` returns (which `RunSetupWizard` doesn't even call -- it returns immediately before the event loop runs), check `C.isSetupComplete()`, read `C.getSelectedDevice()`, construct a `Config` struct, and write it to `DefaultConfigPath()`.

---

### R3. Setup wizard returns before NSApp run loop -- goroutines call into dead UI
**File**: `setup.go:40-99`
**Severity**: REJECT

`RunSetupWizard` launches two goroutines (accessibility polling on line 41, model download on line 77), then immediately returns on line 99. The comment says "the caller runs [NSApp run]" but no caller does. This means:
1. `showSetupWindow()` creates the window, but without `[NSApp run]`, the window never processes events -- it is frozen.
2. The goroutines call `dispatch_async(dispatch_get_main_queue(), ...)` to update UI, but the main run loop is not running, so these blocks queue up and never execute.
3. The accessibility polling goroutine runs forever (no cancellation mechanism -- no context, no done channel).

Even if a caller did run `[NSApp run]`, the goroutines capture `C` function pointers and call `dispatch_async` from Go goroutines which may be on arbitrary OS threads. This is fine for `dispatch_async` specifically (it's thread-safe), but the lack of any coordination between the goroutines and the setup completion is problematic.

**Fix**: `RunSetupWizard` must either call `[NSApp run]` itself (blocking until the user clicks Continue) or clearly document that the caller is responsible. The accessibility goroutine needs a cancellation signal (e.g., a `done` channel closed when setup completes). Currently it leaks.

---

### R4. populateSetupDevices called from main thread dispatches to main thread -- potential race with freed C strings
**File**: `setup.go:64-72`
**Severity**: REJECT

`populateSetupDevices` is called synchronously from the main goroutine (not in a goroutine), passing `&cNames[0]` -- a slice of `*C.char` pointers. But `populateSetupDevices` in `setup_darwin.m:186` dispatches the actual work to `dispatch_get_main_queue()` asynchronously. Meanwhile, `setup.go:70-72` immediately frees all the C strings:

```go
C.populateSetupDevices(&cNames[0], C.int(len(inputNames)), 0)
for _, cn := range cNames {
    C.free(unsafe.Pointer(cn))  // Freed immediately
}
```

The `dispatch_async` block in `setup_darwin.m:187-195` reads `deviceNames[i]` after the C strings have been freed. This is a use-after-free bug. It may appear to work because `dispatch_async` to the main queue typically executes quickly when the run loop is active, but it is undefined behavior.

**Fix**: Either (a) make `populateSetupDevices` synchronous (use `dispatch_sync` or execute directly since it's already called from the main thread context), or (b) copy the strings inside the C function before dispatching, or (c) don't free the C strings until after the dispatch block has executed.

---

### R5. Accessibility polling goroutine leaks -- runs forever with no shutdown
**File**: `setup.go:41-49`
**Severity**: REJECT

The goroutine that polls accessibility permission has no exit condition other than "granted." If the user closes the setup window without granting accessibility, this goroutine runs `time.Sleep(2 * time.Second)` in an infinite loop until the process exits. There is no `context.Context`, no `done` channel, no way to cancel it.

```go
go func() {
    for {
        granted := C.checkAccessibility(1) == 1
        C.updateSetupAccessibility(boolToCInt(granted))
        if granted {
            return
        }
        time.Sleep(2 * time.Second)
    }
}()
```

Additionally, after the setup window is closed, this goroutine continues calling `C.updateSetupAccessibility` which calls `dispatch_async` to update UI elements (`sStep1Indicator`, `sStep1Status`) that belong to a deallocated window. This is a use-after-free / zombie UI update.

**Fix**: Pass a `context.Context` or `done` channel, and cancel it when the setup window closes (either via Continue or window close).

---

### R6. notification_darwin.m uses title/body pointers inside async completion handler -- use-after-free
**File**: `notification_darwin.m:10-26`
**Severity**: REJECT

The `postNotification` function wraps everything in `@autoreleasepool`. The `requestAuthorizationWithOptions` completion handler is asynchronous -- it runs later, after `postNotification` has returned. Inside that handler, it uses `title` and `body` (the `const char *` parameters):

```objc
content.title = [NSString stringWithUTF8String:title];
content.body = [NSString stringWithUTF8String:body];
```

But `title` and `body` are `C.CString` allocations freed by `defer C.free` in `notification.go:17-18`. By the time the completion handler runs, these pointers are freed. This is a use-after-free.

```go
cTitle := C.CString(title)
cBody := C.CString(body)
defer C.free(unsafe.Pointer(cTitle))
defer C.free(unsafe.Pointer(cBody))
C.postNotification(cTitle, cBody)
// cTitle and cBody freed here, but completion handler hasn't run yet
```

**Fix**: Copy the strings to `NSString` objects before the async call, or copy them inside `postNotification` before starting the async operation.

---

### R7. SetStateCallback is not thread-safe
**File**: `app.go:55-57`
**Severity**: REJECT

`SetStateCallback` writes `a.onStateChange` without synchronization:

```go
func (a *App) SetStateCallback(fn func(AppState)) {
    a.onStateChange = fn
}
```

Meanwhile, `handlePress`, `handleRelease`, and `transcribeAndPaste` all read `a.onStateChange` from different goroutines (the event loop goroutine and the transcription goroutine). This is a data race. While in practice `SetStateCallback` is likely called once before `Run`, there is no guarantee or documentation of this constraint, and the Go race detector would flag it.

**Fix**: Either (a) make `onStateChange` an `atomic.Value`, (b) protect it with a mutex, or (c) require it to be set in `NewApp` (pass it as a parameter) so it's immutable after construction.

---

## WARN-Level Issues

### W1. suppressStderr leaks file descriptor
**File**: `main.go:57-63`
**Severity**: WARN

`suppressStderr` opens a file but never stores the handle for later closing. The `Dup2` redirects fd 2 to the file's fd, but the original file handle `f` is never closed. This means two file descriptors point to the same file -- `f.Fd()` and fd 2. The `f` handle will never be garbage collected because the GC doesn't know it should close the underlying fd.

```go
f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
if err != nil {
    return
}
syscall.Dup2(int(f.Fd()), 2)
// f is never closed, leaked
```

**Fix**: After `Dup2`, close the original `f` (the fd has been duplicated, so the underlying file remains open via fd 2).

---

### W2. statusbar_appkit.go statusBarQuitClicked silently swallows FindProcess error
**File**: `statusbar_appkit.go:27-30`
**Severity**: WARN

```go
func statusBarQuitClicked() {
    p, err := os.FindProcess(os.Getpid())
    if err != nil {
        return  // Silent failure
    }
    p.Signal(syscall.SIGTERM)
}
```

The error from `p.Signal` is also unchecked. Both errors are silently swallowed. Per project rules: "zero silent failures." If quitting fails, the user clicks Quit and nothing happens with no feedback.

**Fix**: Log errors using slog (requires making the logger accessible to this function).

---

### W3. statusbar_darwin.m createBubbleJIcon does not check for nil NSImage
**File**: `statusbar_darwin.m:13`
**Severity**: WARN

`[[NSImage alloc] initWithSize:]` could theoretically return nil (out of memory). The code immediately calls `[image lockFocus]` on it without a nil check.

**Fix**: Add nil check before `lockFocus`.

---

### W4. setup_darwin.m does not check NSWindow allocation for nil
**File**: `setup_darwin.m:67-71`
**Severity**: WARN

`[[NSWindow alloc] initWithContentRect:...]` is not checked for nil before accessing properties like `delegate`, `contentView`, etc.

**Fix**: Add nil check after window creation.

---

### W5. Makefile app target does not set up Resources directory for icon
**File**: `Makefile:43-51`
**Severity**: WARN

The `app` target conditionally copies `icon.icns` only if it exists. But no target or instruction generates `icon.icns`. The spec says "Generates icon.icns from the Bubble J SVG via iconutil (or embeds a pre-built .icns)." Neither happens. The app will have no icon.

```makefile
@if [ -f icon.icns ]; then cp icon.icns $(APP_BUNDLE)/Contents/Resources/; fi
```

**Fix**: Either generate the icon programmatically in the build, include a pre-built `.icns`, or at minimum log a warning when the icon is missing.

---

### W6. setup.go does not initialize PortAudio before calling portaudio.Devices()
**File**: `setup.go:54`
**Severity**: WARN

`RunSetupWizard` calls `portaudio.Devices()` on line 54, but there is no `InitAudio()` / `portaudio.Initialize()` call before it. If `RunSetupWizard` is called before `InitAudio()` (which it would be, since app mode would call setup before the normal startup sequence), `portaudio.Devices()` will likely fail or return garbage.

**Fix**: Call `InitAudio()` before device enumeration, and `TerminateAudio()` after (or defer it).

---

### W7. Model download error in setup wizard silently fails -- user sees frozen progress bar
**File**: `setup.go:91-93`
**Severity**: WARN

When the model download fails, the goroutine logs the error and returns, but never updates the UI. The user sees a stuck progress bar forever. `updateSetupReady()` is never called, so the Continue button stays disabled. The user's only option is to close the window.

```go
if dlErr != nil {
    l.Error("model download failed", "operation", "RunSetupWizard", "error", dlErr)
    return  // UI stays stuck at partial progress
}
```

**Fix**: Add an error update function to the setup UI (e.g., show "Download failed - check connection" in the progress label) and/or enable a retry mechanism.

---

### W8. Default config creation in LoadConfig is wrong for first-run flow
**File**: `config.go:38-45`
**Severity**: WARN

`LoadConfig` auto-creates a config file with defaults when the file doesn't exist. This conflicts with `IsFirstRun()` which checks for config file existence. If `LoadConfig` runs before `IsFirstRun` (which it does in the current `runTerminalMode` flow), first-run detection will never trigger because `LoadConfig` creates the file.

In the designed app mode flow, `IsFirstRun()` should be called before `LoadConfig()`. But this dependency is fragile and undocumented.

**Fix**: Either (a) make `IsFirstRun` check something other than config file existence (e.g., a `.setup-complete` sentinel), or (b) split `LoadConfig` into `LoadConfig` (errors if not found) and `LoadOrCreateConfig`, or (c) document the ordering requirement.

---

### W9. setup_darwin.m sSelectedDeviceBuffer is a fixed 512-byte buffer
**File**: `setup_darwin.m:18, 233-251`
**Severity**: WARN

Device names are truncated at 511 bytes. This is documented behavior (the code does truncate properly), but some audio interface names can be long (e.g., "Universal Audio Volt 476 + Thunderbolt"). The truncation is silent.

---

### W10. statusbar_darwin.m updateStatusBar sets sStatusMenuItem.enabled = (state == 1) -- menu item is non-interactive in most states
**File**: `statusbar_darwin.m:106`
**Severity**: WARN

When the status menu item is "Ready -- Fn+Shift to dictate," it becomes enabled (clickable), but clicking it does nothing because no action is assigned (`action:nil` on line 88). This is confusing UX -- the item looks clickable but does nothing.

**Fix**: Either keep it always disabled, or assign an action that does something useful (e.g., shows a help message).

---

### W11. The spec says transcriber.go is unchanged but it was modified
**File**: `transcriber.go`
**Severity**: WARN

The spec under "Unchanged Files" lists `transcriber.go`. However, `transcriber.go` was modified to extract `downloadModelWithProgress` from `ensureModel` and add the `DownloadProgressFunc` type and `callbackProgressReader` struct. The git status confirms this. While the changes are reasonable, the spec is inaccurate.

---

### W12. notification_darwin.m does not handle UNUserNotificationCenter being nil
**File**: `notification_darwin.m:7`
**Severity**: WARN

`[UNUserNotificationCenter currentNotificationCenter]` could return nil if the app is not properly bundled or on unsupported configurations. The code calls methods on it without a nil check.

---

### W13. setup_darwin.m uses deprecated NSBezelStyleRounded
**File**: `setup_darwin.m:158`
**Severity**: WARN

`NSBezelStyleRounded` was renamed to `NSBezelStyleRegularSquare` / replaced in newer macOS SDKs. While it still compiles, it may produce warnings. Minor.

---

## Summary of Issues

| # | Severity | File | Issue |
|---|----------|------|-------|
| R1 | REJECT | main.go:66-75 | `runAppMode` is a TODO stub -- entire v1.5 feature set unreachable |
| R2 | REJECT | setup.go:99 | Setup wizard never writes config file, never reads selected device |
| R3 | REJECT | setup.go:40-99 | Wizard returns before NSApp run loop -- goroutines update dead UI |
| R4 | REJECT | setup.go:64-72 | Use-after-free: C strings freed before dispatch_async reads them |
| R5 | REJECT | setup.go:41-49 | Accessibility goroutine leaks, updates deallocated UI after close |
| R6 | REJECT | notification_darwin.m:10-26 | Use-after-free: C string pointers used in async completion handler |
| R7 | REJECT | app.go:55-57 | SetStateCallback is a data race (unsynchronized write to function pointer) |
| W1 | WARN | main.go:57-63 | suppressStderr leaks file handle |
| W2 | WARN | statusbar_appkit.go:27-30 | Quit handler swallows errors silently |
| W3 | WARN | statusbar_darwin.m:13 | No nil check on NSImage alloc |
| W4 | WARN | setup_darwin.m:67-71 | No nil check on NSWindow alloc |
| W5 | WARN | Makefile:43-51 | No icon.icns exists or is generated |
| W6 | WARN | setup.go:54 | PortAudio not initialized before Devices() call |
| W7 | WARN | setup.go:91-93 | Download failure leaves UI stuck with no user feedback |
| W8 | WARN | config.go:38-45 | LoadConfig auto-creates file, breaking IsFirstRun detection |
| W9 | WARN | setup_darwin.m:18 | Fixed 512-byte device name buffer with silent truncation |
| W10 | WARN | statusbar_darwin.m:106 | Status menu item is clickable but has no action |
| W11 | WARN | transcriber.go | Spec says unchanged but file was modified |
| W12 | WARN | notification_darwin.m:7 | No nil check on UNUserNotificationCenter |
| W13 | WARN | setup_darwin.m:158 | Uses deprecated NSBezelStyleRounded |

---

## VERDICT: REJECTED

7 REJECT-level issues found. The most fundamental problem is that `runAppMode` is a stub -- the entire v1.5 feature set (setup wizard, status bar, notifications) is dead code that cannot be reached through any code path. Beyond that, there are three use-after-free bugs (R4, R5, R6) involving async dispatch with freed pointers, a goroutine leak (R5), and a data race (R7). The setup wizard does not write the config file (R2) and does not actually run its event loop (R3).

The Objective-C UI code itself (setup_darwin.m, statusbar_darwin.m) is well-structured and reasonably clean, but the Go orchestration layer that is supposed to wire it all together is either missing or broken. The v1.5 changes compile but do not function.
