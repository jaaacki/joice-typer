#ifndef HOTKEY_DARWIN_H
#define HOTKEY_DARWIN_H

#include <stdint.h>

// ensureNSApp initialises the NSApplication singleton exactly once.
// Safe to call multiple times from any code path (setup wizard, status bar, hotkey).
void ensureNSApp(void);

// checkAccessibility returns 1 if accessibility is granted, 0 if not.
// Silent check only — never shows a system dialog.
int checkAccessibility(void);

// checkInputMonitoring returns 1 if Input Monitoring is granted, 0 if not.
// Uses CGPreflightListenEventAccess — the official macOS API.
int checkInputMonitoring(void);

// startHotkeyListener creates a CGEvent tap monitoring modifier flags and optionally a key.
// targetFlags: bitmask of modifier flags that must all be held.
// targetKeycode: virtual keycode that must be pressed (pass -1 for modifier-only triggers).
// Returns 0 on success, -1 if event tap creation fails (no Accessibility permission).
int startHotkeyListener(uint64_t targetFlags, int targetKeycode);

// stopHotkeyListener disables the event tap and releases resources.
void stopHotkeyListener(void);

// setHotkeyListenerEnabled toggles the runtime hotkey event tap without tearing
// it down. Used to suspend hotkey detection during settings hotkey capture so
// the user's existing trigger doesn't fire while rebinding. Safe no-op when the
// tap isn't installed. enabled=1 resumes, enabled=0 suspends.
void setHotkeyListenerEnabled(int enabled);

// runMainLoop runs CFRunLoopRun on the current thread. Blocks until stopMainLoop is called.
void runMainLoop(void);

// stopMainLoop stops the CFRunLoop started by runMainLoop.
void stopMainLoop(void);

#endif
