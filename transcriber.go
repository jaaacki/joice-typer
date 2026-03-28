package main

/*
#cgo CFLAGS: -I${SRCDIR}/third_party/whisper.cpp/include -I${SRCDIR}/third_party/whisper.cpp/ggml/include
#cgo LDFLAGS: -L${SRCDIR}/third_party/whisper.cpp/build/src -L${SRCDIR}/third_party/whisper.cpp/build/ggml/src -L${SRCDIR}/third_party/whisper.cpp/build/ggml/src/ggml-metal -L${SRCDIR}/third_party/whisper.cpp/build/ggml/src/ggml-blas -lwhisper -lggml -lggml-base -lggml-cpu -lggml-metal -lggml-blas -lstdc++ -framework Accelerate -framework Metal -framework Foundation -framework CoreML
#include <whisper.h>
#include <stdlib.h>
*/
import "C"

import (
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

// ensureModel checks if the model file exists and downloads it if not.
func ensureModel(modelPath string, logger *slog.Logger) error {
	if _, err := os.Stat(modelPath); err == nil {
		logger.Info("model found", "operation", "ensureModel", "path", modelPath)
		return nil
	}

	// Extract model name from path (e.g., "ggml-small.bin" from the full path)
	modelFile := filepath.Base(modelPath)
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + modelFile

	logger.Warn("model not found, downloading from network",
		"operation", "ensureModel",
		"url", url,
		"dest", modelPath,
	)

	dir := filepath.Dir(modelPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("transcriber.ensureModel: create dir: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("transcriber.ensureModel: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("transcriber.ensureModel: download returned status %d", resp.StatusCode)
	}

	tmpPath := modelPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("transcriber.ensureModel: create file: %w", err)
	}

	pr := &progressWriter{
		reader: resp.Body,
		total:  resp.ContentLength,
		logger: logger,
	}

	written, copyErr := io.Copy(f, pr)
	closeErr := f.Close()
	if copyErr != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logger.Error("failed to remove temp file", "operation", "ensureModel", "error", removeErr)
		}
		return fmt.Errorf("transcriber.ensureModel: write: %w", copyErr)
	}
	if closeErr != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			logger.Error("failed to remove temp file", "operation", "ensureModel", "error", removeErr)
		}
		return fmt.Errorf("transcriber.ensureModel: close file: %w", closeErr)
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		return fmt.Errorf("transcriber.ensureModel: rename: %w", err)
	}

	// TODO(v2): Add SHA256 checksum verification of downloaded model file.

	logger.Info("model downloaded", "operation", "ensureModel", "bytes", written)
	return nil
}

type progressWriter struct {
	reader  io.Reader
	total   int64
	written int64
	logger  *slog.Logger
	lastPct int
}

func (pw *progressWriter) Read(p []byte) (int, error) {
	n, err := pw.reader.Read(p)
	pw.written += int64(n)
	if pw.total > 0 {
		pct := int(pw.written * 100 / pw.total)
		if pct/10 > pw.lastPct/10 {
			pw.logger.Info("downloading model",
				"operation", "ensureModel",
				"progress_pct", pct,
				"bytes_written", pw.written,
				"bytes_total", pw.total,
			)
			pw.lastPct = pct
		}
	}
	return n, err
}
