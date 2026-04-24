//go:build (!windows && !darwin) || !cgo

package audio

import (
	"log/slog"

	bridgepkg "voicetype/internal/core/bridge"
)

type noopInputLevelMonitor struct{}

func NewInputLevelMonitor(sampleRate int, deviceID string, logger *slog.Logger) (InputLevelMonitor, error) {
	return &noopInputLevelMonitor{}, nil
}

func (m *noopInputLevelMonitor) Snapshot() bridgepkg.InputLevelSnapshot {
	return bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"}
}

func (m *noopInputLevelMonitor) SetInputDevice(deviceID string) error {
	return nil
}

func (m *noopInputLevelMonitor) Close() error {
	return nil
}
