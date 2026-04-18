#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#include <stdatomic.h>
#include "settings_darwin.h"
#include "hotkey_darwin.h"

static NSWindow *sSetupWindow = nil;
static NSTextField *sStep1Status = nil;
static NSTextField *sStep2Status = nil;
static NSPopUpButton *sMicDropdown = nil;
static NSPopUpButton *sLangDropdown = nil;
static char sSelectedLangBuffer[8] = {0};
static NSPopUpButton *sDecodeDropdown = nil;
static char sSelectedDecodeBuffer[16] = {0};
static NSPopUpButton *sPunctuationDropdown = nil;
static char sSelectedPunctuationBuffer[32] = {0};
static NSPopUpButton *sModelDropdown = nil;
static NSButton *sModelBtn1 = nil;      // "In Use" / "Use" / "Download"
static NSButton *sModelBtn2 = nil;      // "Delete" / "Confirm?" / "Cancel"
static NSProgressIndicator *sProgressBar = nil;
static NSTextField *sProgressLabel = nil;
static char sSelectedModelBuffer[32] = {0};
static char sActiveModelSize[32] = {0};
static NSTextField *sStep7Status = nil;
static NSScrollView *sVocabScrollView = nil;
static NSTextView *sVocabTextView = nil;
static NSButton *sContinueButton = nil;
static NSButton *sSaveButton = nil;
static BOOL sSetupComplete = NO;
static NSTextField *sStep1Indicator = nil;
static NSTextField *sStep2Indicator = nil;
static NSTextField *sStep3Indicator = nil;
static NSTextField *sStep4Indicator = nil;
static NSTextField *sStep6Indicator = nil;
static char sSelectedDeviceBuffer[512] = {0};

static BOOL sAccessibilityGranted = NO;
static BOOL sInputMonitoringGranted = NO;
static BOOL sIsOnboarding = YES;
static dispatch_semaphore_t sWindowDone = NULL;

static NSTextField *sHotkeyLabel = nil;
static NSButton *sHotkeyChangeBtn = nil;
static NSButton *sHotkeyCancelBtn = nil;
static NSButton *sHotkeyConfirmBtn = nil;
static char sHotkeyBuffer[128] = {0};
static _Atomic uint64_t sRecordedFlags = 0;
static uint64_t sConfirmedFlags = 0;
static int sRecordedKeycode = -1;
static int sConfirmedKeycode = -1;
static CFMachPortRef sRecorderTap = NULL;
static CFRunLoopSourceRef sRecorderSource = NULL;
static id sRecorderLocalMonitor = nil;
static id sRecorderFlagsMonitor = nil;

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

static NSString *keycodeToDisplayName(int keycode) {
    switch (keycode) {
        case 0x00: return @"A"; case 0x01: return @"S"; case 0x02: return @"D";
        case 0x03: return @"F"; case 0x04: return @"H"; case 0x05: return @"G";
        case 0x06: return @"Z"; case 0x07: return @"X"; case 0x08: return @"C";
        case 0x09: return @"V"; case 0x0B: return @"B"; case 0x0C: return @"Q";
        case 0x0D: return @"W"; case 0x0E: return @"E"; case 0x0F: return @"R";
        case 0x10: return @"Y"; case 0x11: return @"T"; case 0x12: return @"1";
        case 0x13: return @"2"; case 0x14: return @"3"; case 0x15: return @"4";
        case 0x16: return @"6"; case 0x17: return @"5"; case 0x1A: return @"7";
        case 0x1C: return @"8"; case 0x19: return @"9"; case 0x1D: return @"0";
        case 0x1F: return @"O"; case 0x20: return @"U"; case 0x22: return @"I";
        case 0x23: return @"P"; case 0x25: return @"L"; case 0x26: return @"J";
        case 0x28: return @"K"; case 0x2D: return @"N"; case 0x2E: return @"M";
        case 0x31: return @"Space"; case 0x30: return @"Tab"; case 0x24: return @"Return";
        case 0x35: return @"Escape"; case 0x33: return @"Delete";
        case 0x7A: return @"F1"; case 0x78: return @"F2"; case 0x63: return @"F3";
        case 0x76: return @"F4"; case 0x60: return @"F5"; case 0x61: return @"F6";
        case 0x62: return @"F7"; case 0x64: return @"F8"; case 0x65: return @"F9";
        case 0x6D: return @"F10"; case 0x67: return @"F11"; case 0x6F: return @"F12";
        default: return [NSString stringWithFormat:@"Key(%d)", keycode];
    }
}

static NSString *buildHotkeyDisplayString(uint64_t flags, int keycode) {
    NSMutableArray *parts = [NSMutableArray array];
    if (flags & 0x800000) [parts addObject:@"Fn"];
    if (flags & 0x40000)  [parts addObject:@"Ctrl"];
    if (flags & 0x80000)  [parts addObject:@"Option"];
    if (flags & 0x20000)  [parts addObject:@"Shift"];
    if (flags & 0x100000) [parts addObject:@"Cmd"];
    if (keycode >= 0) [parts addObject:keycodeToDisplayName(keycode)];
    if (parts.count == 0) return @"Press keys...";
    return [parts componentsJoinedByString:@" + "];
}

static void refreshContinueState(void) {
    BOOL allPerms = sAccessibilityGranted && sInputMonitoringGranted;
    if (allPerms) {
        sStep7Status.stringValue = @"Ready";
        sStep7Status.textColor = [NSColor systemGreenColor];
    } else {
        sStep7Status.stringValue = @"Not Ready";
        sStep7Status.textColor = [NSColor systemOrangeColor];
    }
    if (sIsOnboarding) {
        sContinueButton.enabled = allPerms;
    }
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

    if (type == kCGEventFlagsChanged) {
        uint64_t flags = CGEventGetFlags(event);
        uint64_t relevant = flags & (0x800000 | 0x20000 | 0x40000 | 0x80000 | 0x100000);
        sRecordedFlags = relevant;
        // If no key pressed yet, show modifiers only
        if (sRecordedKeycode < 0) {
            dispatch_async(dispatch_get_main_queue(), ^{
                sHotkeyLabel.stringValue = buildHotkeyDisplayString(relevant, -1);
            });
        }
    } else if (type == kCGEventKeyDown) {
        int keycode = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
        sRecordedKeycode = keycode;
        uint64_t flags = sRecordedFlags; // current modifier state
        dispatch_async(dispatch_get_main_queue(), ^{
            sHotkeyLabel.stringValue = buildHotkeyDisplayString(flags, keycode);
        });
    }
    // Ignore kCGEventKeyUp — we want to capture the key, not track release

    return event;
}

extern void modelBtn1Clicked(void);
extern void modelBtn2Clicked(void);
extern void modelDropdownChanged(void);

@interface SetupDelegate : NSObject <NSWindowDelegate>
- (void)continueClicked:(id)sender;
- (void)hotkeyChangeClicked:(id)sender;
- (void)hotkeyCancelClicked:(id)sender;
- (void)hotkeyConfirmClicked:(id)sender;
- (void)openAccessibilitySettings:(id)sender;
- (void)openInputMonitoringSettings:(id)sender;
- (void)modelBtn1Clicked:(id)sender;
- (void)modelBtn2Clicked:(id)sender;
- (void)modelSelectionChanged:(id)sender;
- (void)stopRecorder;
@end

@implementation SetupDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    [self stopRecorder];
    // In preferences mode, closing via red X saves (auto-save on close).
    // In onboarding mode, closing via red X cancels (user must click Start).
    sSetupComplete = !sIsOnboarding;
    [sSetupWindow orderOut:nil];
    if (sWindowDone) {
        dispatch_semaphore_signal(sWindowDone);
    } else {
        [NSApp stopModal];
    }
    return NO;
}

- (void)continueClicked:(id)sender {
    [self stopRecorder];
    sSetupComplete = YES;
    [sSetupWindow orderOut:nil];
    if (sWindowDone) {
        dispatch_semaphore_signal(sWindowDone);
    } else {
        [NSApp stopModal];
    }
}

- (void)hotkeyChangeClicked:(id)sender {
    sHotkeyChangeBtn.hidden = YES;
    sHotkeyCancelBtn.hidden = NO;
    sHotkeyConfirmBtn.hidden = NO;
    sHotkeyLabel.stringValue = @"Press keys...";
    sRecordedFlags = 0;
    sRecordedKeycode = -1;

    // Use NSEvent local monitors instead of CGEvent tap.
    // CGEvent taps don't receive key events reliably inside a modal window.
    // Local monitors work within the app's own event loop including modals.

    // Monitor modifier flag changes
    sRecorderFlagsMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
        handler:^NSEvent *(NSEvent *event) {
            uint64_t flags = [event modifierFlags];
            uint64_t relevant = flags & (NSEventModifierFlagFunction | NSEventModifierFlagShift |
                                          NSEventModifierFlagControl | NSEventModifierFlagOption |
                                          NSEventModifierFlagCommand);
            // Convert to CGEvent flag format for consistency
            uint64_t cgFlags = 0;
            if (relevant & NSEventModifierFlagFunction) cgFlags |= 0x800000;
            if (relevant & NSEventModifierFlagShift)    cgFlags |= 0x20000;
            if (relevant & NSEventModifierFlagControl)  cgFlags |= 0x40000;
            if (relevant & NSEventModifierFlagOption)    cgFlags |= 0x80000;
            if (relevant & NSEventModifierFlagCommand)  cgFlags |= 0x100000;
            sRecordedFlags = cgFlags;
            if (sRecordedKeycode < 0) {
                sHotkeyLabel.stringValue = buildHotkeyDisplayString(cgFlags, -1);
            }
            return event;
        }];

    // Monitor key-down events
    sRecorderLocalMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
        handler:^NSEvent *(NSEvent *event) {
            sRecordedKeycode = (int)[event keyCode];
            uint64_t flags = sRecordedFlags;
            int kc = sRecordedKeycode;
            sHotkeyLabel.stringValue = buildHotkeyDisplayString(flags, kc);
            return nil; // consume the event so it doesn't beep
        }];
}

- (void)hotkeyCancelClicked:(id)sender {
    [self stopRecorder];
    sHotkeyLabel.stringValue = [NSString stringWithUTF8String:sHotkeyBuffer];
}

- (void)hotkeyConfirmClicked:(id)sender {
    [self stopRecorder];
    if (sRecordedFlags == 0 && sRecordedKeycode < 0) {
        sHotkeyLabel.stringValue = [NSString stringWithUTF8String:sHotkeyBuffer];
        return;
    }
    sConfirmedFlags = sRecordedFlags;
    sConfirmedKeycode = sRecordedKeycode;
    NSString *display = buildHotkeyDisplayString(sConfirmedFlags, sConfirmedKeycode);
    strlcpy(sHotkeyBuffer, [display UTF8String], sizeof(sHotkeyBuffer));
    sHotkeyLabel.stringValue = display;
    // No auto-save — settings auto-save when the window is closed.
}

- (void)openAccessibilitySettings:(id)sender {
    [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"]];
}

- (void)openInputMonitoringSettings:(id)sender {
    [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent"]];
}

void openAccessibilitySettingsFromGo(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"]];
    });
}

void openInputMonitoringSettingsFromGo(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent"]];
    });
}

- (void)modelBtn1Clicked:(id)sender {
    modelBtn1Clicked();
}

- (void)modelBtn2Clicked:(id)sender {
    modelBtn2Clicked();
}

- (void)modelSelectionChanged:(id)sender {
    modelDropdownChanged();
}

- (void)stopRecorder {
    sHotkeyChangeBtn.hidden = NO;
    sHotkeyCancelBtn.hidden = YES;
    sHotkeyConfirmBtn.hidden = YES;
    if (sRecorderLocalMonitor != nil) {
        [NSEvent removeMonitor:sRecorderLocalMonitor];
        sRecorderLocalMonitor = nil;
    }
    if (sRecorderFlagsMonitor != nil) {
        [NSEvent removeMonitor:sRecorderFlagsMonitor];
        sRecorderFlagsMonitor = nil;
    }
}
@end

static SetupDelegate *sSetupDelegate = nil;

void showSettingsWindow(int onboarding) {
    @autoreleasepool {
        // Reset stale hotkey recorder state
        sConfirmedFlags = 0;
        sConfirmedKeycode = -1;
        sRecordedFlags = 0;
        sRecordedKeycode = -1;

        // Reset permission tracking for onboarding gate
        sAccessibilityGranted = NO;
        sInputMonitoringGranted = NO;

        sIsOnboarding = (onboarding != 0);
        sSetupComplete = NO;

        // Reuse existing window if available — never close/recreate.
        // The window is hidden (orderOut) on dismiss, not released.
        if (sSetupWindow != nil) {
            [sSetupWindow setTitle:sIsOnboarding ? @"JoiceTyper Setup" : @"JoiceTyper Preferences"];
            sContinueButton.title = @"Start JoiceTyper";
            // In prefs mode: no Save button — auto-saves on close (red X).
            // In onboarding mode: button visible, enabled when permissions granted.
            sContinueButton.hidden = !sIsOnboarding;
            sSaveButton.hidden = sIsOnboarding;
            sContinueButton.enabled = NO;

            // Reset step indicators to initial state
            sStep1Indicator.stringValue = @"\u23F3";
            sStep1Status.stringValue = @"Checking...";
            sStep1Status.textColor = [NSColor secondaryLabelColor];
            sStep2Indicator.stringValue = @"\u23F3";
            sStep2Status.stringValue = @"Checking...";
            sStep2Status.textColor = [NSColor secondaryLabelColor];
            sStep6Indicator.stringValue = @"\u23F3";
            sProgressBar.hidden = YES;
            sProgressBar.doubleValue = 0;
            sProgressLabel.hidden = YES;
            sProgressLabel.stringValue = @"";

            sStep7Status.stringValue = @"Not Ready";
            sStep7Status.textColor = [NSColor systemOrangeColor];

            // Reset hotkey display
            sHotkeyChangeBtn.hidden = NO;
            sHotkeyCancelBtn.hidden = YES;
            sHotkeyConfirmBtn.hidden = YES;

            [sSetupWindow makeKeyAndOrderFront:nil];
            [NSApp activateIgnoringOtherApps:YES];
            return;
        }

        // First-time window creation
        CGFloat w = 480, h = 1060;
        NSRect frame = NSMakeRect(0, 0, w, h);
        sSetupWindow = [[NSWindow alloc]
            initWithContentRect:frame
                      styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable
                        backing:NSBackingStoreBuffered
                          defer:NO];
        [sSetupWindow setReleasedWhenClosed:NO];
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
        NSTextField *subtitle = makeLabel(@"You Speak, Joice Types", 12, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad, y, innerW, 18));
        subtitle.alignment = NSTextAlignmentCenter;
        [content addSubview:subtitle];
        y -= 24;

        // Ready status (below tagline, bigger font)
        sStep7Status = makeLabel(@"Not Ready", 14, YES,
            [NSColor systemOrangeColor], NSMakeRect(pad, y, innerW, 20));
        sStep7Status.alignment = NSTextAlignmentCenter;
        [content addSubview:sStep7Status];
        y -= 30;

        // Step 1: Accessibility
        sStep1Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep1Indicator];
        NSTextField *s1title = makeLabel(@"1. Accessibility Permission", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s1title];
        y -= 20;
        sStep1Status = makeLabel(@"Checking...", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 90, 16));
        [content addSubview:sStep1Status];

        NSButton *s1OpenBtn = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 60, y + 16, 60, 20)];
        s1OpenBtn.title = @"Open";
        s1OpenBtn.bezelStyle = NSBezelStyleRounded;
        s1OpenBtn.controlSize = NSControlSizeSmall;
        s1OpenBtn.font = [NSFont systemFontOfSize:10];
        s1OpenBtn.target = sSetupDelegate;
        s1OpenBtn.action = @selector(openAccessibilitySettings:);
        [content addSubview:s1OpenBtn];

        NSTextField *s1Help = makeLabel(@"If toggled on but showing denied: remove entry (\u2212) and re-add it", 11, NO,
            [NSColor tertiaryLabelColor], NSMakeRect(pad + 28, y - 14, innerW - 90, 16));
        [content addSubview:s1Help];
        y -= 50;

        // Step 2: Input Monitoring
        sStep2Indicator = makeLabel(@"\u23F3", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep2Indicator];
        NSTextField *s2title = makeLabel(@"2. Input Monitoring Permission", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s2title];
        y -= 20;
        sStep2Status = makeLabel(@"Checking...", 11, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 90, 16));
        [content addSubview:sStep2Status];

        NSButton *s2OpenBtn = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 60, y + 16, 60, 20)];
        s2OpenBtn.title = @"Open";
        s2OpenBtn.bezelStyle = NSBezelStyleRounded;
        s2OpenBtn.controlSize = NSControlSizeSmall;
        s2OpenBtn.font = [NSFont systemFontOfSize:10];
        s2OpenBtn.target = sSetupDelegate;
        s2OpenBtn.action = @selector(openInputMonitoringSettings:);
        [content addSubview:s2OpenBtn];

        NSTextField *s2Help = makeLabel(@"If toggled on but showing denied: remove entry (\u2212) and re-add it", 11, NO,
            [NSColor tertiaryLabelColor], NSMakeRect(pad + 28, y - 14, innerW - 90, 16));
        [content addSubview:s2Help];
        y -= 50;

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

        // Step 5: Decode mode
        NSTextField *s5DecodeIndicator = makeLabel(@"\u2699\uFE0F", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:s5DecodeIndicator];
        NSTextField *s5title = makeLabel(@"5. Decode Quality", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s5title];
        y -= 28;
        sDecodeDropdown = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 26) pullsDown:NO];
        [content addSubview:sDecodeDropdown];
        y -= 36;

        // Step 6: Punctuation mode
        NSTextField *s6PunctuationIndicator = makeLabel(@"\u270D\uFE0F", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:s6PunctuationIndicator];
        NSTextField *s6title = makeLabel(@"6. Punctuation", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s6title];
        y -= 28;
        sPunctuationDropdown = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 26) pullsDown:NO];
        [content addSubview:sPunctuationDropdown];
        y -= 36;

        // Step 7: Hotkey
        NSTextField *s7HkIndicator = makeLabel(@"\u2328\uFE0F", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:s7HkIndicator];
        NSTextField *s7title = makeLabel(@"7. Hotkey", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s7title];
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

        // Step 8: Speech Model
        sStep6Indicator = makeLabel(@"\U0001F9E0", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep6Indicator];
        NSTextField *s8title = makeLabel(@"8. Speech Model", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s8title];
        y -= 28;
        sModelDropdown = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 26) pullsDown:NO];
        sModelDropdown.target = sSetupDelegate;
        sModelDropdown.action = @selector(modelSelectionChanged:);
        [content addSubview:sModelDropdown];
        y -= 28;

        // Buttons row: btn1 (left) and btn2 (right of btn1)
        sModelBtn1 = [[NSButton alloc] initWithFrame:NSMakeRect(pad + 28, y, 90, 24)];
        sModelBtn1.bezelStyle = NSBezelStyleRounded;
        sModelBtn1.target = sSetupDelegate;
        sModelBtn1.action = @selector(modelBtn1Clicked:);
        [content addSubview:sModelBtn1];

        sModelBtn2 = [[NSButton alloc] initWithFrame:NSMakeRect(pad + 28 + 94, y, 80, 24)];
        sModelBtn2.bezelStyle = NSBezelStyleRounded;
        sModelBtn2.target = sSetupDelegate;
        sModelBtn2.action = @selector(modelBtn2Clicked:);
        sModelBtn2.hidden = YES;
        [content addSubview:sModelBtn2];
        y -= 28;

        // Progress bar for downloads
        sProgressBar = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(pad + 28, y + 4, innerW - 28, 8)];
        sProgressBar.style = NSProgressIndicatorStyleBar;
        sProgressBar.minValue = 0;
        sProgressBar.maxValue = 1.0;
        sProgressBar.indeterminate = NO;
        sProgressBar.hidden = YES;
        [content addSubview:sProgressBar];
        y -= 14;
        sProgressLabel = makeLabel(@"", 10, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 14));
        sProgressLabel.hidden = YES;
        [content addSubview:sProgressLabel];
        y -= 28;

        // Step 9: Words You Use Often
        NSTextField *s9VocabIndicator = makeLabel(@"\U0001F4AC", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:s9VocabIndicator];
        NSTextField *s9title = makeLabel(@"9. Words You Use Often", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s9title];
        y -= 22;

        // Description (above text box)
        NSTextField *vocabHelp = makeLabel(@"Separate words or phrases with commas. These help the speech model recognise your terminology.", 11, NO,
            [NSColor tertiaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 28));
        vocabHelp.maximumNumberOfLines = 2;
        [content addSubview:vocabHelp];
        y -= 32;

        // Text box (scrollable)
        sVocabScrollView = [[NSScrollView alloc] initWithFrame:NSMakeRect(pad + 28, y - 120, innerW - 28, 120)];
        sVocabScrollView.hasVerticalScroller = YES;
        sVocabScrollView.borderType = NSBezelBorder;
        sVocabTextView = [[NSTextView alloc] initWithFrame:NSMakeRect(0, 0, innerW - 28 - 16, 120)];
        sVocabTextView.font = [NSFont systemFontOfSize:12];
        sVocabTextView.textColor = [NSColor labelColor];
        sVocabTextView.backgroundColor = [NSColor textBackgroundColor];
        sVocabTextView.minSize = NSMakeSize(0, 120);
        sVocabTextView.maxSize = NSMakeSize(CGFLOAT_MAX, CGFLOAT_MAX);
        sVocabTextView.verticallyResizable = YES;
        sVocabTextView.horizontallyResizable = NO;
        sVocabTextView.textContainer.widthTracksTextView = YES;
        sVocabScrollView.documentView = sVocabTextView;
        [content addSubview:sVocabScrollView];
        y -= (120 + 8);

        // Save button (preferences mode only — right-aligned, below vocabulary)
        sSaveButton = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 80, y - 28, 80, 28)];
        sSaveButton.title = @"Save";
        sSaveButton.bezelStyle = NSBezelStyleRounded;
        sSaveButton.target = sSetupDelegate;
        sSaveButton.action = @selector(continueClicked:);
        sSaveButton.hidden = sIsOnboarding;
        [content addSubview:sSaveButton];
        y -= 36;

        // Continue button (bottom right, initially disabled)
        sContinueButton = [[NSButton alloc] initWithFrame:NSMakeRect(w - pad - 120, 16, 120, 32)];
        sContinueButton.title = @"Start JoiceTyper";
        sContinueButton.bezelStyle = NSBezelStyleRounded;
        sContinueButton.hidden = !sIsOnboarding; // prefs: no button, auto-saves on close
        sContinueButton.enabled = NO; // onboarding: enabled when permissions granted
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
            sAccessibilityGranted = YES;
            sStep1Indicator.stringValue = @"\u2705";
            sStep1Status.stringValue = @"Granted";
            sStep1Status.textColor = [NSColor systemGreenColor];
        } else {
            sAccessibilityGranted = NO;
            sStep1Indicator.stringValue = @"\u23F3";
            sStep1Status.stringValue = @"System Settings \u2192 Privacy & Security \u2192 Accessibility";
            sStep1Status.textColor = [NSColor systemOrangeColor];
        }
        refreshContinueState();
    });
}

void updateSetupInputMonitoring(int granted) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (granted) {
            sInputMonitoringGranted = YES;
            sStep2Indicator.stringValue = @"\u2705";
            sStep2Status.stringValue = @"Granted";
            sStep2Status.textColor = [NSColor systemGreenColor];
            sStep3Indicator.stringValue = @"\U0001F3A4";
        } else {
            sInputMonitoringGranted = NO;
            sStep2Indicator.stringValue = @"\u23F3";
            sStep2Status.stringValue = @"System Settings \u2192 Privacy & Security \u2192 Input Monitoring";
            sStep2Status.textColor = [NSColor systemOrangeColor];
        }
        refreshContinueState();
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

void updateSetupDownloadComplete(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep6Indicator.stringValue = @"\u2705";
    });
}

void setPrefsPermissionState(void) {
    // Check ACTUAL permission state — don't assume granted just because
    // the app is running. Preferences is reachable during StateNoPermission.
    int accGranted = checkAccessibility();
    int inpGranted = checkInputMonitoring();

    dispatch_async(dispatch_get_main_queue(), ^{
        // Accessibility
        sAccessibilityGranted = (accGranted != 0);
        if (accGranted) {
            sStep1Indicator.stringValue = @"\u2705";
            sStep1Status.stringValue = @"Granted";
            sStep1Status.textColor = [NSColor systemGreenColor];
        } else {
            sStep1Indicator.stringValue = @"\u26A0\uFE0F";
            sStep1Status.stringValue = @"Not granted \u2014 System Settings \u2192 Privacy & Security \u2192 Accessibility";
            sStep1Status.textColor = [NSColor systemOrangeColor];
        }

        // Input Monitoring (probed via event tap)
        sInputMonitoringGranted = (inpGranted != 0);
        if (inpGranted) {
            sStep2Indicator.stringValue = @"\u2705";
            sStep2Status.stringValue = @"Granted";
            sStep2Status.textColor = [NSColor systemGreenColor];
        } else {
            sStep2Indicator.stringValue = @"\u26A0\uFE0F";
            sStep2Status.stringValue = @"Not granted \u2014 System Settings \u2192 Privacy & Security \u2192 Input Monitoring";
            sStep2Status.textColor = [NSColor systemOrangeColor];
        }

        sStep3Indicator.stringValue = @"\U0001F3A4";

        // Model step: current configuration is already using a resolved model.
        sStep6Indicator.stringValue = @"\u2705";

        refreshContinueState();
    });
}

void updateSetupReady(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sContinueButton.title = @"Start JoiceTyper";
        refreshContinueState();
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

void setVocabularyText(const char *text) {
    if (sVocabTextView == nil) return;
    // Copy string before dispatch — caller may free immediately.
    NSString *str = [NSString stringWithUTF8String:text];
    dispatch_async(dispatch_get_main_queue(), ^{
        sVocabTextView.string = str;
    });
}

static char sVocabBuffer[16384] = {0};

const char *getVocabularyText(void) {
    if (sVocabTextView == nil) {
        sVocabBuffer[0] = '\0';
        return sVocabBuffer;
    }
    // Read NSTextView on main thread — required for AppKit thread safety.
    __block NSString *text = nil;
    if ([NSThread isMainThread]) {
        text = [sVocabTextView.string copy];
    } else {
        dispatch_sync(dispatch_get_main_queue(), ^{
            text = [sVocabTextView.string copy];
        });
    }
    const char *utf8 = [text UTF8String];
    if (utf8 == NULL) {
        sVocabBuffer[0] = '\0';
        return sVocabBuffer;
    }
    size_t len = strlen(utf8);
    if (len >= sizeof(sVocabBuffer)) {
        len = sizeof(sVocabBuffer) - 1;
        NSLog(@"JoiceTyper: vocabulary truncated to %zu bytes", sizeof(sVocabBuffer) - 1);
    }
    memcpy(sVocabBuffer, utf8, len);
    sVocabBuffer[len] = '\0';
    return sVocabBuffer;
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

void populateSettingsDecodeModes(const char **codes, const char **names, int count, int defaultIndex) {
    [sDecodeDropdown removeAllItems];
    for (int i = 0; i < count; i++) {
        NSString *title = [NSString stringWithUTF8String:names[i]];
        [sDecodeDropdown addItemWithTitle:title];
        [sDecodeDropdown lastItem].representedObject = [NSString stringWithUTF8String:codes[i]];
    }
    if (defaultIndex >= 0 && defaultIndex < count) {
        [sDecodeDropdown selectItemAtIndex:defaultIndex];
    }
}

const char *getSelectedDecodeMode(void) {
    if (sDecodeDropdown == nil || sDecodeDropdown.selectedItem == nil) {
        sSelectedDecodeBuffer[0] = '\0';
        return sSelectedDecodeBuffer;
    }
    NSString *code = sDecodeDropdown.selectedItem.representedObject;
    const char *utf8 = [code UTF8String];
    if (utf8 == NULL) {
        sSelectedDecodeBuffer[0] = '\0';
        return sSelectedDecodeBuffer;
    }
    size_t len = strlen(utf8);
    if (len >= sizeof(sSelectedDecodeBuffer)) {
        len = sizeof(sSelectedDecodeBuffer) - 1;
    }
    memcpy(sSelectedDecodeBuffer, utf8, len);
    sSelectedDecodeBuffer[len] = '\0';
    return sSelectedDecodeBuffer;
}

void populateSettingsPunctuationModes(const char **codes, const char **names, int count, int defaultIndex) {
    [sPunctuationDropdown removeAllItems];
    for (int i = 0; i < count; i++) {
        NSString *title = [NSString stringWithUTF8String:names[i]];
        [sPunctuationDropdown addItemWithTitle:title];
        [sPunctuationDropdown lastItem].representedObject = [NSString stringWithUTF8String:codes[i]];
    }
    if (defaultIndex >= 0 && defaultIndex < count) {
        [sPunctuationDropdown selectItemAtIndex:defaultIndex];
    }
}

const char *getSelectedPunctuationMode(void) {
    if (sPunctuationDropdown == nil || sPunctuationDropdown.selectedItem == nil) {
        sSelectedPunctuationBuffer[0] = '\0';
        return sSelectedPunctuationBuffer;
    }
    NSString *code = sPunctuationDropdown.selectedItem.representedObject;
    const char *utf8 = [code UTF8String];
    if (utf8 == NULL) {
        sSelectedPunctuationBuffer[0] = '\0';
        return sSelectedPunctuationBuffer;
    }
    size_t len = strlen(utf8);
    if (len >= sizeof(sSelectedPunctuationBuffer)) {
        len = sizeof(sSelectedPunctuationBuffer) - 1;
    }
    memcpy(sSelectedPunctuationBuffer, utf8, len);
    sSelectedPunctuationBuffer[len] = '\0';
    return sSelectedPunctuationBuffer;
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

int getSettingsHotkeyKeycode(void) {
    return sConfirmedKeycode;
}

void runSetupEventLoop(void) {
    if (sIsOnboarding) {
        // Onboarding: no [NSApp run] yet — use modal to get an event loop.
        ensureNSApp();
        [NSApp runModalForWindow:sSetupWindow];
    } else {
        // Preferences: called from a background goroutine. Block on a
        // semaphore until the window closes. The main thread's [NSApp run]
        // stays responsive — menu bar clicks, macOS termination, etc. work.
        sWindowDone = dispatch_semaphore_create(0);
        dispatch_semaphore_wait(sWindowDone, DISPATCH_TIME_FOREVER);
        sWindowDone = NULL;
    }
}

void populateSettingsModels(const char **sizes, const char **descriptions, int count, int defaultIndex) {
    [sModelDropdown removeAllItems];
    for (int i = 0; i < count; i++) {
        NSString *title = [NSString stringWithUTF8String:descriptions[i]];
        [sModelDropdown addItemWithTitle:title];
        [sModelDropdown lastItem].representedObject = [NSString stringWithUTF8String:sizes[i]];
    }
    if (defaultIndex >= 0 && defaultIndex < count) {
        [sModelDropdown selectItemAtIndex:defaultIndex];
    }
}

const char *getSelectedModel(void) {
    // Return the active model (set by Go side via sActiveModelSize)
    return sActiveModelSize;
}

const char *getDropdownModel(void) {
    if (sModelDropdown == nil || sModelDropdown.selectedItem == nil) {
        sSelectedModelBuffer[0] = '\0';
        return sSelectedModelBuffer;
    }
    NSString *size = sModelDropdown.selectedItem.representedObject;
    const char *utf8 = [size UTF8String];
    if (utf8 == NULL) {
        sSelectedModelBuffer[0] = '\0';
        return sSelectedModelBuffer;
    }
    size_t len = strlen(utf8);
    if (len >= sizeof(sSelectedModelBuffer)) len = sizeof(sSelectedModelBuffer) - 1;
    memcpy(sSelectedModelBuffer, utf8, len);
    sSelectedModelBuffer[len] = '\0';
    return sSelectedModelBuffer;
}

void setActiveModelSize(const char *size) {
    strlcpy(sActiveModelSize, size, sizeof(sActiveModelSize));
}

// Update the buttons below the dropdown for the currently selected model.
// state: 0=not downloaded, 1=active, 2=downloaded not active,
//        3=downloading, 4=download failed, 5=delete confirm
void updateModelButtons(int state) {
    dispatch_async(dispatch_get_main_queue(), ^{
        switch (state) {
        case 0: // Not downloaded
            sModelBtn1.title = @"Download";
            sModelBtn1.enabled = YES;
            sModelBtn1.contentTintColor = nil;
            sModelBtn1.hidden = NO;
            sModelBtn2.hidden = YES;
            sProgressBar.hidden = YES;
            sProgressLabel.hidden = YES;
            break;
        case 1: // Active (in use)
            sModelBtn1.title = @"In Use";
            sModelBtn1.enabled = NO;
            sModelBtn1.contentTintColor = nil;
            sModelBtn1.hidden = NO;
            sModelBtn2.hidden = YES;
            sProgressBar.hidden = YES;
            sProgressLabel.hidden = YES;
            break;
        case 2: // Downloaded, not active
            sModelBtn1.title = @"Use";
            sModelBtn1.enabled = YES;
            sModelBtn1.contentTintColor = nil;
            sModelBtn1.hidden = NO;
            sModelBtn2.title = @"Delete";
            sModelBtn2.contentTintColor = [NSColor systemRedColor];
            sModelBtn2.enabled = YES;
            sModelBtn2.hidden = NO;
            sProgressBar.hidden = YES;
            sProgressLabel.hidden = YES;
            break;
        case 3: // Downloading
            sModelBtn1.title = @"Downloading...";
            sModelBtn1.enabled = NO;
            sModelBtn1.contentTintColor = nil;
            sModelBtn1.hidden = NO;
            sModelBtn2.hidden = YES;
            sProgressBar.hidden = NO;
            sProgressBar.doubleValue = 0;
            sProgressLabel.hidden = NO;
            sProgressLabel.stringValue = @"Starting...";
            break;
        case 4: // Download failed
            sModelBtn1.title = @"Download";
            sModelBtn1.enabled = YES;
            sModelBtn1.contentTintColor = nil;
            sModelBtn1.hidden = NO;
            sModelBtn2.hidden = YES;
            sProgressBar.hidden = YES;
            sProgressLabel.hidden = NO;
            sProgressLabel.stringValue = @"Download failed — try again";
            break;
        case 5: // Delete confirmation
            sModelBtn1.title = @"Confirm?";
            sModelBtn1.enabled = YES;
            sModelBtn1.contentTintColor = [NSColor systemRedColor];
            sModelBtn1.hidden = NO;
            sModelBtn2.title = @"Cancel";
            sModelBtn2.contentTintColor = nil;
            sModelBtn2.enabled = YES;
            sModelBtn2.hidden = NO;
            sProgressBar.hidden = YES;
            sProgressLabel.hidden = YES;
            break;
        }
    });
}

void updateDownloadProgress(double progress, long long downloaded, long long total) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sProgressBar.hidden = NO;
        sProgressBar.doubleValue = progress;
        sProgressLabel.hidden = NO;
        if (total > 0) {
            sProgressLabel.stringValue = [NSString stringWithFormat:@"%.0f MB / %.0f MB",
                (double)downloaded / 1048576.0, (double)total / 1048576.0];
        }
    });
}
