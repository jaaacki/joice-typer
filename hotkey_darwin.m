#include "hotkey_darwin.h"
#import <CoreGraphics/CoreGraphics.h>
#import <Carbon/Carbon.h>

// Defined in hotkey.go via //export
extern void hotkeyCallback(int eventType);

static uint64_t sTargetFlags = 0;
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

    if (type != kCGEventFlagsChanged) {
        return event;
    }

    uint64_t flags = CGEventGetFlags(event);
    int allHeld = (flags & sTargetFlags) == sTargetFlags;

    if (allHeld && !sTriggered) {
        sTriggered = 1;
        hotkeyCallback(0);  // TriggerPressed
    } else if (!allHeld && sTriggered) {
        sTriggered = 0;
        hotkeyCallback(1);  // TriggerReleased
    }

    return event;
}

int startHotkeyListener(uint64_t targetFlags) {
    sTargetFlags = targetFlags;
    sTriggered = 0;

    CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged);

    sEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        eventTapCallback,
        NULL
    );

    if (sEventTap == NULL) {
        return -1;  // Accessibility permission not granted
    }

    sRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, sEventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetMain(), sRunLoopSource, kCFRunLoopCommonModes);
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
        CFRunLoopRemoveSource(CFRunLoopGetMain(), sRunLoopSource, kCFRunLoopCommonModes);
        CFRelease(sRunLoopSource);
        sRunLoopSource = NULL;
    }
}

void runMainLoop(void) {
    CFRunLoopRun();
}

void stopMainLoop(void) {
    CFRunLoopStop(CFRunLoopGetMain());
}
