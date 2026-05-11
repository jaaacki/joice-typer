//go:build darwin

package platform

import (
	"context"
	"log/slog"

	apppkg "voicetype/internal/core/runtime"
	darwinpkg "voicetype/internal/platform/darwin"
)

type PowerEvent = darwinpkg.PowerEvent

func NewHotkeyListener(triggerKeys []string, logger *slog.Logger) apppkg.HotkeyListener {
	return darwinpkg.NewHotkeyListener(triggerKeys, logger)
}

func NewPaster(logger *slog.Logger) apppkg.Paster {
	return darwinpkg.NewPaster(logger)
}

func PostNotification(title, body string) {
	darwinpkg.PostNotification(title, body)
}

func IsFirstRun() bool {
	return darwinpkg.IsFirstRun()
}

// RunWebOnboardingWizard opens the embedded webview in onboarding mode and
// blocks until the user closes the window. The legacy native AppKit setup
// wizard is gone — webview is the only UI.
func RunWebOnboardingWizard(ctx context.Context, logger *slog.Logger) error {
	return darwinpkg.RunWebOnboardingWizard(ctx, logger)
}

func InitStatusBar() {
	darwinpkg.InitStatusBar()
}

func StartUpdater() {
	darwinpkg.StartUpdater()
}

func UpdateStatusBar(state apppkg.AppState) {
	darwinpkg.UpdateStatusBar(state)
}

func SetStatusBarHotkeyText(text string) {
	darwinpkg.SetStatusBarHotkeyText(text)
}

func InitPowerObserver() {
	darwinpkg.InitPowerObserver()
}

func SetPowerEventHandler(handler func(PowerEvent)) {
	darwinpkg.SetPowerEventHandler(handler)
}

func ActiveHotkey() apppkg.HotkeyListener {
	return darwinpkg.ActiveHotkey()
}

func SetActiveHotkey(h apppkg.HotkeyListener) {
	darwinpkg.SetActiveHotkey(h)
}

func SetSettingsLogger(logger *slog.Logger) {
	darwinpkg.SetSettingsLogger(logger)
}

func SetSettingsRecorder(rec apppkg.Recorder) {
	darwinpkg.SetSettingsRecorder(rec)
}

func ClearHotkeyEvents() {
	darwinpkg.ClearHotkeyEvents()
}

func HotkeyRestartCh() <-chan struct{} {
	return darwinpkg.HotkeyRestartCh()
}

func MakePowerEventHandler(app *apppkg.App, recorder func() apppkg.Recorder, logger *slog.Logger) func(PowerEvent) {
	return darwinpkg.MakePowerEventHandler(app, recorder, logger)
}

func FormatHotkeyDisplay(keys []string) string {
	return darwinpkg.FormatHotkeyDisplay(keys)
}
