//go:build windows && !cgo

package audio

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

type unsupportedRecorder struct {
	logger *slog.Logger
}

func unsupportedAudioError(operation string) error {
	return apppkg.UnsupportedDependencyError("recorder", operation, "audio recording", runtime.GOOS, runtime.GOARCH)
}

func NewRecorder(sampleRate int, deviceName string, logger *slog.Logger) apppkg.Recorder {
	return &unsupportedRecorder{logger: logger.With("component", "recorder")}
}

func InitAudio() error {
	return nil
}

func TerminateAudio() error {
	return nil
}

func ListInputDevices() error {
	devices, err := ListInputDeviceSnapshots()
	if err != nil {
		return fmt.Errorf("recorder.ListInputDevices: %w", err)
	}
	fmt.Println("Available input devices:")
	for _, device := range devices {
		defaultSuffix := ""
		if device.IsDefault {
			defaultSuffix = " (default)"
		}
		fmt.Printf("  %s%s\n", device.Name, defaultSuffix)
	}
	fmt.Printf("\nSet input_device in %s to use a specific device.\n", listDevicesConfigHint())
	return nil
}

func listDevicesConfigHint() string {
	cfgPath, err := configpkg.DefaultConfigPath()
	if err != nil {
		return `%APPDATA%\JoiceTyper\config.yaml`
	}
	return cfgPath
}

func (r *unsupportedRecorder) Warm() {}

func (r *unsupportedRecorder) Start(ctx context.Context) error {
	return unsupportedAudioError("Start")
}

func (r *unsupportedRecorder) Stop() ([]float32, error) {
	return nil, unsupportedAudioError("Stop")
}

func (r *unsupportedRecorder) Snapshot() []float32 {
	return nil
}

func (r *unsupportedRecorder) RefreshDevices() error {
	return unsupportedAudioError("RefreshDevices")
}

func (r *unsupportedRecorder) MarkStale(reason string) {}

func (r *unsupportedRecorder) Close() error {
	return unsupportedAudioError("Close")
}
