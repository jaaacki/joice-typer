#import "webview_darwin.h"

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static NSWindow *sWebSettingsWindow = nil;
static WKWebView *sWebSettingsView = nil;
static id sWebSettingsWindowDelegate = nil;
static id sWebHotkeyFlagsMonitor = nil;
static id sWebHotkeyLocalMonitor = nil;
static uint64_t sWebRecordedFlags = 0;
static int sWebRecordedKeycode = -1;

extern void webSettingsHotkeyCaptureChanged(unsigned long long flags, int keycode, int recording);
extern void webSettingsNativeTransportWarning(char *operation, char *message);
static void stopWebHotkeyCaptureRecorder(void);

static void reportWebSettingsNativeTransportWarning(NSString *operation, NSString *message) {
    char *op = (char *)(operation != nil ? [operation UTF8String] : "unknown");
    char *msg = (char *)(message != nil ? [message UTF8String] : "unknown");
    webSettingsNativeTransportWarning(op, msg);
}

@interface JoiceTyperWebSettingsWindowDelegate : NSObject <NSWindowDelegate>
@end

@implementation JoiceTyperWebSettingsWindowDelegate

- (void)windowWillClose:(NSNotification *)notification {
    stopWebHotkeyCaptureRecorder();
    webSettingsWindowClosed();
}

@end

@interface JoiceTyperWebSettingsHandler : NSObject <WKScriptMessageHandler>
@end

@implementation JoiceTyperWebSettingsHandler

- (void)userContentController:(WKUserContentController *)userContentController didReceiveScriptMessage:(WKScriptMessage *)message {
    if (![message.name isEqualToString:@"joicetyper"]) {
        return;
    }
    if (![NSJSONSerialization isValidJSONObject:message.body]) {
        reportWebSettingsNativeTransportWarning(@"invalid web settings message body",
                                                @"message body is not valid JSON");
        return;
    }

    NSError *jsonError = nil;
    NSData *data = [NSJSONSerialization dataWithJSONObject:message.body options:0 error:&jsonError];
    if (data == nil || jsonError != nil) {
        reportWebSettingsNativeTransportWarning(@"failed to encode web settings message",
                                                jsonError.localizedDescription ?: @"unknown JSON encoding error");
        return;
    }

    NSString *jsonString = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    if (jsonString == nil) {
        reportWebSettingsNativeTransportWarning(@"failed to decode web settings message",
                                                @"failed to decode UTF-8 request string");
        return;
    }

    char *request = strdup([jsonString UTF8String]);
    if (request == NULL) {
        reportWebSettingsNativeTransportWarning(@"failed to duplicate web settings request",
                                                @"strdup returned NULL");
        return;
    }

    char *response = handleWebSettingsMessage(request);
    free(request);
    NSString *responseScript = nil;
    if (response != NULL) {
        responseScript = [NSString stringWithUTF8String:response];
        free(response);
    }

    if (responseScript != nil) {
        [sWebSettingsView evaluateJavaScript:responseScript
                           completionHandler:^(id result, NSError *error) {
            if (error != nil) {
                reportWebSettingsNativeTransportWarning(@"failed to evaluate web settings response script",
                                                        error.localizedDescription ?: @"unknown JavaScript evaluation error");
                return;
            }
            if ([result isKindOfClass:[NSNumber class]] && [(NSNumber *)result boolValue]) {
                [sWebSettingsWindow close];
            }
        }];
    }
}

@end

void showWebSettingsWindow(const char *indexPath) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (indexPath == NULL) {
            return;
        }

        NSString *path = [NSString stringWithUTF8String:indexPath];
        if (path == nil || path.length == 0) {
            return;
        }

        if (sWebSettingsWindow == nil) {
            NSRect frame = NSMakeRect(0, 0, 960, 700);
            NSUInteger styleMask = NSWindowStyleMaskTitled |
                                   NSWindowStyleMaskClosable |
                                   NSWindowStyleMaskMiniaturizable |
                                   NSWindowStyleMaskResizable;
            sWebSettingsWindow = [[NSWindow alloc] initWithContentRect:frame
                                                             styleMask:styleMask
                                                               backing:NSBackingStoreBuffered
                                                                 defer:NO];
            [sWebSettingsWindow setTitle:@"JoiceTyper Preferences"];
            sWebSettingsWindowDelegate = [[JoiceTyperWebSettingsWindowDelegate alloc] init];
            [sWebSettingsWindow setDelegate:sWebSettingsWindowDelegate];

            WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
            WKUserContentController *controller = [[WKUserContentController alloc] init];
            [controller addScriptMessageHandler:[[JoiceTyperWebSettingsHandler alloc] init] name:@"joicetyper"];
            configuration.userContentController = controller;

            sWebSettingsView = [[WKWebView alloc] initWithFrame:[[sWebSettingsWindow contentView] bounds]
                                                  configuration:configuration];
            [sWebSettingsView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
            [[sWebSettingsWindow contentView] addSubview:sWebSettingsView];
        }

        NSURL *indexURL = [NSURL fileURLWithPath:path];
        NSURL *readAccessURL = [indexURL URLByDeletingLastPathComponent];
        [sWebSettingsView loadFileURL:indexURL allowingReadAccessToURL:readAccessURL];
        [sWebSettingsWindow center];
        [sWebSettingsWindow makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
    });
}

void dispatchWebSettingsScript(const char *script) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (script == NULL || sWebSettingsView == nil) {
            return;
        }

        NSString *scriptString = [NSString stringWithUTF8String:script];
        if (scriptString == nil || scriptString.length == 0) {
            return;
        }

        [sWebSettingsView evaluateJavaScript:scriptString completionHandler:^(id result, NSError *error) {
            (void)result;
            if (error != nil) {
                reportWebSettingsNativeTransportWarning(@"failed to evaluate web settings event script",
                                                        error.localizedDescription ?: @"unknown JavaScript evaluation error");
            }
        }];
    });
}

static void stopWebHotkeyCaptureRecorder(void) {
    if (sWebHotkeyFlagsMonitor != nil) {
        [NSEvent removeMonitor:sWebHotkeyFlagsMonitor];
        sWebHotkeyFlagsMonitor = nil;
    }
    if (sWebHotkeyLocalMonitor != nil) {
        [NSEvent removeMonitor:sWebHotkeyLocalMonitor];
        sWebHotkeyLocalMonitor = nil;
    }
}

void startWebHotkeyCapture(void) {
    void (^startCapture)(void) = ^{
        stopWebHotkeyCaptureRecorder();
        sWebRecordedFlags = 0;
        sWebRecordedKeycode = -1;
        webSettingsHotkeyCaptureChanged(0, -1, 1);

        sWebHotkeyFlagsMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
            handler:^NSEvent *(NSEvent *event) {
                uint64_t flags = [event modifierFlags];
                uint64_t relevant = flags & (NSEventModifierFlagFunction | NSEventModifierFlagShift |
                                              NSEventModifierFlagControl | NSEventModifierFlagOption |
                                              NSEventModifierFlagCommand);
                uint64_t cgFlags = 0;
                if (relevant & NSEventModifierFlagFunction) cgFlags |= 0x800000;
                if (relevant & NSEventModifierFlagShift)    cgFlags |= 0x20000;
                if (relevant & NSEventModifierFlagControl)  cgFlags |= 0x40000;
                if (relevant & NSEventModifierFlagOption)   cgFlags |= 0x80000;
                if (relevant & NSEventModifierFlagCommand)  cgFlags |= 0x100000;
                sWebRecordedFlags = cgFlags;
                webSettingsHotkeyCaptureChanged(sWebRecordedFlags, sWebRecordedKeycode, 1);
                return event;
            }];

        sWebHotkeyLocalMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
            handler:^NSEvent *(NSEvent *event) {
                sWebRecordedKeycode = (int)[event keyCode];
                webSettingsHotkeyCaptureChanged(sWebRecordedFlags, sWebRecordedKeycode, 1);
                return nil;
            }];
    };
    if ([NSThread isMainThread]) {
        startCapture();
    } else {
        dispatch_async(dispatch_get_main_queue(), startCapture);
    }
}

void cancelWebHotkeyCapture(void) {
    void (^cancelCapture)(void) = ^{
        stopWebHotkeyCaptureRecorder();
        sWebRecordedFlags = 0;
        sWebRecordedKeycode = -1;
    };
    if ([NSThread isMainThread]) {
        cancelCapture();
    } else {
        dispatch_async(dispatch_get_main_queue(), cancelCapture);
    }
}

int confirmWebHotkeyCapture(unsigned long long *flags, int *keycode) {
    __block int ok = 0;
    void (^confirmCapture)(void) = ^{
        if (flags != NULL) {
            *flags = sWebRecordedFlags;
        }
        if (keycode != NULL) {
            *keycode = sWebRecordedKeycode;
        }
        ok = (sWebRecordedFlags != 0 || sWebRecordedKeycode >= 0) ? 1 : 0;
        stopWebHotkeyCaptureRecorder();
    };
    if ([NSThread isMainThread]) {
        confirmCapture();
    } else {
        dispatch_sync(dispatch_get_main_queue(), confirmCapture);
    }
    return ok;
}
