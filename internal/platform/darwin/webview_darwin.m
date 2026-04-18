#import "webview_darwin.h"

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static NSWindow *sWebSettingsWindow = nil;
static WKWebView *sWebSettingsView = nil;
static id sWebSettingsWindowDelegate = nil;

@interface JoiceTyperWebSettingsWindowDelegate : NSObject <NSWindowDelegate>
@end

@implementation JoiceTyperWebSettingsWindowDelegate

- (void)windowWillClose:(NSNotification *)notification {
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
        return;
    }

    NSError *jsonError = nil;
    NSData *data = [NSJSONSerialization dataWithJSONObject:message.body options:0 error:&jsonError];
    if (data == nil || jsonError != nil) {
        return;
    }

    NSString *jsonString = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    if (jsonString == nil) {
        return;
    }

    char *request = strdup([jsonString UTF8String]);
    if (request == NULL) {
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
        [sWebSettingsView evaluateJavaScript:responseScript completionHandler:nil];
    }

    if (response == NULL) {
        [sWebSettingsWindow close];
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
