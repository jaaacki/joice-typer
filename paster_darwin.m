#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include "paster_darwin.h"

int pasteText(const char* text) {
    @autoreleasepool {
        NSPasteboard* pb = [NSPasteboard generalPasteboard];

        // Save current clipboard contents
        NSData *savedData = nil;
        NSString *savedType = nil;
        NSArray<NSString *> *types = [pb types];
        if ([types containsObject:NSPasteboardTypeString]) {
            savedData = [pb dataForType:NSPasteboardTypeString];
            savedType = NSPasteboardTypeString;
        }

        // Set clipboard to our text
        [pb clearContents];
        NSString* str = [NSString stringWithUTF8String:text];
        if (str == nil) {
            return 1;
        }
        BOOL ok = [pb setString:str forType:NSPasteboardTypeString];
        if (!ok) {
            return 2;
        }

        // Brief pause to let pasteboard settle before simulating keypress
        usleep(50000); // 50ms

        // Simulate Cmd+V
        // 'v' key = keycode 0x09
        CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 0x09, true);
        if (keyDown == NULL) return 3;
        CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);

        CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, 0x09, false);
        if (keyUp == NULL) {
            CFRelease(keyDown);
            return 4;
        }
        CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

        CGEventPost(kCGHIDEventTap, keyDown);
        CGEventPost(kCGHIDEventTap, keyUp);

        CFRelease(keyDown);
        CFRelease(keyUp);

        // Restore clipboard after a delay (let the paste complete first)
        NSData *restoreData = [savedData copy];
        NSString *restoreType = [savedType copy];
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(200 * NSEC_PER_MSEC)),
                       dispatch_get_main_queue(), ^{
            if (restoreData != nil) {
                [pb clearContents];
                [pb setData:restoreData forType:restoreType];
            }
        });

        return 0;
    }
}
