#include "hotkey_darwin.h"
#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>
#import <Carbon/Carbon.h>
#import <ApplicationServices/ApplicationServices.h>
#import <IOKit/hidsystem/IOHIDLib.h>

// Defined in hotkey.go via //export
extern void hotkeyCallback(int eventType);
extern void hotkeyFlagsChanged(uint64_t flags);

int checkAccessibility(int prompt) {
    NSDictionary *options = @{(__bridge id)kAXTrustedCheckOptionPrompt: @(prompt ? YES : NO)};
    return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options) ? 1 : 0;
}

int checkInputMonitoring(int prompt) {
    if (prompt) {
        IOHIDRequestAccess(kIOHIDRequestTypeListenEvent);
    }
    return IOHIDCheckAccess(kIOHIDRequestTypeListenEvent) == kIOHIDAccessTypeGranted ? 1 : 0;
}

static CGEventRef probeCallback(CGEventTapProxy p, CGEventType t, CGEventRef e, void *u) {
    return e;
}

int probeEventTap(void) {
    CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged) | CGEventMaskBit(kCGEventKeyDown);
    CFMachPortRef tap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        probeCallback,
        NULL
    );
    if (tap == NULL) {
        return 0;
    }
    CFRelease(tap);
    return 1;
}

void ensureNSApp(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    });
}

static uint64_t sTargetFlags = 0;
static int sTargetKeycode = -1;
static int sTriggered = 0;
static CFMachPortRef sEventTap = NULL;
static CFRunLoopSourceRef sRunLoopSource = NULL;

static CGEventRef eventTapCallback(
    CGEventTapProxy proxy,
    CGEventType type,
    CGEventRef event,
    void *userInfo
) {
    // Re-enable tap if it gets disabled by the system (timeout)
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (sEventTap != NULL) {
            CGEventTapEnable(sEventTap, true);
        }
        return event;
    }

    uint64_t flags = CGEventGetFlags(event);
    hotkeyFlagsChanged(flags);

    if (sTargetKeycode < 0) {
        // Modifier-only mode (existing behavior)
        if (type != kCGEventFlagsChanged) return event;
        int allHeld = (flags & sTargetFlags) == sTargetFlags;
        if (allHeld && !sTriggered) {
            sTriggered = 1;
            hotkeyCallback(0);  // TriggerPressed
        } else if (!allHeld && sTriggered) {
            sTriggered = 0;
            hotkeyCallback(1);  // TriggerReleased
        }
    } else {
        // Modifier+key mode
        int modsHeld = (sTargetFlags == 0) || ((flags & sTargetFlags) == sTargetFlags);
        if (type == kCGEventKeyDown && modsHeld) {
            int keycode = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
            if (keycode == sTargetKeycode && !sTriggered) {
                sTriggered = 1;
                hotkeyCallback(0);  // TriggerPressed
            }
        } else if (type == kCGEventKeyUp) {
            int keycode = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
            if (keycode == sTargetKeycode && sTriggered) {
                sTriggered = 0;
                hotkeyCallback(1);  // TriggerReleased
            }
        } else if (type == kCGEventFlagsChanged && sTriggered) {
            // Modifiers released while key is held — treat as release
            if (!modsHeld) {
                sTriggered = 0;
                hotkeyCallback(1);  // TriggerReleased
            }
        }
    }

    return event;
}

int startHotkeyListener(uint64_t targetFlags, int targetKeycode) {
    // Clean up any existing tap before creating a new one.
    // This prevents stale taps from accumulating on re-entry.
    stopHotkeyListener();

    sTargetFlags = targetFlags;
    sTargetKeycode = targetKeycode;
    sTriggered = 0;

    CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged);
    if (targetKeycode >= 0) {
        mask |= CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp);
    }

    sEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        eventTapCallback,
        NULL
    );

    if (sEventTap == NULL) {
        return -1;
    }

    sRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, sEventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), sRunLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(sEventTap, true);

    return 0;
}

void stopHotkeyListener(void) {
    if (sEventTap != NULL) {
        CGEventTapEnable(sEventTap, false);
        CFRelease(sEventTap);
        sEventTap = NULL;
    }
    if (sRunLoopSource != NULL) {
        CFRunLoopRemoveSource(CFRunLoopGetCurrent(), sRunLoopSource, kCFRunLoopCommonModes);
        CFRelease(sRunLoopSource);
        sRunLoopSource = NULL;
    }
}

void runMainLoop(void) {
    // Use CFRunLoopRun instead of [NSApp run]. The CGEvent tap is a
    // Core Foundation construct that only needs CFRunLoop. Using [NSApp run]
    // caused [NSApp stopModal] from preferences to bleed into the hotkey
    // event loop, killing the listener. CFRunLoop is isolated from NSApp's
    // modal session management.
    ensureNSApp(); // still needed for status bar and preferences window
    CFRunLoopRun();
}

void stopMainLoop(void) {
    CFRunLoopStop(CFRunLoopGetMain());
}
