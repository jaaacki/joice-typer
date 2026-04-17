//go:build darwin

package darwin

import (
	"log/slog"
	"sync"
)

var (
	activeHotkeyMu sync.Mutex
	activeHotkey   HotkeyListener
)

func SetActiveHotkey(h HotkeyListener) {
	activeHotkeyMu.Lock()
	activeHotkey = h
	activeHotkeyMu.Unlock()
}

func ActiveHotkey() HotkeyListener {
	activeHotkeyMu.Lock()
	defer activeHotkeyMu.Unlock()
	return activeHotkey
}

func SetSettingsLogger(logger *slog.Logger) {
	settingsLogger = logger
}

func SetSettingsRecorder(rec Recorder) {
	settingsRecorder = rec
}

func HotkeyRestartCh() <-chan struct{} {
	return hotkeyRestartCh
}

func ClearHotkeyEvents() {
	hotkeyMu.Lock()
	hotkeyEvents = nil
	hotkeyMu.Unlock()
}
