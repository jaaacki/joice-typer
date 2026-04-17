//go:build !darwin

package audio

import (
	"context"
	"log/slog"
	"runtime"

	apppkg "voicetype/internal/app"
)

type unsupportedRecorder struct {
	logger *slog.Logger
}

func unsupportedAudioError() error {
	return apppkg.UnsupportedDependencyError("recorder", "unsupported", "audio recording", runtime.GOOS, runtime.GOARCH)
}

func NewRecorder(sampleRate int, deviceName string, logger *slog.Logger) apppkg.Recorder {
	return &unsupportedRecorder{logger: logger.With("component", "recorder")}
}

func InitAudio() error {
	return unsupportedAudioError()
}

func TerminateAudio() error {
	return apppkg.UnsupportedDependencyError("recorder", "TerminateAudio", "audio recording", runtime.GOOS, runtime.GOARCH)
}

func ListInputDevices() error {
	return unsupportedAudioError()
}

func (r *unsupportedRecorder) Warm() {}

func (r *unsupportedRecorder) Start(ctx context.Context) error {
	return unsupportedAudioError()
}

func (r *unsupportedRecorder) Stop() ([]float32, error) {
	return nil, unsupportedAudioError()
}

func (r *unsupportedRecorder) Snapshot() []float32 {
	return nil
}

func (r *unsupportedRecorder) RefreshDevices() error {
	return unsupportedAudioError()
}

func (r *unsupportedRecorder) MarkStale(reason string) {}

func (r *unsupportedRecorder) Close() error {
	return apppkg.UnsupportedDependencyError("recorder", "Close", "audio recording", runtime.GOOS, runtime.GOARCH)
}
