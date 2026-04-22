package audio

import bridgepkg "voicetype/internal/core/bridge"

type InputLevelMonitor interface {
	Snapshot() bridgepkg.InputLevelSnapshot
	SetInputDevice(string) error
	Close() error
}
