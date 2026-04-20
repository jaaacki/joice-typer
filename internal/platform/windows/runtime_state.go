//go:build windows

package windows

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

type runtimeState struct {
	mu               sync.Mutex
	activeHotkey     HotkeyListener
	settingsLogger   *slog.Logger
	settingsRecorder Recorder
	hotkeyRestartCh  chan struct{}

	appState int32
}

var runtimeSingleton = &runtimeState{
	settingsLogger:  slog.Default().With("component", "settings"),
	hotkeyRestartCh: make(chan struct{}, 1),
}

func SetActiveHotkey(h HotkeyListener) {
	runtimeSingleton.mu.Lock()
	runtimeSingleton.activeHotkey = h
	runtimeSingleton.mu.Unlock()
}

func ActiveHotkey() HotkeyListener {
	runtimeSingleton.mu.Lock()
	defer runtimeSingleton.mu.Unlock()
	return runtimeSingleton.activeHotkey
}

func SetSettingsLogger(logger *slog.Logger) {
	runtimeSingleton.mu.Lock()
	if logger == nil {
		runtimeSingleton.settingsLogger = slog.Default().With("component", "settings")
	} else {
		runtimeSingleton.settingsLogger = logger
	}
	runtimeSingleton.mu.Unlock()
}

func currentSettingsLogger() *slog.Logger {
	runtimeSingleton.mu.Lock()
	defer runtimeSingleton.mu.Unlock()
	if runtimeSingleton.settingsLogger == nil {
		return slog.Default().With("component", "settings")
	}
	return runtimeSingleton.settingsLogger
}

func SetSettingsRecorder(rec Recorder) {
	runtimeSingleton.mu.Lock()
	runtimeSingleton.settingsRecorder = rec
	runtimeSingleton.mu.Unlock()
}

func currentSettingsRecorder() Recorder {
	runtimeSingleton.mu.Lock()
	defer runtimeSingleton.mu.Unlock()
	return runtimeSingleton.settingsRecorder
}

func HotkeyRestartCh() <-chan struct{} {
	return runtimeSingleton.hotkeyRestartCh
}

func signalHotkeyRestartCh() {
	select {
	case runtimeSingleton.hotkeyRestartCh <- struct{}{}:
	default:
	}
}

func storeCurrentAppState(state AppState) {
	atomic.StoreInt32(&runtimeSingleton.appState, int32(state))
}

func currentAppState() AppState {
	return AppState(atomic.LoadInt32(&runtimeSingleton.appState))
}

func ClearHotkeyEvents() {}
