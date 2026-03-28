#ifndef HOTKEY_DARWIN_H
#define HOTKEY_DARWIN_H

#include <stdint.h>

// startHotkeyListener creates a CGEvent tap monitoring modifier flags.
// targetFlags is the bitmask of modifier flags that must all be held to trigger.
// Returns 0 on success, -1 if event tap creation fails (no Accessibility permission).
int startHotkeyListener(uint64_t targetFlags);

// stopHotkeyListener disables the event tap and releases resources.
void stopHotkeyListener(void);

// runMainLoop runs CFRunLoopRun on the current thread. Blocks until stopMainLoop is called.
void runMainLoop(void);

// stopMainLoop stops the CFRunLoop started by runMainLoop.
void stopMainLoop(void);

#endif
