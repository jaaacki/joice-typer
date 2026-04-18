#import "webview_darwin.h"

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static NSWindow *sWebSettingsWindow = nil;
static WKWebView *sWebSettingsView = nil;
static id sWebSettingsWindowDelegate = nil;
static id sWebSettingsNavigationDelegate = nil;
static id sWebHotkeyFlagsMonitor = nil;
static id sWebHotkeyLocalMonitor = nil;
static uint64_t sWebRecordedFlags = 0;
static int sWebRecordedKeycode = -1;

extern void webSettingsHotkeyCaptureChanged(unsigned long long flags, int keycode, int recording);
extern void webSettingsNativeTransportInfo(char *operation, char *message);
extern void webSettingsNativeTransportWarning(char *operation, char *message);
static void stopWebHotkeyCaptureRecorder(void);

static void reportWebSettingsNativeTransportInfo(NSString *operation, NSString *message) {
    char *op = (char *)(operation != nil ? [operation UTF8String] : "unknown");
    char *msg = (char *)(message != nil ? [message UTF8String] : "unknown");
    webSettingsNativeTransportInfo(op, msg);
}

static void reportWebSettingsNativeTransportWarning(NSString *operation, NSString *message) {
    char *op = (char *)(operation != nil ? [operation UTF8String] : "unknown");
    char *msg = (char *)(message != nil ? [message UTF8String] : "unknown");
    webSettingsNativeTransportWarning(op, msg);
}

@interface JoiceTyperWebSettingsWindowDelegate : NSObject <NSWindowDelegate>
@end

@implementation JoiceTyperWebSettingsWindowDelegate

- (void)windowWillClose:(NSNotification *)notification {
    (void)notification;
    stopWebHotkeyCaptureRecorder();
    if (sWebSettingsView != nil) {
        [sWebSettingsView setNavigationDelegate:nil];
        sWebSettingsView = nil;
    }
    sWebSettingsNavigationDelegate = nil;
    sWebSettingsWindowDelegate = nil;
    sWebSettingsWindow = nil;
    webSettingsWindowClosed();
}

@end

@interface JoiceTyperWebSettingsNavigationDelegate : NSObject <WKNavigationDelegate>
@end

@implementation JoiceTyperWebSettingsNavigationDelegate

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    (void)webView;
    (void)navigation;
    reportWebSettingsNativeTransportInfo(@"web settings navigation finished",
                                         @"WKWebView finished loading embedded preferences");
    NSString *domProbe =
        @"JSON.stringify({"
        "readyState: document.readyState,"
        "hasBootstrap: !!window.__JOICETYPER_BOOTSTRAP__,"
        "scripts: Array.from(document.scripts).map(function(s){ return { src: s.src || '', type: s.type || '' }; }),"
        "stylesheets: Array.from(document.querySelectorAll('link[rel=\"stylesheet\"]')).map(function(l){ return l.href || ''; }),"
        "rootHTML: document.getElementById('root') ? document.getElementById('root').innerHTML : null,"
        "bodyText: document.body ? document.body.innerText : null"
        "})";
    [webView evaluateJavaScript:domProbe completionHandler:^(id result, NSError *error) {
        if (error != nil) {
            reportWebSettingsNativeTransportWarning(@"failed to probe web settings DOM",
                                                    error.localizedDescription ?: @"unknown DOM probe error");
            return;
        }
        NSString *snapshot = nil;
        if ([result isKindOfClass:[NSString class]]) {
            snapshot = (NSString *)result;
        } else if (result != nil) {
            snapshot = [result description];
        }
        reportWebSettingsNativeTransportInfo(@"web settings DOM snapshot",
                                             snapshot ?: @"<empty DOM snapshot>");
    }];
}

- (void)webView:(WKWebView *)webView
didFailNavigation:(WKNavigation *)navigation
       withError:(NSError *)error {
    (void)webView;
    (void)navigation;
    reportWebSettingsNativeTransportWarning(@"failed web settings navigation",
                                            error.localizedDescription ?: @"unknown navigation error");
}

- (void)webView:(WKWebView *)webView
didFailProvisionalNavigation:(WKNavigation *)navigation
                withError:(NSError *)error {
    (void)webView;
    (void)navigation;
    reportWebSettingsNativeTransportWarning(@"failed provisional web settings navigation",
                                            error.localizedDescription ?: @"unknown provisional navigation error");
}

@end

@interface JoiceTyperWebSettingsHandler : NSObject <WKScriptMessageHandler>
@end

@implementation JoiceTyperWebSettingsHandler

- (void)userContentController:(WKUserContentController *)userContentController didReceiveScriptMessage:(WKScriptMessage *)message {
    if ([message.name isEqualToString:@"joicetyperConsole"]) {
        NSString *level = @"log";
        NSString *text = @"";
        if ([message.body isKindOfClass:[NSDictionary class]]) {
            NSDictionary *payload = (NSDictionary *)message.body;
            id maybeLevel = payload[@"level"];
            id maybeText = payload[@"message"];
            if ([maybeLevel isKindOfClass:[NSString class]] && [((NSString *)maybeLevel) length] > 0) {
                level = (NSString *)maybeLevel;
            }
            if ([maybeText isKindOfClass:[NSString class]]) {
                text = (NSString *)maybeText;
            } else if (maybeText != nil) {
                text = [maybeText description];
            }
        } else if ([message.body isKindOfClass:[NSString class]]) {
            text = (NSString *)message.body;
        } else if (message.body != nil) {
            text = [message.body description];
        }

        NSString *operation = [NSString stringWithFormat:@"web settings console %@", level];
        if ([level isEqualToString:@"error"] || [level isEqualToString:@"unhandledrejection"]) {
            reportWebSettingsNativeTransportWarning(operation, text);
        } else {
            reportWebSettingsNativeTransportInfo(operation, text);
        }
        return;
    }

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

    int closeWindow = 0;
    char *response = handleWebSettingsMessage(request, &closeWindow);
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
            }
            (void)result;
            if (closeWindow == 1) {
                [sWebSettingsWindow close];
            }
        }];
    }
}

@end

void showWebSettingsWindow(const char *indexPath) {
    NSString *path = nil;
    if (indexPath != NULL) {
        path = [NSString stringWithUTF8String:indexPath];
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        reportWebSettingsNativeTransportInfo(@"show web settings window requested",
                                             @"received request to present web preferences");
        if (indexPath == NULL) {
            reportWebSettingsNativeTransportWarning(@"show web settings window requested",
                                                   @"index path is NULL");
            return;
        }

        if (path == nil || path.length == 0) {
            reportWebSettingsNativeTransportWarning(@"show web settings window requested",
                                                   @"index path string is empty");
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
            reportWebSettingsNativeTransportInfo(@"created web settings window",
                                                 @"NSWindow created for web preferences");

            WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
            WKUserContentController *controller = [[WKUserContentController alloc] init];
            [controller addScriptMessageHandler:[[JoiceTyperWebSettingsHandler alloc] init] name:@"joicetyper"];
            [controller addScriptMessageHandler:[[JoiceTyperWebSettingsHandler alloc] init] name:@"joicetyperConsole"];
            NSString *consoleBridgeSource =
                @"(function(){"
                "const native = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.joicetyperConsole;"
                "if (!native || typeof native.postMessage !== 'function') { return; }"
                "const send = (level, message) => {"
                "  try { native.postMessage({ level, message: String(message) }); } catch (_) {}"
                "};"
                "window.addEventListener('error', function(event) {"
                "  const target = event && event.target;"
                "  if (target && target !== window) {"
                "    const tag = target.tagName || 'unknown';"
                "    const source = target.src || target.href || '';"
                "    send('resourceerror', tag + ' ' + source);"
                "    return;"
                "  }"
                "  send('error', event && event.message ? event.message : 'unknown window error');"
                "}, true);"
                "window.addEventListener('unhandledrejection', function(event) {"
                "  const reason = event && event.reason !== undefined ? event.reason : 'unknown rejection';"
                "  send('unhandledrejection', reason);"
                "});"
                "const originalError = console.error ? console.error.bind(console) : null;"
                "console.error = function() {"
                "  try { send('error', Array.prototype.join.call(arguments, ' ')); } catch (_) {}"
                "  if (originalError) { originalError.apply(console, arguments); }"
                "};"
                "})();";
            WKUserScript *consoleBridgeScript = [[WKUserScript alloc] initWithSource:consoleBridgeSource
                                                                       injectionTime:WKUserScriptInjectionTimeAtDocumentStart
                                                                    forMainFrameOnly:YES];
            [controller addUserScript:consoleBridgeScript];
            configuration.userContentController = controller;

            sWebSettingsView = [[WKWebView alloc] initWithFrame:[[sWebSettingsWindow contentView] bounds]
                                                  configuration:configuration];
            sWebSettingsNavigationDelegate = [[JoiceTyperWebSettingsNavigationDelegate alloc] init];
            [sWebSettingsView setNavigationDelegate:sWebSettingsNavigationDelegate];
            [sWebSettingsView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
            [[sWebSettingsWindow contentView] addSubview:sWebSettingsView];
        }

        NSURL *indexURL = [NSURL fileURLWithPath:path];
        NSURL *readAccessURL = [indexURL URLByDeletingLastPathComponent];
        reportWebSettingsNativeTransportInfo(@"loading web settings index",
                                             indexURL.absoluteString ?: @"missing index URL");
        [sWebSettingsView loadFileURL:indexURL allowingReadAccessToURL:readAccessURL];
        [sWebSettingsWindow center];
        [sWebSettingsWindow makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
        reportWebSettingsNativeTransportInfo(@"web settings window visible",
                                             @"requested key/front activation for web preferences window");
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
