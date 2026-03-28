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

void ensureNSApp(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    });
}

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
    hotkeyFlagsChanged(flags);
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
    @autoreleasepool {
        // NSApplication is REQUIRED for a CLI process to receive system events.
        // Without it, CGEvent taps are created but never fire.
        // ensureNSApp() guarantees the singleton exists; it may have been
        // created earlier by the setup wizard or status bar.
        ensureNSApp();
        [NSApp run];
    }
}

void stopMainLoop(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp stop:nil];
        // Post dummy event to unblock [NSApp run]
        NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                            location:NSMakePoint(0, 0)
                                       modifierFlags:0
                                           timestamp:0
                                        windowNumber:0
                                             context:nil
                                             subtype:0
                                               data1:0
                                               data2:0];
        [NSApp postEvent:event atStart:YES];
    });
}
