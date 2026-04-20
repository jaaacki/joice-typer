//go:build windows

package platform

import (
	"context"
	"log/slog"

	apppkg "voicetype/internal/core/runtime"
	windowspkg "voicetype/internal/platform/windows"
)

type PowerEvent = windowspkg.PowerEvent

func NewHotkeyListener(triggerKeys []string, logger *slog.Logger) apppkg.HotkeyListener {
	return windowspkg.NewHotkeyListener(triggerKeys, logger)
}

func NewPaster(logger *slog.Logger) apppkg.Paster {
	return windowspkg.NewPaster(logger)
}

func PostNotification(title, body string) {
	windowspkg.PostNotification(title, body)
}

func IsFirstRun() bool {
	return windowspkg.IsFirstRun()
}

func RunSetupWizard(ctx context.Context, logger *slog.Logger) (string, error) {
	return windowspkg.RunSetupWizard(ctx, logger)
}

func InitStatusBar() {
	windowspkg.InitStatusBar()
}

func UpdateStatusBar(state apppkg.AppState) {
	windowspkg.UpdateStatusBar(state)
}

func SetStatusBarHotkeyText(text string) {
	windowspkg.SetStatusBarHotkeyText(text)
}

func InitPowerObserver() {
	windowspkg.InitPowerObserver()
}

func SetPowerEventHandler(handler func(PowerEvent)) {
	windowspkg.SetPowerEventHandler(handler)
}

func ActiveHotkey() apppkg.HotkeyListener {
	return windowspkg.ActiveHotkey()
}

func SetActiveHotkey(h apppkg.HotkeyListener) {
	windowspkg.SetActiveHotkey(h)
}

func SetSettingsLogger(logger *slog.Logger) {
	windowspkg.SetSettingsLogger(logger)
}

func SetSettingsRecorder(rec apppkg.Recorder) {
	windowspkg.SetSettingsRecorder(rec)
}

func SetQuitHandler(fn func()) {
	windowspkg.SetQuitHandler(fn)
}

func ClearHotkeyEvents() {
	windowspkg.ClearHotkeyEvents()
}

func HotkeyRestartCh() <-chan struct{} {
	return windowspkg.HotkeyRestartCh()
}

func MakePowerEventHandler(app *apppkg.App, recorder func() apppkg.Recorder, logger *slog.Logger) func(PowerEvent) {
	return windowspkg.MakePowerEventHandler(app, recorder, logger)
}

func FormatHotkeyDisplay(keys []string) string {
	return windowspkg.FormatHotkeyDisplay(keys)
}
