#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>
#include "notification_darwin.h"

void postNotification(const char *title, const char *body) {
    @autoreleasepool {
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

        // Request permission (first time only, macOS remembers)
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                              completionHandler:^(BOOL granted, NSError *error) {
            if (!granted) return;

            UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
            content.title = [NSString stringWithUTF8String:title];
            content.body = [NSString stringWithUTF8String:body];
            content.sound = [UNNotificationSound defaultSound];

            UNNotificationRequest *request = [UNNotificationRequest
                requestWithIdentifier:@"joicetyper-ready"
                              content:content
                              trigger:nil]; // deliver immediately

            [center addNotificationRequest:request withCompletionHandler:nil];
        }];
    }
}
