#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#include "settings_darwin.h"
#include "hotkey_darwin.h"

static NSWindow *sSetupWindow = nil;
static NSTextField *sStep1Status = nil;
static NSTextField *sStep2Status = nil;
static NSPopUpButton *sMicDropdown = nil;
static NSPopUpButton *sLangDropdown = nil;
static char sSelectedLangBuffer[8] = {0};
static NSProgressIndicator *sProgressBar = nil;
static NSTextField *sProgressLabel = nil;
static NSTextField *sStep6Status = nil;
static NSTextField *sStep7Status = nil;
static NSButton *sContinueButton = nil;
static BOOL sSetupComplete = NO;
static NSTextField *sStep1Indicator = nil;
static NSTextField *sStep2Indicator = nil;
static NSTextField *sStep3Indicator = nil;
static NSTextField *sStep4Indicator = nil;
static NSTextField *sStep6Indicator = nil;
static NSTextField *sStep7Indicator = nil;
static char sSelectedDeviceBuffer[512] = {0};

static NSTextField *sHotkeyLabel = nil;
static NSButton *sHotkeyChangeBtn = nil;
static NSButton *sHotkeyCancelBtn = nil;
static NSButton *sHotkeyConfirmBtn = nil;
static char sHotkeyBuffer[128] = {0};
static uint64_t sRecordedFlags = 0;
static uint64_t sConfirmedFlags = 0;
static CFMachPortRef sRecorderTap = NULL;

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

static NSString *flagsToDisplayString(uint64_t flags) {
    NSMutableArray *parts = [NSMutableArray array];
    if (flags & 0x800000) [parts addObject:@"Fn"];
    if (flags & 0x20000)  [parts addObject:@"Shift"];
    if (flags & 0x40000)  [parts addObject:@"Ctrl"];
    if (flags & 0x80000)  [parts addObject:@"Option"];
    if (flags & 0x100000) [parts addObject:@"Cmd"];
    if (parts.count == 0) return @"Press keys...";
    return [parts componentsJoinedByString:@" + "];
}

static CGEventRef recorderTapCallback(
    CGEventTapProxy proxy,
    CGEventType type,
    CGEventRef event,
    void *userInfo
) {
    if (type == kCGEventTapDisabledByTimeout) {
        if (sRecorderTap != NULL) CGEventTapEnable(sRecorderTap, true);
        return event;
    }
    if (type != kCGEventFlagsChanged) return event;

    uint64_t flags = CGEventGetFlags(event);
    uint64_t relevant = flags & (0x800000 | 0x20000 | 0x40000 | 0x80000 | 0x100000);
    sRecordedFlags = relevant;

    dispatch_async(dispatch_get_main_queue(), ^{
        sHotkeyLabel.stringValue = flagsToDisplayString(relevant);
    });

    return event;
}

@interface SetupDelegate : NSObject <NSWindowDelegate>
- (void)continueClicked:(id)sender;
- (void)hotkeyChangeClicked:(id)sender;
- (void)hotkeyCancelClicked:(id)sender;
- (void)hotkeyConfirmClicked:(id)sender;
- (void)stopRecorder;
@end

@implementation SetupDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    sSetupComplete = NO;
    [NSApp stopModal];
    return YES;
}

- (void)continueClicked:(id)sender {
    sSetupComplete = YES;
    [sSetupWindow close];
    [NSApp stopModal];
}

- (void)hotkeyChangeClicked:(id)sender {
    sHotkeyChangeBtn.hidden = YES;
    sHotkeyCancelBtn.hidden = NO;
    sHotkeyConfirmBtn.hidden = NO;
    sHotkeyLabel.stringValue = @"Press keys...";
    sRecordedFlags = 0;

    CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged);
    sRecorderTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        recorderTapCallback,
        NULL
    );
    if (sRecorderTap != NULL) {
        CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, sRecorderTap, 0);
        CFRunLoopAddSource(CFRunLoopGetMain(), src, kCFRunLoopCommonModes);
        CGEventTapEnable(sRecorderTap, true);
        CFRelease(src);
    }
}

- (void)hotkeyCancelClicked:(id)sender {
    [self stopRecorder];
    sHotkeyLabel.stringValue = [NSString stringWithUTF8String:sHotkeyBuffer];
}

- (void)hotkeyConfirmClicked:(id)sender {
    [self stopRecorder];
    if (sRecordedFlags == 0) {
        sHotkeyLabel.stringValue = [NSString stringWithUTF8String:sHotkeyBuffer];
        return;
    }
    sConfirmedFlags = sRecordedFlags;
    NSString *display = flagsToDisplayString(sConfirmedFlags);
    strlcpy(sHotkeyBuffer, [display UTF8String], sizeof(sHotkeyBuffer));
    sHotkeyLabel.stringValue = display;
}

- (void)stopRecorder {
    sHotkeyChangeBtn.hidden = NO;
    sHotkeyCancelBtn.hidden = YES;
    sHotkeyConfirmBtn.hidden = YES;
    if (sRecorderTap != NULL) {
        CGEventTapEnable(sRecorderTap, false);
        CFRelease(sRecorderTap);
        sRecorderTap = NULL;
    }
}
@end

static SetupDelegate *sSetupDelegate = nil;
static BOOL sIsOnboarding = YES;

void showSettingsWindow(int onboarding) {
    @autoreleasepool {
        sIsOnboarding = (onboarding != 0);
        CGFloat w = 480, h = 640;
        NSRect frame = NSMakeRect(0, 0, w, h);
        sSetupWindow = [[NSWindow alloc]
            initWithContentRect:frame
                      styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable
                        backing:NSBackingStoreBuffered
                          defer:NO];
        [sSetupWindow setTitle:sIsOnboarding ? @"JoiceTyper Setup" : @"JoiceTyper Preferences"];
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

        // Step 2: Input Monitoring
        sStep2Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep2Indicator];
        NSTextField *s2title = makeLabel(@"2. Input Monitoring Permission", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s2title];
        y -= 20;
        sStep2Status = makeLabel(@"Checking...", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 16));
        [content addSubview:sStep2Status];
        y -= 36;

        // Step 3: Microphone
        sStep3Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep3Indicator];
        NSTextField *s3title = makeLabel(@"3. Select Microphone", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s3title];
        y -= 28;
        sMicDropdown = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 26) pullsDown:NO];
        [content addSubview:sMicDropdown];
        y -= 36;

        // Step 4: Language
        sStep4Indicator = makeLabel(@"\U0001F310", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep4Indicator];
        NSTextField *s4title = makeLabel(@"4. Language", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s4title];
        y -= 28;
        sLangDropdown = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 26) pullsDown:NO];
        [content addSubview:sLangDropdown];
        y -= 36;

        // Step 5: Hotkey
        NSTextField *s5HkIndicator = makeLabel(@"\u2328\uFE0F", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:s5HkIndicator];
        NSTextField *s5title = makeLabel(@"5. Hotkey", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s5title];
        y -= 28;

        sHotkeyLabel = makeLabel(@"Fn + Shift", 12, NO,
            [NSColor labelColor], NSMakeRect(pad + 28, y, 200, 20));
        [content addSubview:sHotkeyLabel];

        sHotkeyChangeBtn = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 80, y, 80, 24)];
        sHotkeyChangeBtn.title = @"Change";
        sHotkeyChangeBtn.bezelStyle = NSBezelStyleRounded;
        sHotkeyChangeBtn.target = sSetupDelegate;
        sHotkeyChangeBtn.action = @selector(hotkeyChangeClicked:);
        [content addSubview:sHotkeyChangeBtn];

        sHotkeyCancelBtn = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 170, y, 80, 24)];
        sHotkeyCancelBtn.title = @"Cancel";
        sHotkeyCancelBtn.bezelStyle = NSBezelStyleRounded;
        sHotkeyCancelBtn.target = sSetupDelegate;
        sHotkeyCancelBtn.action = @selector(hotkeyCancelClicked:);
        sHotkeyCancelBtn.hidden = YES;
        [content addSubview:sHotkeyCancelBtn];

        sHotkeyConfirmBtn = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 80, y, 80, 24)];
        sHotkeyConfirmBtn.title = @"Confirm";
        sHotkeyConfirmBtn.bezelStyle = NSBezelStyleRounded;
        sHotkeyConfirmBtn.target = sSetupDelegate;
        sHotkeyConfirmBtn.action = @selector(hotkeyConfirmClicked:);
        sHotkeyConfirmBtn.hidden = YES;
        [content addSubview:sHotkeyConfirmBtn];

        y -= 36;

        // Step 6: Download
        sStep6Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep6Indicator];
        NSTextField *s6title = makeLabel(@"6. Download Speech Model", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s6title];
        y -= 16;
        sStep6Status = makeLabel(@"whisper-small \u00B7 466 MB", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 16));
        [content addSubview:sStep6Status];
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

        // Step 7: Ready
        sStep7Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep7Indicator];
        NSTextField *s7title = makeLabel(@"7. Ready", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s7title];
        y -= 20;
        sStep7Status = makeLabel(@"Waiting...", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 16));
        [content addSubview:sStep7Status];

        // Continue button (bottom right, initially disabled)
        sContinueButton = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 120, 16, 120, 32)];
        sContinueButton.title = sIsOnboarding ? @"Continue" : @"Save";
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
        } else {
            sStep1Indicator.stringValue = @"\u23F3";
            sStep1Status.stringValue = @"System Settings \u2192 Privacy & Security \u2192 Accessibility";
            sStep1Status.textColor = [NSColor systemOrangeColor];
        }
    });
}

void updateSetupInputMonitoring(int granted) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (granted) {
            sStep2Indicator.stringValue = @"\u2705";
            sStep2Status.stringValue = @"Granted";
            sStep2Status.textColor = [NSColor systemGreenColor];
            sStep3Indicator.stringValue = @"\U0001F3A4";
        } else {
            sStep2Indicator.stringValue = @"\u23F3";
            sStep2Status.stringValue = @"System Settings \u2192 Privacy & Security \u2192 Input Monitoring";
            sStep2Status.textColor = [NSColor systemOrangeColor];
        }
    });
}

void populateSetupDevices(const char **deviceNames, int count, int defaultIndex) {
    // Called from main thread — no dispatch needed. Must be synchronous
    // because the caller frees the C strings immediately after this returns.
    [sMicDropdown removeAllItems];
    for (int i = 0; i < count; i++) {
        [sMicDropdown addItemWithTitle:[NSString stringWithUTF8String:deviceNames[i]]];
    }
    if (defaultIndex >= 0 && defaultIndex < count) {
        [sMicDropdown selectItemAtIndex:defaultIndex];
    }
}

void updateSetupDownloadProgress(double progress, long long bytesDownloaded, long long bytesTotal) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep6Indicator.stringValue = @"\u2B07\uFE0F";
        sProgressBar.doubleValue = progress;
        long long mb_done = bytesDownloaded / (1024 * 1024);
        long long mb_total = bytesTotal / (1024 * 1024);
        sProgressLabel.stringValue = [NSString stringWithFormat:@"%lld MB / %lld MB \u2014 %d%%",
            mb_done, mb_total, (int)(progress * 100)];
    });
}

void updateSetupDownloadComplete(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep6Indicator.stringValue = @"\u2705";
        sProgressBar.doubleValue = 1.0;
        sProgressLabel.stringValue = @"Download complete";
        sStep6Status.stringValue = @"Model ready";
        sStep6Status.textColor = [NSColor systemGreenColor];
    });
}

void updateSetupDownloadFailed(const char *errorMsg) {
    NSString *msg = [NSString stringWithUTF8String:errorMsg];
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep6Indicator.stringValue = @"\u274C";
        sProgressBar.doubleValue = 0;
        sProgressLabel.stringValue = msg;
        sStep6Status.stringValue = @"Download failed \u2014 restart to retry";
        sStep6Status.textColor = [NSColor systemRedColor];
    });
}

void updateSetupReady(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep7Indicator.stringValue = @"\u2705";
        sStep7Status.stringValue = @"All set!";
        sStep7Status.textColor = [NSColor systemGreenColor];
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

void populateSettingsLanguages(const char **codes, const char **names, int count, int defaultIndex) {
    [sLangDropdown removeAllItems];
    for (int i = 0; i < count; i++) {
        NSString *title = [NSString stringWithUTF8String:names[i]];
        [sLangDropdown addItemWithTitle:title];
        [sLangDropdown lastItem].representedObject = [NSString stringWithUTF8String:codes[i]];
    }
    if (defaultIndex >= 0 && defaultIndex < count) {
        [sLangDropdown selectItemAtIndex:defaultIndex];
    }
}

const char *getSelectedLanguage(void) {
    if (sLangDropdown == nil || sLangDropdown.selectedItem == nil) {
        sSelectedLangBuffer[0] = '\0';
        return sSelectedLangBuffer;
    }
    NSString *code = sLangDropdown.selectedItem.representedObject;
    const char *utf8 = [code UTF8String];
    if (utf8 == NULL) {
        sSelectedLangBuffer[0] = '\0';
        return sSelectedLangBuffer;
    }
    size_t len = strlen(utf8);
    if (len >= sizeof(sSelectedLangBuffer)) {
        len = sizeof(sSelectedLangBuffer) - 1;
    }
    memcpy(sSelectedLangBuffer, utf8, len);
    sSelectedLangBuffer[len] = '\0';
    return sSelectedLangBuffer;
}

void setSettingsHotkey(const char *displayText) {
    strlcpy(sHotkeyBuffer, displayText, sizeof(sHotkeyBuffer));
    dispatch_async(dispatch_get_main_queue(), ^{
        if (sHotkeyLabel != nil) {
            sHotkeyLabel.stringValue = [NSString stringWithUTF8String:sHotkeyBuffer];
        }
    });
}

const char *getSettingsHotkey(void) {
    return sHotkeyBuffer;
}

uint64_t getSettingsHotkeyFlags(void) {
    return sConfirmedFlags;
}

void runSetupEventLoop(void) {
    // Ensure NSApplication singleton exists (idempotent, safe to call many times).
    ensureNSApp();
    // Run as modal session — does NOT call [NSApp run].
    // The single [NSApp run] happens later in hotkey's runMainLoop().
    [NSApp runModalForWindow:sSetupWindow];
}
