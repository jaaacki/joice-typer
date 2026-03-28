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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// modelSpec defines expected properties for each whisper model size.
type modelSpec struct {
	minBytes int64
	sha256   string // empty = accept any; pin after verified download
}

// Minimum sizes are generous lower bounds — well below actual model sizes
// to avoid rejecting legitimate models while catching truncated downloads.
var modelManifest = map[string]modelSpec{
	"tiny":   {minBytes: 30 * 1024 * 1024},  // actual ~75MB
	"base":   {minBytes: 70 * 1024 * 1024},  // actual ~142MB
	"small":  {minBytes: 200 * 1024 * 1024}, // actual ~466MB
	"medium": {minBytes: 700 * 1024 * 1024}, // actual ~1.5GB
}

type whisperTranscriber struct {
	mu     sync.Mutex
	ctx    *C.struct_whisper_context
	lang   string
	logger *slog.Logger
}

func NewTranscriber(modelPath string, modelSize string, language string, logger *slog.Logger) (Transcriber, error) {
	l := logger.With("component", "transcriber")

	if err := ensureModel(modelPath, modelSize, l); err != nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: %w", err)
	}

	l.Info("loading model", "operation", "NewTranscriber", "model_path", modelPath)

	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	cparams := C.whisper_context_default_params()
	ctx := C.whisper_init_from_file_with_params(cPath, cparams)
	if ctx == nil {
		// Quarantine bad model so future startups re-download instead of failing forever
		badPath := modelPath + ".bad"
		if renameErr := os.Rename(modelPath, badPath); renameErr == nil {
			l.Warn("quarantined corrupt model", "operation", "NewTranscriber",
				"bad_path", filepath.Base(badPath))
		}
		return nil, fmt.Errorf("transcriber.NewTranscriber: model corrupt or incompatible")
	}

	// Validate language against whisper's own language list
	if language != "" {
		cLang := C.CString(language)
		defer C.free(unsafe.Pointer(cLang))
		if C.whisper_lang_id(cLang) < 0 {
			C.whisper_free(ctx)
			return nil, fmt.Errorf("transcriber.NewTranscriber: unsupported language %q", language)
		}
	}

	l.Info("model loaded", "operation", "NewTranscriber")
	return &whisperTranscriber{ctx: ctx, lang: language, logger: l}, nil
}

func (t *whisperTranscriber) Transcribe(audio []float32) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("transcriber.Transcribe: empty audio buffer")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ctx == nil {
		return "", fmt.Errorf("transcriber.Transcribe: transcriber is closed")
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
		cText := C.whisper_full_get_segment_text(t.ctx, C.int(i))
		if cText == nil {
			continue // skip nil segments instead of crashing
		}
		segments = append(segments, C.GoString(cText))
	}

	text := strings.TrimSpace(strings.Join(segments, ""))
	t.logger.Info("transcribed", "operation", "Transcribe",
		"segments", nSegments, "text_length", len(text))
	return text, nil
}

func (t *whisperTranscriber) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

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
func ensureModel(modelPath string, modelSize string, logger *slog.Logger) error {
	info, err := os.Stat(modelPath)
	if err == nil {
		// File exists — validate against per-model minimum size
		minBytes := int64(0)
		if spec, ok := modelManifest[modelSize]; ok {
			minBytes = spec.minBytes
		}
		if minBytes > 0 && info.Size() < minBytes {
			logger.Warn("model file appears truncated, re-downloading",
				"operation", "ensureModel",
				"size", info.Size(),
				"min_expected", minBytes,
				"model_size", modelSize,
			)
			if removeErr := os.Remove(modelPath); removeErr != nil {
				return fmt.Errorf("transcriber.ensureModel: remove truncated model: %w", removeErr)
			}
			// Fall through to download
		} else {
			logger.Info("model found", "operation", "ensureModel",
				"path", modelPath, "size", info.Size())
			return nil
		}
	}

	var lastPct int
	return downloadModelWithProgress(modelPath, modelSize, func(progress float64, downloaded, total int64) {
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
func downloadModelWithProgress(modelPath string, modelSize string, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	modelFile := filepath.Base(modelPath)
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + modelFile

	logger.Warn("downloading model from network",
		"operation", "downloadModelWithProgress", "model_size", modelSize)

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

	transport := &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Minute,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("transcriber.downloadModelWithProgress: HTTP %d", resp.StatusCode)
	}

	// Reject non-binary content types: HTML error pages, JSON errors, proxied junk
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "application/json") {
		return fmt.Errorf("transcriber.downloadModelWithProgress: unexpected content type %q", ct)
	}

	tmpPath := modelPath + ".tmp"
	os.Remove(tmpPath) // clean up any stale temp from a previous failed download

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: create file: %w", err)
	}

	// Cap download at 2GB to prevent abuse
	const maxDownloadBytes = 2 * 1024 * 1024 * 1024
	limitedBody := io.LimitReader(resp.Body, maxDownloadBytes)

	// Hash during download for integrity verification
	hashWriter := sha256.New()
	teedBody := io.TeeReader(limitedBody, hashWriter)

	pr := &callbackProgressReader{
		reader:     teedBody,
		total:      resp.ContentLength,
		onProgress: onProgress,
	}

	written, copyErr := io.Copy(f, pr)
	if syncErr := f.Sync(); syncErr != nil && copyErr == nil {
		copyErr = syncErr
	}
	closeErr := f.Close()

	if copyErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModelWithProgress: write: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModelWithProgress: close: %w", closeErr)
	}

	// Validate Content-Length match (detects truncation)
	if resp.ContentLength > 0 && written != resp.ContentLength {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModelWithProgress: truncated: got %d bytes, expected %d", written, resp.ContentLength)
	}

	// Detect hitting the 2GB safety cap (oversized stream or upstream lies about length)
	if written >= maxDownloadBytes {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModelWithProgress: exceeded %d byte limit", maxDownloadBytes)
	}

	// Validate against per-model minimum size
	if spec, ok := modelManifest[modelSize]; ok && written < spec.minBytes {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModelWithProgress: too small (%d bytes, minimum %d for %s)", written, spec.minBytes, modelSize)
	}

	hash := hex.EncodeToString(hashWriter.Sum(nil))

	// Verify SHA-256 if pinned in manifest
	if spec, ok := modelManifest[modelSize]; ok && spec.sha256 != "" {
		if hash != spec.sha256 {
			os.Remove(tmpPath)
			return fmt.Errorf("transcriber.downloadModelWithProgress: sha256 mismatch: got %s, expected %s", hash, spec.sha256)
		}
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: rename: %w", err)
	}

	logger.Info("model downloaded", "operation", "downloadModelWithProgress",
		"bytes", written, "sha256", hash)
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
