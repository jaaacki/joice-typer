package main

// AppState represents the current operational state of JoiceTyper.
type AppState int

const (
	StateLoading      AppState = iota
	StateReady
	StateRecording
	StateTranscribing
	StateNoPermission
)

func (s AppState) String() string {
	switch s {
	case StateLoading:
		return "loading"
	case StateReady:
		return "ready"
	case StateRecording:
		return "recording"
	case StateTranscribing:
		return "transcribing"
	case StateNoPermission:
		return "no_permission"
	default:
		return "unknown"
	}
}
