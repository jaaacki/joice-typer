//go:build darwin

package darwin

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	audiopkg "voicetype/internal/core/audio"
)

type runtimeState struct {
	mu               sync.Mutex
	activeHotkey     HotkeyListener
	settingsLogger   *slog.Logger
	settingsRecorder Recorder
	inputMonitor     audiopkg.InputLevelMonitor
	hotkeyRestartCh  chan struct{}

	prefsMu     sync.Mutex
	prefsCtx    context.Context
	prefsCancel context.CancelFunc
	prefsOpen   int32
	appState    int32
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

func preferencesOpenCompareAndSwap(old, new int32) bool {
	return atomic.CompareAndSwapInt32(&runtimeSingleton.prefsOpen, old, new)
}

func preferencesOpenStore(v int32) {
	atomic.StoreInt32(&runtimeSingleton.prefsOpen, v)
}

func preferencesOpenLoad() int32 {
	return atomic.LoadInt32(&runtimeSingleton.prefsOpen)
}

func setPreferencesContext(ctx context.Context, cancel context.CancelFunc) {
	runtimeSingleton.prefsMu.Lock()
	if runtimeSingleton.prefsCancel != nil {
		runtimeSingleton.prefsCancel()
	}
	runtimeSingleton.prefsCtx = ctx
	runtimeSingleton.prefsCancel = cancel
	runtimeSingleton.prefsMu.Unlock()
}

func currentPreferencesContext() context.Context {
	runtimeSingleton.prefsMu.Lock()
	defer runtimeSingleton.prefsMu.Unlock()
	return runtimeSingleton.prefsCtx
}

func cancelPreferencesContext() {
	runtimeSingleton.prefsMu.Lock()
	if runtimeSingleton.prefsCancel != nil {
		runtimeSingleton.prefsCancel()
	}
	runtimeSingleton.prefsMu.Unlock()
}

func storeCurrentAppState(state AppState) {
	atomic.StoreInt32(&runtimeSingleton.appState, int32(state))
}

func currentAppState() AppState {
	return AppState(atomic.LoadInt32(&runtimeSingleton.appState))
}

func ClearHotkeyEvents() {
	hotkeyMu.Lock()
	hotkeyEvents = nil
	hotkeyMu.Unlock()
}

func setSettingsInputMonitor(m audiopkg.InputLevelMonitor) {
	runtimeSingleton.mu.Lock()
	defer runtimeSingleton.mu.Unlock()
	runtimeSingleton.inputMonitor = m
}

func currentSettingsInputMonitor() audiopkg.InputLevelMonitor {
	runtimeSingleton.mu.Lock()
	defer runtimeSingleton.mu.Unlock()
	return runtimeSingleton.inputMonitor
}
