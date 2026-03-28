#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include "paster_darwin.h"

int setClipboard(const char* text) {
    @autoreleasepool {
        NSPasteboard* pb = [NSPasteboard generalPasteboard];
        [pb clearContents];
        NSString* str = [NSString stringWithUTF8String:text];
        if (str == nil) {
            return 1;
        }
        BOOL ok = [pb setString:str forType:NSPasteboardTypeString];
        return ok ? 0 : 1;
    }
}

void simulateCmdV(void) {
    // 'v' key = keycode 0x09
    CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 0x09, true);
    CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);
    CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, 0x09, false);
    CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

    CGEventPost(kCGHIDEventTap, keyDown);
    CGEventPost(kCGHIDEventTap, keyUp);

    CFRelease(keyDown);
    CFRelease(keyUp);
}
