#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include "paster_darwin.h"

int pasteText(const char* text) {
    @autoreleasepool {
        NSPasteboard* pb = [NSPasteboard generalPasteboard];

        // Save ALL current clipboard items (text, images, files, rich text, etc.)
        NSArray<NSPasteboardItem *> *oldItems = pb.pasteboardItems;
        NSMutableArray<NSDictionary<NSPasteboardType, NSData *> *> *savedItems = [NSMutableArray array];
        for (NSPasteboardItem *item in oldItems) {
            NSMutableDictionary<NSPasteboardType, NSData *> *itemData = [NSMutableDictionary dictionary];
            for (NSPasteboardType type in item.types) {
                NSData *data = [item dataForType:type];
                if (data != nil) {
                    itemData[type] = data;
                }
            }
            if (itemData.count > 0) {
                [savedItems addObject:itemData];
            }
        }

        // Save changeCount before we touch the clipboard.
        // clearContents (+1) and setString (+1) = savedChangeCount + 2.
        // If the count is anything else when we restore, another app wrote to the clipboard.
        NSInteger savedChangeCount = [pb changeCount];

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

        // Restore original clipboard after a delay (let the paste complete first).
        // Build the full item array first, then write atomically in one call.
        NSArray *restoreItems = [savedItems copy];
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(200 * NSEC_PER_MSEC)),
                       dispatch_get_main_queue(), ^{
            // Only restore if nobody else touched the clipboard during our paste.
            // We did clearContents (+1) and setString (+1) = savedChangeCount + 2.
            // If it differs, another app wrote to the clipboard — don't overwrite.
            if ([pb changeCount] != savedChangeCount + 2) {
                return;
            }
            if (restoreItems.count > 0) {
                NSMutableArray<NSPasteboardItem *> *itemsToRestore = [NSMutableArray array];
                for (NSDictionary<NSPasteboardType, NSData *> *itemData in restoreItems) {
                    NSPasteboardItem *newItem = [[NSPasteboardItem alloc] init];
                    for (NSPasteboardType type in itemData) {
                        [newItem setData:itemData[type] forType:type];
                    }
                    [itemsToRestore addObject:newItem];
                }
                [pb clearContents];
                [pb writeObjects:itemsToRestore];
            }
        });

        return 0;
    }
}
