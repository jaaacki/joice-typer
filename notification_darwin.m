#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>
#include "notification_darwin.h"

static BOOL sNotificationAuthorized = NO;
static BOOL sNotificationAuthChecked = NO;

static void ensureNotificationAuth(UNUserNotificationCenter *center) {
    static dispatch_once_t sNotificationOnce;
    dispatch_once(&sNotificationOnce, ^{
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                              completionHandler:^(BOOL granted, NSError *error) {
            sNotificationAuthorized = granted;
            sNotificationAuthChecked = YES;
        }];
    });
}

void postNotification(const char *title, const char *body) {
    @autoreleasepool {
        // Copy to NSString immediately — the C pointers will be freed by the caller
        NSString *nsTitle = [NSString stringWithUTF8String:title];
        NSString *nsBody = [NSString stringWithUTF8String:body];

        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        if (center == nil) return;

        // Request permission once (dispatch_once ensures single call)
        ensureNotificationAuth(center);

        // Post the notification — the completion handler from auth may not
        // have fired yet on the first call, but UNUserNotificationCenter
        // queues requests until authorization resolves.
        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = nsTitle;
        content.body = nsBody;
        content.sound = [UNNotificationSound defaultSound];

        UNNotificationRequest *request = [UNNotificationRequest
            requestWithIdentifier:@"joicetyper-ready"
                          content:content
                          trigger:nil]; // deliver immediately

        [center addNotificationRequest:request withCompletionHandler:nil];
    }
}
