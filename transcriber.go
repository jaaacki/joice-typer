package main

/*
#cgo CFLAGS: -I${SRCDIR}/third_party/whisper.cpp/include -I${SRCDIR}/third_party/whisper.cpp/ggml/include
#cgo LDFLAGS: -L${SRCDIR}/third_party/whisper.cpp/build/src -L${SRCDIR}/third_party/whisper.cpp/build/ggml/src -L${SRCDIR}/third_party/whisper.cpp/build/ggml/src/ggml-metal -L${SRCDIR}/third_party/whisper.cpp/build/ggml/src/ggml-blas -lwhisper -lggml -lggml-base -lggml-cpu -lggml-metal -lggml-blas -lstdc++ -framework Accelerate -framework Metal -framework Foundation -framework CoreML
#include <whisper.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

type whisperTranscriber struct {
	ctx    *C.struct_whisper_context
	lang   string
	logger *slog.Logger
}

func NewTranscriber(modelPath string, language string, logger *slog.Logger) (Transcriber, error) {
	l := logger.With("component", "transcriber")

	if err := ensureModel(modelPath, l); err != nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: %w", err)
	}

	l.Info("loading model", "operation", "NewTranscriber", "model_path", modelPath)

	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	cparams := C.whisper_context_default_params()
	ctx := C.whisper_init_from_file_with_params(cPath, cparams)
	if ctx == nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: failed to load model from %s", modelPath)
	}

	l.Info("model loaded", "operation", "NewTranscriber")
	return &whisperTranscriber{ctx: ctx, lang: language, logger: l}, nil
}

func (t *whisperTranscriber) Transcribe(audio []float32) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("transcriber.Transcribe: empty audio buffer")
	}

	t.logger.Info("transcribing", "operation", "Transcribe", "samples", len(audio))

	params := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
	params.print_progress = C._Bool(false)
	params.print_timestamps = C._Bool(false)
	params.print_realtime = C._Bool(false)
	params.print_special = C._Bool(false)
	params.single_segment = C._Bool(false)

	if t.lang != "" {
		cLang := C.CString(t.lang)
		defer C.free(unsafe.Pointer(cLang))
		params.language = cLang
	}

	result := C.whisper_full(t.ctx, params, (*C.float)(unsafe.Pointer(&audio[0])), C.int(len(audio)))
	if result != 0 {
		return "", fmt.Errorf("transcriber.Transcribe: whisper_full returned %d", result)
	}

	nSegments := int(C.whisper_full_n_segments(t.ctx))
	var segments []string
	for i := 0; i < nSegments; i++ {
		text := C.GoString(C.whisper_full_get_segment_text(t.ctx, C.int(i)))
		segments = append(segments, text)
	}

	text := strings.TrimSpace(strings.Join(segments, ""))
	t.logger.Info("transcribed", "operation", "Transcribe",
		"segments", nSegments, "text_length", len(text))
	return text, nil
}

func (t *whisperTranscriber) Close() error {
	t.logger.Info("closing", "operation", "Close")
	if t.ctx != nil {
		C.whisper_free(t.ctx)
		t.ctx = nil
	}
	return nil
}

// DownloadProgressFunc is called during model download with progress info.
type DownloadProgressFunc func(progress float64, bytesDownloaded, bytesTotal int64)

// ensureModel checks if the model file exists and downloads it if not.
func ensureModel(modelPath string, logger *slog.Logger) error {
	info, err := os.Stat(modelPath)
	if err == nil {
		// File exists — validate minimum size (whisper small model is ~460MB)
		const minModelBytes = 100 * 1024 * 1024 // 100MB minimum for any whisper model
		if info.Size() < minModelBytes {
			logger.Warn("model file appears truncated, re-downloading",
				"operation", "ensureModel",
				"path", modelPath,
				"size", info.Size(),
				"min_expected", minModelBytes,
			)
			if err := os.Remove(modelPath); err != nil {
				return fmt.Errorf("transcriber.ensureModel: remove corrupt model: %w", err)
			}
			// Fall through to download
		} else {
			logger.Info("model found", "operation", "ensureModel",
				"path", modelPath, "size", info.Size())
			return nil
		}
	}

	var lastPct int
	return downloadModelWithProgress(modelPath, func(progress float64, downloaded, total int64) {
		pct := int(progress * 100)
		if pct/10 > lastPct/10 {
			logger.Info("downloading model", "operation", "ensureModel",
				"progress_pct", pct, "bytes_written", downloaded, "bytes_total", total)
			lastPct = pct
		}
	}, logger)
}

// downloadModelWithProgress downloads a whisper model to modelPath, calling
// onProgress with download progress. The caller is responsible for existence
// checks — this function always downloads.
func downloadModelWithProgress(modelPath string, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	modelFile := filepath.Base(modelPath)
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + modelFile

	logger.Warn("downloading model from network",
		"operation", "downloadModelWithProgress", "url", url, "dest", modelPath)

	dir := filepath.Dir(modelPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: create dir: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("transcriber.downloadModelWithProgress: status %d", resp.StatusCode)
	}

	// Reject HTML error pages from proxies
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		return fmt.Errorf("transcriber.downloadModelWithProgress: got HTML instead of model")
	}

	tmpPath := modelPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: create file: %w", err)
	}

	// Cap download at 2GB to prevent abuse
	const maxDownloadBytes = 2 * 1024 * 1024 * 1024
	limitedBody := io.LimitReader(resp.Body, maxDownloadBytes)

	pr := &callbackProgressReader{
		reader:     limitedBody,
		total:      resp.ContentLength,
		onProgress: onProgress,
	}

	written, copyErr := io.Copy(f, pr)
	closeErr := f.Close()
	if copyErr != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logger.Error("failed to remove temp file",
				"operation", "downloadModelWithProgress", "error", removeErr)
		}
		return fmt.Errorf("transcriber.downloadModelWithProgress: write: %w", copyErr)
	}
	if closeErr != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logger.Error("failed to remove temp file",
				"operation", "downloadModelWithProgress", "error", removeErr)
		}
		return fmt.Errorf("transcriber.downloadModelWithProgress: close: %w", closeErr)
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: rename: %w", err)
	}

	// TODO(v2): Add SHA256 checksum verification of downloaded model file.

	logger.Info("model downloaded", "operation", "downloadModelWithProgress", "bytes", written)
	return nil
}

type callbackProgressReader struct {
	reader     io.Reader
	total      int64
	written    int64
	onProgress DownloadProgressFunc
}

func (r *callbackProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.written += int64(n)
	if r.total > 0 && r.onProgress != nil {
		r.onProgress(float64(r.written)/float64(r.total), r.written, r.total)
	}
	return n, err
}
