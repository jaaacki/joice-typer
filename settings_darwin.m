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
static NSProgressIndicator *sProgressBar = nil;
static NSTextField *sProgressLabel = nil;
static NSPopUpButton *sModelDropdown = nil;
static NSTextField *sModelStatus = nil;
static NSButton *sModelActionBtn = nil;
static char sSelectedModelBuffer[32] = {0};
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

static BOOL sAccessibilityGranted = NO;
static BOOL sInputMonitoringGranted = NO;
static BOOL sDownloadComplete = NO;
static BOOL sIsOnboarding = YES;

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
    if (!sIsOnboarding) {
        // Prefs mode: no Save button. Step 7 shows permission status only.
        BOOL allPerms = sAccessibilityGranted && sInputMonitoringGranted;
        if (allPerms) {
            sStep7Indicator.stringValue = @"\u2705";
            sStep7Status.stringValue = @"Settings auto-save when you close this window";
            sStep7Status.textColor = [NSColor systemGreenColor];
        } else {
            sStep7Indicator.stringValue = @"\u26A0\uFE0F";
            sStep7Status.stringValue = @"Grant all permissions above first";
            sStep7Status.textColor = [NSColor systemOrangeColor];
        }
        return;
    }
    // In onboarding, Continue requires only permissions — model download
    // happens AFTER the user clicks Continue with their selected model.
    BOOL ready = sAccessibilityGranted && sInputMonitoringGranted;
    sContinueButton.enabled = ready;
    if (ready) {
        sStep7Indicator.stringValue = @"\u2705";
        sStep7Status.stringValue = @"All set! Model will be downloaded after you continue.";
        sStep7Status.textColor = [NSColor systemGreenColor];
    } else {
        sStep7Indicator.stringValue = @"\u23F3";
        sStep7Status.stringValue = @"Waiting for permissions...";
        sStep7Status.textColor = [NSColor secondaryLabelColor];
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

extern void modelActionButtonClicked(void);
extern void modelDropdownChanged(void);

@interface SetupDelegate : NSObject <NSWindowDelegate>
- (void)continueClicked:(id)sender;
- (void)hotkeyChangeClicked:(id)sender;
- (void)hotkeyCancelClicked:(id)sender;
- (void)hotkeyConfirmClicked:(id)sender;
- (void)openAccessibilitySettings:(id)sender;
- (void)openInputMonitoringSettings:(id)sender;
- (void)modelActionClicked:(id)sender;
- (void)modelSelectionChanged:(id)sender;
- (void)stopRecorder;
@end

@implementation SetupDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    [self stopRecorder];
    // In preferences mode, closing via red X saves (auto-save on close).
    // In onboarding mode, closing via red X cancels (user must click Start).
    sSetupComplete = !sIsOnboarding;
    [NSApp stopModal];
    [sSetupWindow orderOut:nil];
    return NO;
}

- (void)continueClicked:(id)sender {
    [self stopRecorder];
    sSetupComplete = YES;
    [sSetupWindow orderOut:nil];
    [NSApp stopModal];
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

- (void)modelActionClicked:(id)sender {
    modelActionButtonClicked();
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

        // Reset permission/download tracking for onboarding gate
        sAccessibilityGranted = NO;
        sInputMonitoringGranted = NO;
        sDownloadComplete = NO;

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
            sContinueButton.enabled = NO;

            // Reset step indicators to initial state
            sStep1Indicator.stringValue = @"\u23F3";
            sStep1Status.stringValue = @"Checking...";
            sStep1Status.textColor = [NSColor secondaryLabelColor];
            sStep2Indicator.stringValue = @"\u23F3";
            sStep2Status.stringValue = @"Checking...";
            sStep2Status.textColor = [NSColor secondaryLabelColor];
            sStep6Indicator.stringValue = @"\u23F3";
            if (sModelStatus != nil) {
                sModelStatus.stringValue = @"";
            }
            if (sProgressBar != nil) {
                sProgressBar.hidden = YES;
                sProgressBar.doubleValue = 0;
            }
            if (sProgressLabel != nil) {
                sProgressLabel.hidden = YES;
                sProgressLabel.stringValue = @"";
            }
            if (sModelActionBtn != nil) {
                sModelActionBtn.title = @"Download";
                sModelActionBtn.enabled = YES;
            }

            sStep7Indicator.stringValue = @"\u23F3";
            sStep7Status.stringValue = @"Waiting...";
            sStep7Status.textColor = [NSColor secondaryLabelColor];

            // Reset hotkey display
            sHotkeyChangeBtn.hidden = NO;
            sHotkeyCancelBtn.hidden = YES;
            sHotkeyConfirmBtn.hidden = YES;

            [sSetupWindow makeKeyAndOrderFront:nil];
            [NSApp activateIgnoringOtherApps:YES];
            return;
        }

        // First-time window creation
        CGFloat w = 480, h = 750;
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

        NSTextField *s1Help = makeLabel(@"If toggled on but showing denied: remove entry (\u2212) and re-add it", 9, NO,
            [NSColor tertiaryLabelColor], NSMakeRect(pad + 28, y - 14, innerW - 90, 14));
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

        NSTextField *s2Help = makeLabel(@"If toggled on but showing denied: remove entry (\u2212) and re-add it", 9, NO,
            [NSColor tertiaryLabelColor], NSMakeRect(pad + 28, y - 14, innerW - 90, 14));
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

        // Step 6: Speech Model
        sStep6Indicator = makeLabel(@"\U0001F9E0", 16, NO, [NSColor labelColor], NSMakeRect(pad, y, 24, 24));
        [content addSubview:sStep6Indicator];
        NSTextField *s6title = makeLabel(@"6. Speech Model", 13, YES,
            [NSColor labelColor], NSMakeRect(pad + 28, y, innerW - 28, 20));
        [content addSubview:s6title];
        y -= 28;
        sModelDropdown = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 26) pullsDown:NO];
        sModelDropdown.target = sSetupDelegate;
        sModelDropdown.action = @selector(modelSelectionChanged:);
        [content addSubview:sModelDropdown];
        y -= 20;
        sModelStatus = makeLabel(@"", 10, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 14));
        [content addSubview:sModelStatus];
        y -= 22;

        sModelActionBtn = [[NSButton alloc] initWithFrame:NSMakeRect(pad + 28, y, 120, 24)];
        sModelActionBtn.title = @"Download";
        sModelActionBtn.bezelStyle = NSBezelStyleRounded;
        sModelActionBtn.target = sSetupDelegate;
        sModelActionBtn.action = @selector(modelActionClicked:);
        [content addSubview:sModelActionBtn];
        y -= 28;

        // Keep progress bar for downloads
        sProgressBar = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(pad + 28, y, innerW - 28, 8)];
        sProgressBar.style = NSProgressIndicatorStyleBar;
        sProgressBar.minValue = 0;
        sProgressBar.maxValue = 1.0;
        sProgressBar.doubleValue = 0;
        sProgressBar.indeterminate = NO;
        sProgressBar.hidden = YES;
        [content addSubview:sProgressBar];
        y -= 14;
        sProgressLabel = makeLabel(@"", 10, NO,
            [NSColor secondaryLabelColor], NSMakeRect(pad + 28, y, innerW - 28, 14));
        sProgressLabel.hidden = YES;
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

void updateSetupDownloadProgress(double progress, long long bytesDownloaded, long long bytesTotal) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep6Indicator.stringValue = @"\u2B07\uFE0F";
        sProgressBar.hidden = NO;
        sProgressBar.doubleValue = progress;
        long long mb_done = bytesDownloaded / (1024 * 1024);
        long long mb_total = bytesTotal / (1024 * 1024);
        sProgressLabel.hidden = NO;
        sProgressLabel.stringValue = [NSString stringWithFormat:@"%lld MB / %lld MB \u2014 %d%%",
            mb_done, mb_total, (int)(progress * 100)];
    });
}

void updateSetupDownloadComplete(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sDownloadComplete = YES;
        sStep6Indicator.stringValue = @"\u2705";
        sProgressBar.doubleValue = 1.0;
        sProgressBar.hidden = YES;
        sProgressLabel.hidden = YES;
        if (sModelStatus != nil) sModelStatus.stringValue = @"Model ready";
    });
}

void updateSetupDownloadFailed(const char *errorMsg) {
    NSString *msg = [NSString stringWithUTF8String:errorMsg];
    dispatch_async(dispatch_get_main_queue(), ^{
        sStep6Indicator.stringValue = @"\u274C";
        sProgressBar.hidden = YES;
        sProgressLabel.hidden = YES;
        if (sModelStatus != nil) sModelStatus.stringValue = msg;
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

        // Download step: model already loaded if we got this far
        sDownloadComplete = YES;
        sStep6Indicator.stringValue = @"\u2705";

        // Ready step — reflects actual permission state
        if (accGranted && inpGranted) {
            sStep7Indicator.stringValue = @"\u2705";
            sStep7Status.stringValue = @"Edit settings and click Save";
            sStep7Status.textColor = [NSColor systemGreenColor];
        } else {
            sStep7Indicator.stringValue = @"\u26A0\uFE0F";
            sStep7Status.stringValue = @"Grant all permissions above first";
            sStep7Status.textColor = [NSColor systemOrangeColor];
        }

        refreshContinueState();
    });
}

void updateSetupReady(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        sDownloadComplete = YES;
        sStep7Indicator.stringValue = @"\u2705";
        sStep7Status.stringValue = @"All set!";
        sStep7Status.textColor = [NSColor systemGreenColor];
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

int getSettingsHotkeyKeycode(void) {
    return sConfirmedKeycode;
}

void runSetupEventLoop(void) {
    // Ensure NSApplication singleton exists (idempotent, safe to call many times).
    ensureNSApp();
    // Run as modal session — does NOT call [NSApp run].
    // The single [NSApp run] happens later in hotkey's runMainLoop().
    [NSApp runModalForWindow:sSetupWindow];
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
    if (len >= sizeof(sSelectedModelBuffer)) {
        len = sizeof(sSelectedModelBuffer) - 1;
    }
    memcpy(sSelectedModelBuffer, utf8, len);
    sSelectedModelBuffer[len] = '\0';
    return sSelectedModelBuffer;
}

void updateSettingsModelStatus(const char *status) {
    NSString *statusStr = [NSString stringWithUTF8String:status];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (sModelStatus != nil) {
            sModelStatus.stringValue = statusStr;
        }
    });
}

void updateModelActionButton(const char *title, int enabled) {
    // Copy the string before dispatching — the caller may free it immediately.
    NSString *titleStr = [NSString stringWithUTF8String:title];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (sModelActionBtn == nil) return;
        sModelActionBtn.title = titleStr;
        sModelActionBtn.enabled = (enabled != 0);
    });
}
