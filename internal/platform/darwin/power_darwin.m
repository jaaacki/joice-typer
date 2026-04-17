#import <Cocoa/Cocoa.h>
#include "hotkey_darwin.h"
#include "power_darwin.h"

extern void powerEventCallback(int eventType);

@interface JoiceTyperPowerObserver : NSObject
@end

@implementation JoiceTyperPowerObserver
- (void)handleWillSleep:(NSNotification *)notification {
    powerEventCallback(0);
}

- (void)handleDidWake:(NSNotification *)notification {
    powerEventCallback(1);
}
@end

static JoiceTyperPowerObserver *sPowerObserver = nil;

void startPowerObserver(void) {
    ensureNSApp();

    static dispatch_once_t once;
    dispatch_once(&once, ^{
        sPowerObserver = [[JoiceTyperPowerObserver alloc] init];
        NSNotificationCenter *center = [[NSWorkspace sharedWorkspace] notificationCenter];
        [center addObserver:sPowerObserver
                   selector:@selector(handleWillSleep:)
                       name:NSWorkspaceWillSleepNotification
                     object:nil];
        [center addObserver:sPowerObserver
                   selector:@selector(handleDidWake:)
                       name:NSWorkspaceDidWakeNotification
                     object:nil];
    });
}
