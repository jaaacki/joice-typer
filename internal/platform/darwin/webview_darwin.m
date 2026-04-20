#import "webview_darwin.h"

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <string.h>

static NSWindow *sWebSettingsWindow = nil;
static WKWebView *sWebSettingsView = nil;
static id sWebSettingsWindowDelegate = nil;
static id sWebSettingsNavigationDelegate = nil;
static id sWebHotkeyFlagsMonitor = nil;
static id sWebHotkeyLocalMonitor = nil;
static uint64_t sWebRecordedFlags = 0;
static int sWebRecordedKeycode = -1;
static NSString * const kJoiceTyperBridgeEventName = @"joicetyper-bridge-message";

extern void webSettingsHotkeyCaptureChanged(unsigned long long flags, int keycode, int recording);
extern void webSettingsNativeTransportInfo(char *operation, char *message);
extern void webSettingsNativeTransportWarning(char *operation, char *message);
static void stopWebHotkeyCaptureRecorder(void);
static void dispatchBridgeErrorResponse(NSString *requestID, NSString *message);
static NSString *bridgeJSONStringLiteral(NSString *value);
static void dispatchBridgeEnvelopeJSON(NSString *payloadJSON, BOOL closeWindow);

static BOOL shouldProbeWebSettingsDOM(void) {
    const char *value = getenv("JOICETYPER_DEBUG_WEB_SETTINGS");
    return value != NULL && strcmp(value, "1") == 0;
}

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

static NSString *bridgeJSONStringLiteral(NSString *value) {
    NSError *jsonError = nil;
    NSData *data = [NSJSONSerialization dataWithJSONObject:@[value ?: @""] options:0 error:&jsonError];
    if (data == nil || jsonError != nil) {
        reportWebSettingsNativeTransportWarning(@"failed to encode bridge payload string literal",
                                                jsonError.localizedDescription ?: @"unknown JSON string encoding error");
        return nil;
    }
    NSString *arrayLiteral = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    if (arrayLiteral == nil || arrayLiteral.length < 2) {
        reportWebSettingsNativeTransportWarning(@"failed to encode bridge payload string literal",
                                                @"failed to decode UTF-8 payload string literal");
        return nil;
    }
    return [arrayLiteral substringWithRange:NSMakeRange(1, arrayLiteral.length - 2)];
}

static void dispatchBridgeEnvelopeJSON(NSString *payloadJSON, BOOL closeWindow) {
    if (sWebSettingsView == nil || payloadJSON == nil) {
        return;
    }
    NSString *payloadLiteral = bridgeJSONStringLiteral(payloadJSON);
    if (payloadLiteral == nil) {
        return;
    }
    NSString *script = [NSString stringWithFormat:
                        @"(function(){"
                        "try {"
                        "  const detail = typeof %@ === 'string' ? JSON.parse(%@) : %@;"
                        "  window.dispatchEvent(new CustomEvent('%@', { detail }));"
                        "  return 'ok';"
                        "} catch (error) {"
                        "  return 'dispatch_error:' + (error && error.message ? error.message : String(error));"
                        "}"
                        "})();",
                        payloadLiteral,
                        payloadLiteral,
                        payloadLiteral,
                        kJoiceTyperBridgeEventName];
    [sWebSettingsView evaluateJavaScript:script completionHandler:^(id result, NSError *error) {
        if (error != nil) {
            reportWebSettingsNativeTransportWarning(@"failed to evaluate bridge envelope dispatch",
                                                    error.localizedDescription ?: @"unknown JavaScript evaluation error");
            return;
        }
        if ([result isKindOfClass:[NSString class]] && [(NSString *)result hasPrefix:@"dispatch_error:"]) {
            reportWebSettingsNativeTransportWarning(@"failed to dispatch bridge envelope in page",
                                                    [(NSString *)result substringFromIndex:[@"dispatch_error:" length]]);
            return;
        }
        if (closeWindow && sWebSettingsWindow != nil) {
            [sWebSettingsWindow close];
        }
    }];
}

static void dispatchBridgeErrorResponse(NSString *requestID, NSString *message) {
    if (sWebSettingsView == nil) {
        return;
    }
    NSDictionary *payload = @{
        @"v": @1,
        @"kind": @"response",
        @"id": requestID ?: @"",
        @"ok": @NO,
        @"error": @{
            @"code": @"bridge.invalid_request",
            @"message": message ?: @"invalid bridge request",
            @"details": @{},
            @"retriable": @NO,
        },
    };
    NSError *jsonError = nil;
    NSData *data = [NSJSONSerialization dataWithJSONObject:payload options:0 error:&jsonError];
    if (data == nil || jsonError != nil) {
        reportWebSettingsNativeTransportWarning(@"failed to encode bridge error response",
                                                jsonError.localizedDescription ?: @"unknown JSON encoding error");
        return;
    }
    NSString *jsonString = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    if (jsonString == nil) {
        reportWebSettingsNativeTransportWarning(@"failed to encode bridge error response",
                                                @"failed to decode UTF-8 response string");
        return;
    }
    dispatchBridgeEnvelopeJSON(jsonString, NO);
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
    if (!shouldProbeWebSettingsDOM()) {
        return;
    }
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
    NSString *requestID = nil;
    if ([message.body isKindOfClass:[NSDictionary class]]) {
        id maybeID = ((NSDictionary *)message.body)[@"id"];
        if ([maybeID isKindOfClass:[NSString class]]) {
            requestID = (NSString *)maybeID;
        }
    }
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
        dispatchBridgeErrorResponse(requestID, @"message body is not valid JSON");
        return;
    }

    NSError *jsonError = nil;
    NSData *data = [NSJSONSerialization dataWithJSONObject:message.body options:0 error:&jsonError];
    if (data == nil || jsonError != nil) {
        reportWebSettingsNativeTransportWarning(@"failed to encode web settings message",
                                                jsonError.localizedDescription ?: @"unknown JSON encoding error");
        dispatchBridgeErrorResponse(requestID, jsonError.localizedDescription ?: @"unknown JSON encoding error");
        return;
    }

    NSString *jsonString = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    if (jsonString == nil) {
        reportWebSettingsNativeTransportWarning(@"failed to decode web settings message",
                                                @"failed to decode UTF-8 request string");
        dispatchBridgeErrorResponse(requestID, @"failed to decode UTF-8 request string");
        return;
    }

    char *request = strdup([jsonString UTF8String]);
    if (request == NULL) {
        reportWebSettingsNativeTransportWarning(@"failed to duplicate web settings request",
                                                @"strdup returned NULL");
        dispatchBridgeErrorResponse(requestID, @"strdup returned NULL");
        return;
    }

    int closeWindow = 0;
    char *response = handleWebSettingsMessage(request, &closeWindow);
    free(request);
    NSString *responseJSON = nil;
    if (response != NULL) {
        responseJSON = [NSString stringWithUTF8String:response];
        free(response);
    }

    if (responseJSON != nil) {
        dispatchBridgeEnvelopeJSON(responseJSON, closeWindow == 1);
    }
}

@end

void showWebSettingsWindow(const char *htmlContent) {
    NSString *html = nil;
    if (htmlContent != NULL) {
        html = [NSString stringWithUTF8String:htmlContent];
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        reportWebSettingsNativeTransportInfo(@"show web settings window requested",
                                             @"received request to present web preferences");
        if (htmlContent == NULL) {
            reportWebSettingsNativeTransportWarning(@"show web settings window requested",
                                                   @"html content is NULL");
            return;
        }

        if (html == nil || html.length == 0) {
            reportWebSettingsNativeTransportWarning(@"show web settings window requested",
                                                   @"html content string is empty");
            return;
        }

        if (sWebSettingsWindow != nil) {
            [sWebSettingsWindow makeKeyAndOrderFront:nil];
            [NSApp activateIgnoringOtherApps:YES];
            reportWebSettingsNativeTransportInfo(@"focused existing web settings window",
                                                 @"reused existing web preferences window");
            return;
        }

        if (sWebSettingsWindow == nil) {
            NSRect frame = NSMakeRect(0, 0, 1120, 860);
            NSUInteger styleMask = NSWindowStyleMaskTitled |
                                   NSWindowStyleMaskClosable |
                                   NSWindowStyleMaskMiniaturizable |
                                   NSWindowStyleMaskResizable;
            sWebSettingsWindow = [[NSWindow alloc] initWithContentRect:frame
                                                             styleMask:styleMask
                                                               backing:NSBackingStoreBuffered
                                                                 defer:NO];
            [sWebSettingsWindow setTitle:@"JoiceTyper Preferences"];
            [sWebSettingsWindow setMinSize:NSMakeSize(1080, 820)];
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

        reportWebSettingsNativeTransportInfo(@"loading embedded web settings html",
                                             @"loading inlined web preferences shell");
        [sWebSettingsView loadHTMLString:html baseURL:nil];
        [sWebSettingsWindow center];
        [sWebSettingsWindow makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
        reportWebSettingsNativeTransportInfo(@"web settings window visible",
                                             @"requested key/front activation for web preferences window");
    });
}

void focusWebSettingsWindow(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (sWebSettingsWindow == nil) {
            return;
        }
        [sWebSettingsWindow makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
        reportWebSettingsNativeTransportInfo(@"focused existing web settings window",
                                             @"requested key/front activation for existing web preferences window");
    });
}

void dispatchWebSettingsEnvelope(const char *payloadJSON, int closeWindow) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (payloadJSON == NULL || sWebSettingsView == nil) {
            return;
        }

        NSString *payloadString = [NSString stringWithUTF8String:payloadJSON];
        if (payloadString == nil || payloadString.length == 0) {
            return;
        }
        dispatchBridgeEnvelopeJSON(payloadString, closeWindow == 1);
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
