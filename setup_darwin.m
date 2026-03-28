#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#include "setup_darwin.h"

static NSWindow *sSetupWindow = nil;
static NSTextField *sStep1Status = nil;
static NSPopUpButton *sMicDropdown = nil;
static NSProgressIndicator *sProgressBar = nil;
static NSTextField *sProgressLabel = nil;
static NSTextField *sStep3Status = nil;
static NSTextField *sStep4Status = nil;
static NSButton *sContinueButton = nil;
static BOOL sSetupComplete = NO;
static NSTextField *sStep1Indicator = nil;
static NSTextField *sStep2Indicator = nil;
static NSTextField *sStep3Indicator = nil;
static NSTextField *sStep4Indicator = nil;
static char sSelectedDeviceBuffer[512] = {0};

static NSTextField *makeLabel(NSString *text, CGFloat fontSize, BOOL bold, NSColor *color, NSRect frame) {
    NSTextField *label = [[NSTextField alloc] initWithFrame:frame];
    label.stringValue = text;
    label.font = bold ? [NSFont boldSystemFontOfSize:fontSize] : [NSFont systemFontOfSize:fontSize];
    label.textColor = color;
    label.bezeled = NO;
    label.drawsBackground = NO;
    label.editable = NO;
    label.selectable = NO;
    return label;
}

@interface SetupDelegate : NSObject <NSWindowDelegate>
- (void)continueClicked:(id)sender;
@end

@implementation SetupDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    sSetupComplete = NO;
    [NSApp stop:nil];
    return YES;
}

- (void)continueClicked:(id)sender {
    sSetupComplete = YES;
    [sSetupWindow close];
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
}
@end

static SetupDelegate *sSetupDelegate = nil;

void showSetupWindow(void) {
    @autoreleasepool {
        CGFloat w = 480, h = 460;
        NSRect frame = NSMakeRect(0, 0, w, h);
        sSetupWindow = [[NSWindow alloc]
            initWithContentRect:frame
                      styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable
                        backing:NSBackingStoreBuffered
                          defer:NO];
        [sSetupWindow setTitle:@"JoiceTyper Setup"];
        [sSetupWindow center];

        sSetupDelegate = [[SetupDelegate alloc] init];
        sSetupWindow.delegate = sSetupDelegate;

        NSView *content = sSetupWindow.contentView;
        CGFloat y = h - 50;
        CGFloat pad = 20;
        CGFloat innerW = w - 2 * pad;

        // Title
        NSTextField *title = makeLabel(@"Welcome to JoiceTyper", 18, YES,
            [NSColor labelColor], NSMakeRect(pad, y, innerW, 24));
        title.alignment = NSTextAlignmentCenter;
        [content addSubview:title];
        y -= 22;

        // Subtitle
        NSTextField *subtitle = makeLabel(@"Hold a key, speak, text appears at your cursor.", 12, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad, y, innerW, 18));
        subtitle.alignment = NSTextAlignmentCenter;
        [content addSubview:subtitle];
        y -= 40;

        // Step 1: Accessibility
        sStep1Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep1Indicator];
        NSTextField *s1title = makeLabel(@"1. Accessibility Permission", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s1title];
        y -= 20;
        sStep1Status = makeLabel(@"Checking...", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 16));
        [content addSubview:sStep1Status];
        y -= 36;

        // Step 2: Microphone
        sStep2Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep2Indicator];
        NSTextField *s2title = makeLabel(@"2. Select Microphone", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s2title];
        y -= 28;
        sMicDropdown = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 26) pullsDown:NO];
        [content addSubview:sMicDropdown];
        y -= 36;

        // Step 3: Download
        sStep3Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep3Indicator];
        NSTextField *s3title = makeLabel(@"3. Download Speech Model", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s3title];
        y -= 16;
        sStep3Status = makeLabel(@"whisper-small \u00B7 466 MB", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 16));
        [content addSubview:sStep3Status];
        y -= 18;
        sProgressBar = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 8)];
        sProgressBar.style = NSProgressIndicatorStyleBar;
        sProgressBar.minValue = 0;
        sProgressBar.maxValue = 1.0;
        sProgressBar.doubleValue = 0;
        sProgressBar.indeterminate = NO;
        [content addSubview:sProgressBar];
        y -= 18;
        sProgressLabel = makeLabel(@"", 10, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 14));
        [content addSubview:sProgressLabel];
        y -= 36;

        // Step 4: Ready
        sStep4Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep4Indicator];
        NSTextField *s4title = makeLabel(@"4. Ready", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s4title];
        y -= 20;
        sStep4Status = makeLabel(@"Waiting...", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 16));
        [content addSubview:sStep4Status];

        // Continue button (bottom right, initially disabled)
        sContinueButton = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 120, 16, 120, 32)];
        sContinueButton.title = @"Continue";
        sContinueButton.bezelStyle = NSBezelStyleRounded;
        sContinueButton.enabled = NO;
        sContinueButton.target = sSetupDelegate;
        sContinueButton.action = @selector(continueClicked:);
        [content addSubview:sContinueButton];

        sSetupComplete = NO;

        [sSetupWindow makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
    }
}

void updateSetupAccessibility(int granted) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (granted) {
            sStep1Indicator.stringValue = @"\u2705";
            sStep1Status.stringValue = @"Granted";
            sStep1Status.textColor = [NSColor systemGreenColor];
            sStep2Indicator.stringValue = @"\U0001F3A4";
        } else {
            sStep1Indicator.stringValue = @"\u23F3";
            sStep1Status.stringValue = @"Open System Settings to grant access";
            sStep1Status.textColor = [NSColor systemOrangeColor];
        }
    });
}

void populateSetupDevices(const char **deviceNames, int count, int defaultIndex) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [sMicDropdown removeAllItems];
        for (int i = 0; i < count; i++) {
            [sMicDropdown addItemWithTitle:[NSString stringWithUTF8String:deviceNames[i]]];
        }
        if (defaultIndex >= 0 && defaultIndex < count) {
            [sMicDropdown selectItemAtIndex:defaultIndex];
        }
    });
}

void updateSetupDownloadProgress(double progress, long long bytesDownloaded, long long bytesTotal) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep3Indicator.stringValue = @"\u2B07\uFE0F";
        sProgressBar.doubleValue = progress;
        long long mb_done = bytesDownloaded / (1024 * 1024);
        long long mb_total = bytesTotal / (1024 * 1024);
        sProgressLabel.stringValue = [NSString stringWithFormat:@"%lld MB / %lld MB \u2014 %d%%",
            mb_done, mb_total, (int)(progress * 100)];
    });
}

void updateSetupDownloadComplete(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep3Indicator.stringValue = @"\u2705";
        sProgressBar.doubleValue = 1.0;
        sProgressLabel.stringValue = @"Download complete";
        sStep3Status.stringValue = @"Model ready";
        sStep3Status.textColor = [NSColor systemGreenColor];
    });
}

void updateSetupReady(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep4Indicator.stringValue = @"\u2705";
        sStep4Status.stringValue = @"All set!";
        sStep4Status.textColor = [NSColor systemGreenColor];
        sContinueButton.title = @"Start JoiceTyper";
        sContinueButton.enabled = YES;
    });
}

int isSetupComplete(void) {
    return sSetupComplete ? 1 : 0;
}

const char *getSelectedDevice(void) {
    if (sMicDropdown == nil || sMicDropdown.selectedItem == nil) {
        sSelectedDeviceBuffer[0] = '\0';
        return sSelectedDeviceBuffer;
    }
    NSString *title = sMicDropdown.selectedItem.title;
    const char *utf8 = [title UTF8String];
    if (utf8 == NULL) {
        sSelectedDeviceBuffer[0] = '\0';
        return sSelectedDeviceBuffer;
    }
    size_t len = strlen(utf8);
    if (len >= sizeof(sSelectedDeviceBuffer)) {
        len = sizeof(sSelectedDeviceBuffer) - 1;
    }
    memcpy(sSelectedDeviceBuffer, utf8, len);
    sSelectedDeviceBuffer[len] = '\0';
    return sSelectedDeviceBuffer;
}
