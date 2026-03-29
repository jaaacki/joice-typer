#import <Cocoa/Cocoa.h>
#include "statusbar_darwin.h"
#include "hotkey_darwin.h"

// Defined in statusbar_appkit.go via //export
extern void statusBarQuitClicked(void);
extern void statusBarPreferencesClicked(void);

static NSStatusItem *sStatusItem = nil;
static NSMenu *sMenu = nil;
static NSMenuItem *sStatusMenuItem = nil;

// Draw the Bubble J icon with the given color
static NSImage *createBubbleJIcon(NSColor *color) {
    NSImage *image = [[NSImage alloc] initWithSize:NSMakeSize(18, 18)];
    [image lockFocus];

    NSBezierPath *bubble = [NSBezierPath bezierPath];
    // Rounded rect body
    [bubble appendBezierPathWithRoundedRect:NSMakeRect(1, 5, 16, 12)
                                   xRadius:3 yRadius:3];
    // Speech tail
    [bubble moveToPoint:NSMakePoint(5, 5)];
    [bubble lineToPoint:NSMakePoint(4, 1)];
    [bubble lineToPoint:NSMakePoint(9, 5)];
    [bubble closePath];

    [color setStroke];
    [bubble setLineWidth:1.2];
    [bubble stroke];

    // Draw "J"
    NSDictionary *attrs = @{
        NSFontAttributeName: [NSFont boldSystemFontOfSize:8],
        NSForegroundColorAttributeName: color
    };
    NSString *j = @"J";
    NSSize textSize = [j sizeWithAttributes:attrs];
    NSPoint textPoint = NSMakePoint(
        (18 - textSize.width) / 2,
        5 + (12 - textSize.height) / 2
    );
    [j drawAtPoint:textPoint withAttributes:attrs];

    [image unlockFocus];
    [image setTemplate:NO];
    return image;
}

static NSColor *colorForState(int state) {
    switch (state) {
        case 0: return [NSColor grayColor];           // loading
        case 1: return [NSColor systemGreenColor];     // ready
        case 2: return [NSColor systemRedColor];       // recording
        case 3: return [NSColor systemBlueColor];      // transcribing
        case 4: return [NSColor systemOrangeColor];    // no permission
        case 5: return [NSColor systemYellowColor];    // dependency stuck
        default: return [NSColor grayColor];
    }
}

static NSString *textForState(int state) {
    switch (state) {
        case 0: return @"Loading model...";
        case 1: return @"✅ Ready — Fn+Shift to dictate";
        case 2: return @"🔴 Recording...";
        case 3: return @"🔵 Transcribing...";
        case 4: return @"⚠️ Grant Accessibility + Input Monitoring in System Settings";
        case 5: return @"⚠️ Speech engine not responding — try again or restart";
        default: return @"Unknown";
    }
}

@interface StatusBarDelegate : NSObject
@end

@implementation StatusBarDelegate
- (void)quitClicked:(id)sender {
    statusBarQuitClicked();
}
- (void)preferencesClicked:(id)sender {
    statusBarPreferencesClicked();
}
@end

static StatusBarDelegate *sDelegate = nil;

void initStatusBar(void) {
    // Must initialize NSApplication with correct activation policy BEFORE
    // touching any AppKit objects. Without this, [NSStatusBar systemStatusBar]
    // implicitly creates NSApp with the default Regular policy, which breaks
    // CGEvent tap delivery.
    ensureNSApp();

    sDelegate = [[StatusBarDelegate alloc] init];

    sStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    sStatusItem.button.image = createBubbleJIcon([NSColor grayColor]);

    sMenu = [[NSMenu alloc] init];
    sStatusMenuItem = [sMenu addItemWithTitle:@"Loading model..."
                                      action:nil
                               keyEquivalent:@""];
    sStatusMenuItem.enabled = NO;

    [sMenu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *prefsItem = [sMenu addItemWithTitle:@"Preferences..."
                                             action:@selector(preferencesClicked:)
                                      keyEquivalent:@","];
    prefsItem.target = sDelegate;

    [sMenu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *quitItem = [sMenu addItemWithTitle:@"Quit JoiceTyper"
                                            action:@selector(quitClicked:)
                                     keyEquivalent:@"q"];
    quitItem.target = sDelegate;

    sStatusItem.menu = sMenu;
}

void initStatusBarOnMainThread(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        initStatusBar();
    });
}

void updateStatusBar(int state) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (sStatusItem == nil) return;
        sStatusItem.button.image = createBubbleJIcon(colorForState(state));
        sStatusMenuItem.title = textForState(state);
        sStatusMenuItem.enabled = (state == 1); // only clickable when ready
    });
}
