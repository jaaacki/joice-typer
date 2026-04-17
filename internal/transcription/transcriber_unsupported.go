//go:build !darwin

package transcription

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	apppkg "voicetype/internal/app"
)

type unsupportedTranscriber struct{}

type DownloadProgressFunc func(progress float64, bytesDownloaded, bytesTotal int64)

func unsupportedTranscriptionError() error {
	return fmt.Errorf("JoiceTyper bootstrap build for %s/%s does not provide transcription", runtime.GOOS, runtime.GOARCH)
}

func NewTranscriber(ctx context.Context, modelPath string, modelSize string, language string, sampleRate int, decodeMode string, punctuationMode string, logger *slog.Logger) (apppkg.Transcriber, error) {
	return nil, unsupportedTranscriptionError()
}

func DownloadModelWithProgress(ctx context.Context, modelPath string, modelSize string, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	return unsupportedTranscriptionError()
}

func (t *unsupportedTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	return "", unsupportedTranscriptionError()
}

func (t *unsupportedTranscriber) SetVocabulary(vocab string) {}

func (t *unsupportedTranscriber) Close() error {
	return nil
}
