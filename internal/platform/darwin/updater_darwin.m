#import "updater_darwin.h"

#import <AppKit/AppKit.h>
#import <objc/message.h>
#import <objc/runtime.h>
#import <stdlib.h>
#import <string.h>

static id sUpdaterController = nil;

static char *copyErrorMessage(NSString *message)
{
    if (message == nil) {
        message = @"unknown Sparkle startup failure";
    }
    return strdup(message.UTF8String ?: "unknown Sparkle startup failure");
}

char *startSparkleUpdater(void)
{
    @autoreleasepool {
        if (sUpdaterController != nil) {
            return NULL;
        }

        [NSApplication sharedApplication];

        NSString *frameworkPath = [[NSBundle mainBundle].privateFrameworksPath stringByAppendingPathComponent:@"Sparkle.framework"];
        NSBundle *frameworkBundle = [NSBundle bundleWithPath:frameworkPath];
        if (frameworkBundle == nil) {
            return copyErrorMessage([NSString stringWithFormat:@"missing Sparkle.framework at %@", frameworkPath]);
        }

        NSError *loadError = nil;
        if (![frameworkBundle loadAndReturnError:&loadError]) {
            return copyErrorMessage([NSString stringWithFormat:@"failed to load Sparkle.framework: %@", loadError.localizedDescription ?: @"unknown error"]);
        }

        Class updaterControllerClass = NSClassFromString(@"SPUStandardUpdaterController");
        if (updaterControllerClass == Nil) {
            return copyErrorMessage(@"SPUStandardUpdaterController is unavailable after loading Sparkle.framework");
        }

        SEL initSelector = @selector(initWithStartingUpdater:updaterDelegate:userDriverDelegate:);
        if (![updaterControllerClass instancesRespondToSelector:initSelector]) {
            return copyErrorMessage(@"SPUStandardUpdaterController initWithStartingUpdater:updaterDelegate:userDriverDelegate: is unavailable");
        }

        typedef id (*InitMsgSend)(id, SEL, BOOL, id, id);
        InitMsgSend initFn = (InitMsgSend)objc_msgSend;
        id controller = initFn([updaterControllerClass alloc], initSelector, YES, nil, nil);
        if (controller == nil) {
            return copyErrorMessage(@"failed to initialize SPUStandardUpdaterController");
        }

        sUpdaterController = controller;
        return NULL;
    }
}
