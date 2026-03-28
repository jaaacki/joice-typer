#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>
#include "notification_darwin.h"

void postNotification(const char *title, const char *body) {
    @autoreleasepool {
        // Copy to NSString immediately — the C pointers will be freed by the caller
        NSString *nsTitle = [NSString stringWithUTF8String:title];
        NSString *nsBody = [NSString stringWithUTF8String:body];

        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        if (center == nil) return;

        // Request permission (first time only, macOS remembers)
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                              completionHandler:^(BOOL granted, NSError *error) {
            if (!granted) return;

            UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
            content.title = nsTitle;
            content.body = nsBody;
            content.sound = [UNNotificationSound defaultSound];

            UNNotificationRequest *request = [UNNotificationRequest
                requestWithIdentifier:@"joicetyper-ready"
                              content:content
                              trigger:nil]; // deliver immediately

            [center addNotificationRequest:request withCompletionHandler:nil];
        }];
    }
}
