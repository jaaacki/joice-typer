#import "webview_darwin.h"

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static NSWindow *sWebSettingsWindow = nil;
static WKWebView *sWebSettingsView = nil;

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

            sWebSettingsView = [[WKWebView alloc] initWithFrame:[[sWebSettingsWindow contentView] bounds]];
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
