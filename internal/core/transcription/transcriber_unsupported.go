//go:build !darwin && (!windows || !cgo)

package transcription

import (
	"context"
	"log/slog"
	"runtime"

	bridgepkg "voicetype/internal/core/bridge"
	apppkg "voicetype/internal/core/runtime"
)

type unsupportedTranscriber struct{}

func unsupportedTranscriptionError(operation string) error {
	return apppkg.UnsupportedDependencyError("transcriber", operation, "transcription", runtime.GOOS, runtime.GOARCH)
}

func WhisperSystemInfo() string {
	return ""
}

func WindowsBackendInventory() []bridgepkg.MachineBackendSnapshot {
	return nil
}

func NewTranscriber(ctx context.Context, modelPath string, modelSize string, language string, sampleRate int, decodeMode string, punctuationMode string, outputMode string, logger *slog.Logger) (apppkg.Transcriber, error) {
	return nil, unsupportedTranscriptionError("NewTranscriber")
}

func (t *unsupportedTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	return "", unsupportedTranscriptionError("Transcribe")
}

func (t *unsupportedTranscriber) SetVocabulary(vocab string) {}

func (t *unsupportedTranscriber) Close() error {
	return unsupportedTranscriptionError("Close")
}
