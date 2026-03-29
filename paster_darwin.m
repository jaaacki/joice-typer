#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include "paster_darwin.h"

// Clipboard save/restore strategy:
//
// Fast path: clipboard is empty or single plain-text item.
//   → Save only the text string (or nothing). Restore is trivial.
//
// Slow path: clipboard has images, files, rich text, or multiple items.
//   → Deep-copy all items/types. Restore reconstructs everything.
//
// This avoids paying the full deep-copy cost when the clipboard is simple,
// which covers the vast majority of real usage.

typedef enum {
    ClipboardEmpty,
    ClipboardPlainText,
    ClipboardComplex,
} ClipboardKind;

int pasteText(const char* text) {
    @autoreleasepool {
        NSPasteboard *pb = [NSPasteboard generalPasteboard];

        // --- Classify and snapshot the clipboard ---
        ClipboardKind kind = ClipboardEmpty;
        NSString *savedText = nil;
        NSMutableArray<NSDictionary<NSPasteboardType, NSData *> *> *savedItems = nil;

        NSArray<NSPasteboardItem *> *oldItems = pb.pasteboardItems;
        if (oldItems.count == 0) {
            kind = ClipboardEmpty;
        } else if (oldItems.count == 1 && oldItems[0].types.count <= 3) {
            // Single item with few types — check if it's just plain text.
            // NSPasteboardTypeString items typically have 1-3 types
            // (public.utf8-plain-text, public.utf16-plain-text, etc.)
            NSString *existingText = [pb stringForType:NSPasteboardTypeString];
            if (existingText != nil) {
                kind = ClipboardPlainText;
                savedText = [existingText copy];
            } else {
                // Single item but not plain text — fall through to complex
                kind = ClipboardComplex;
            }
        } else {
            kind = ClipboardComplex;
        }

        // Complex path: deep-copy all items (only when needed)
        if (kind == ClipboardComplex) {
            savedItems = [NSMutableArray array];
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
        }

        // --- Set clipboard to our text ---
        [pb clearContents];
        NSString *str = [NSString stringWithUTF8String:text];
        if (str == nil) return 1;
        if (![pb setString:str forType:NSPasteboardTypeString]) return 2;

        NSInteger postWriteChangeCount = [pb changeCount];

        // --- Simulate Cmd+V ---
        // No pre-paste delay — [pb setString:] is synchronous.

        CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 0x09, true); // V key
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

        // --- Restore clipboard after paste completes ---
        // 50ms for the target app to consume the paste event.
        NSArray *restoreComplex = [savedItems copy];
        NSString *restoreText = [savedText copy];
        ClipboardKind restoreKind = kind;

        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(50 * NSEC_PER_MSEC)),
                       dispatch_get_main_queue(), ^{
            // Only restore if nobody else touched the clipboard
            if ([pb changeCount] != postWriteChangeCount) return;

            switch (restoreKind) {
                case ClipboardEmpty:
                    [pb clearContents];
                    break;

                case ClipboardPlainText:
                    [pb clearContents];
                    if (restoreText != nil) {
                        [pb setString:restoreText forType:NSPasteboardTypeString];
                    }
                    break;

                case ClipboardComplex:
                    if (restoreComplex.count > 0) {
                        NSMutableArray<NSPasteboardItem *> *items = [NSMutableArray array];
                        for (NSDictionary<NSPasteboardType, NSData *> *itemData in restoreComplex) {
                            NSPasteboardItem *newItem = [[NSPasteboardItem alloc] init];
                            for (NSPasteboardType type in itemData) {
                                [newItem setData:itemData[type] forType:type];
                            }
                            [items addObject:newItem];
                        }
                        [pb clearContents];
                        [pb writeObjects:items];
                    } else {
                        [pb clearContents];
                    }
                    break;
            }
        });

        return 0;
    }
}
