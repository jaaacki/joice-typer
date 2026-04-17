//go:build darwin

package launcher

import (
	"log/slog"

	apppkg "voicetype/internal/app"
	audiopkg "voicetype/internal/audio"
	loggingpkg "voicetype/internal/logging"
	darwinpkg "voicetype/internal/platform/darwin"
	transcriptionpkg "voicetype/internal/transcription"
)

type App = apppkg.App
type AppState = apppkg.AppState
type HotkeyEvent = apppkg.HotkeyEvent
type HotkeyListener = apppkg.HotkeyListener
type Recorder = apppkg.Recorder
type Transcriber = apppkg.Transcriber
type Paster = apppkg.Paster
type Sound = apppkg.Sound

const (
	StateLoading         = apppkg.StateLoading
	StateReady           = apppkg.StateReady
	StateRecording       = apppkg.StateRecording
	StateTranscribing    = apppkg.StateTranscribing
	StateNoPermission    = apppkg.StateNoPermission
	StateDependencyStuck = apppkg.StateDependencyStuck
)

var (
	NewApp               = apppkg.NewApp
	NewSound             = apppkg.NewSound
	InitAudio            = audiopkg.InitAudio
	TerminateAudio       = audiopkg.TerminateAudio
	ListInputDevices     = audiopkg.ListInputDevices
	NewRecorder          = audiopkg.NewRecorder
	SetupLogger          = loggingpkg.SetupLogger
	NewHotkeyListener    = darwinpkg.NewHotkeyListener
	NewPaster            = darwinpkg.NewPaster
	PostNotification     = darwinpkg.PostNotification
	IsFirstRun           = darwinpkg.IsFirstRun
	RunSetupWizard       = darwinpkg.RunSetupWizard
	InitStatusBar        = darwinpkg.InitStatusBar
	UpdateStatusBar      = darwinpkg.UpdateStatusBar
	SetStatusBarHotkeyText = darwinpkg.SetStatusBarHotkeyText
	InitPowerObserver    = darwinpkg.InitPowerObserver
	SetPowerEventHandler = darwinpkg.SetPowerEventHandler
	NewTranscriber       = transcriptionpkg.NewTranscriber
)

func ActiveHotkey() HotkeyListener {
	return darwinpkg.ActiveHotkey()
}

func SetActiveHotkey(h HotkeyListener) {
	darwinpkg.SetActiveHotkey(h)
}

func SetSettingsLogger(logger *slog.Logger) {
	darwinpkg.SetSettingsLogger(logger)
}

func SetSettingsRecorder(rec Recorder) {
	darwinpkg.SetSettingsRecorder(rec)
}

func ClearHotkeyEvents() {
	darwinpkg.ClearHotkeyEvents()
}

func HotkeyRestartCh() <-chan struct{} {
	return darwinpkg.HotkeyRestartCh()
}

var hotkeyRestartCh = darwinpkg.HotkeyRestartCh()

func makePowerEventHandler(app *App, recorder func() Recorder, logger *slog.Logger) func(darwinpkg.PowerEvent) {
	return darwinpkg.MakePowerEventHandler(app, recorder, logger)
}

func formatHotkeyDisplay(keys []string) string {
	return darwinpkg.FormatHotkeyDisplay(keys)
}
