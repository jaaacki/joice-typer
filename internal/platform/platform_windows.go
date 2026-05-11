//go:build windows

package platform

import (
	"context"
	"log/slog"

	configpkg "voicetype/internal/core/config"
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

// RunWebOnboardingWizard opens the embedded WebView2 in onboarding mode and
// blocks until the user closes the window. Windows already routed first-run
// through the webview path; this is the cross-platform-symmetric entry point
// the launcher uses on both macOS and Windows.
func RunWebOnboardingWizard(ctx context.Context, logger *slog.Logger) error {
	_, err := windowspkg.RunSetupWizard(ctx, logger)
	return err
}

func InitStatusBar() {
	windowspkg.InitStatusBar()
}

func StartUpdater() {}

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

func MigrateWindowsInputDeviceConfig(cfg configpkg.Config) configpkg.Config {
	return windowspkg.MigrateWindowsInputDeviceConfig(cfg)
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
