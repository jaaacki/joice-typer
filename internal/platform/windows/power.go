//go:build windows

package windows

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

func InitPowerObserver() {}

func SetPowerEventHandler(handler func(PowerEvent)) {
	powerHandlerMu.Lock()
	powerHandler = handler
	powerHandlerMu.Unlock()
}

func MakePowerEventHandler(app *App, recorder func() Recorder, logger *slog.Logger) func(PowerEvent) {
	return func(event PowerEvent) {
		_ = app
		_ = recorder
		_ = logger
		powerHandlerMu.RLock()
		handler := powerHandler
		powerHandlerMu.RUnlock()
		if handler != nil {
			handler(event)
		}
	}
}
