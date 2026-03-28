package main

/*
#cgo LDFLAGS: -framework UserNotifications
#include "notification_darwin.h"
#include <stdlib.h>
*/
import "C"

import "unsafe"

// PostNotification sends a macOS notification.
func PostNotification(title, body string) {
	cTitle := C.CString(title)
	cBody := C.CString(body)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cBody))
	C.postNotification(cTitle, cBody)
}
