#ifndef HOTKEY_DARWIN_H
#define HOTKEY_DARWIN_H

#include <stdint.h>

// ensureNSApp initialises the NSApplication singleton exactly once.
// Safe to call multiple times from any code path (setup wizard, status bar, hotkey).
void ensureNSApp(void);

// checkAccessibility returns 1 if accessibility is granted, 0 if not.
// If prompt is 1, shows the macOS permission dialog.
int checkAccessibility(int prompt);

// checkInputMonitoring returns 1 if Input Monitoring is granted, 0 if not.
// If prompt is 1, shows the macOS permission dialog.
int checkInputMonitoring(int prompt);

// dispatch_permission_prompts triggers permission dialogs on the main queue.
// Must be called after [NSApp run] starts so the dialogs can process events.
void dispatch_permission_prompts(void);

// probeEventTap tries to create and immediately destroy a CGEvent tap.
// Returns 1 if creation succeeds (permissions are sufficient), 0 if not.
int probeEventTap(void);

// startHotkeyListener creates a CGEvent tap monitoring modifier flags and optionally a key.
// targetFlags: bitmask of modifier flags that must all be held.
// targetKeycode: virtual keycode that must be pressed (pass -1 for modifier-only triggers).
// Returns 0 on success, -1 if event tap creation fails (no Accessibility permission).
int startHotkeyListener(uint64_t targetFlags, int targetKeycode);

// stopHotkeyListener disables the event tap and releases resources.
void stopHotkeyListener(void);

// runMainLoop runs CFRunLoopRun on the current thread. Blocks until stopMainLoop is called.
void runMainLoop(void);

// stopMainLoop stops the CFRunLoop started by runMainLoop.
void stopMainLoop(void);

#endif
