//go:build darwin

package transcription

import (
	"log/slog"

	bridgepkg "voicetype/internal/core/bridge"
)

type windowsWhisperBackendState struct {
	usingBackend string
	usingVulkan  bool
	noGPUFound   bool
}

func logWindowsBackendInventory(logger *slog.Logger) {}

func beginWindowsWhisperBackendLogging(logger *slog.Logger) {}

func endWindowsWhisperBackendLogging() windowsWhisperBackendState {
	return windowsWhisperBackendState{}
}

func windowsSelectedGPUDevice() (int, bridgepkg.MachineBackendSnapshot, bool) {
	return -1, bridgepkg.MachineBackendSnapshot{}, false
}
