//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework Cocoa
#include "power_darwin.h"
*/
import "C"

import (
	"log/slog"
	"sync"
)

type PowerEvent int

const (
	PowerEventSleep PowerEvent = iota
	PowerEventWake
)

var (
	powerHandlerMu sync.RWMutex
	powerHandler   func(PowerEvent)
)

func InitPowerObserver() {
	C.startPowerObserver()
}

func SetPowerEventHandler(handler func(PowerEvent)) {
	powerHandlerMu.Lock()
	powerHandler = handler
	powerHandlerMu.Unlock()
}

//export powerEventCallback
func powerEventCallback(eventType C.int) {
	powerHandlerMu.RLock()
	handler := powerHandler
	powerHandlerMu.RUnlock()
	if handler == nil {
		return
	}

	switch int(eventType) {
	case 0:
		go handler(PowerEventSleep)
	case 1:
		go handler(PowerEventWake)
	}
}

func MakePowerEventHandler(app *App, recorder func() Recorder, logger *slog.Logger) func(PowerEvent) {
	l := logger.With("component", "power")
	return func(event PowerEvent) {
		rec := recorder()
		if rec == nil {
			return
		}

		switch event {
		case PowerEventSleep:
			l.Info("system will sleep", "operation", "powerEvent")
			rec.MarkStale("system_sleep")
		case PowerEventWake:
			l.Info("system did wake", "operation", "powerEvent")
			if app != nil && !app.IsIdle() {
				l.Info("app busy after wake, deferring audio refresh to next use",
					"operation", "powerEvent")
				return
			}
			if err := rec.RefreshDevices(); err != nil {
				l.Error("failed to refresh recorder after wake",
					"operation", "powerEvent", "error", err)
				UpdateStatusBar(StateDependencyStuck)
				return
			}
			UpdateStatusBar(StateReady)
			l.Info("recorder refreshed after wake", "operation", "powerEvent")
		}
	}
}
