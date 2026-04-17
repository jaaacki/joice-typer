//go:build darwin

package darwin

import apppkg "voicetype/internal/app"

type App = apppkg.App
type AppState = apppkg.AppState

const (
	StateLoading         = apppkg.StateLoading
	StateReady           = apppkg.StateReady
	StateRecording       = apppkg.StateRecording
	StateTranscribing    = apppkg.StateTranscribing
	StateNoPermission    = apppkg.StateNoPermission
	StateDependencyStuck = apppkg.StateDependencyStuck
)

type HotkeyEvent = apppkg.HotkeyEvent

const (
	TriggerPressed  = apppkg.TriggerPressed
	TriggerReleased = apppkg.TriggerReleased
)

type HotkeyListener = apppkg.HotkeyListener
type Recorder = apppkg.Recorder
type Transcriber = apppkg.Transcriber
type Paster = apppkg.Paster
type Sound = apppkg.Sound

var NewApp = apppkg.NewApp
var NewSound = apppkg.NewSound
