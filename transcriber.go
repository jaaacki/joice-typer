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
// SHA-256 hashes are pinned from the Git LFS OIDs on huggingface.co/ggerganov/whisper.cpp.
// These are the trusted root — not derived from downloaded content.
type modelSpec struct {
	sha256   string // pinned content hash (Git LFS OID)
	exactLen int64  // expected file size in bytes
}

var modelManifest = map[string]modelSpec{
	"tiny":   {sha256: "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21", exactLen: 77691713},
	"base":   {sha256: "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe", exactLen: 147951465},
	"small":  {sha256: "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b", exactLen: 487601967},
	"medium": {sha256: "6c14d5adee5f86394037b4e4e8b59f1673b6cee10e3cf0b11bbdbee79c156208", exactLen: 1533763059},
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

type transcribeResult struct {
	text string
	err  error
}

func (t *whisperTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("transcriber.Transcribe: empty audio buffer")
	}

	ch := make(chan transcribeResult, 1)
	go func() {
		text, err := t.transcribeBlocking(audio)
		ch <- transcribeResult{text, err}
	}()

	select {
	case <-ctx.Done():
		// CGO call still running but we return immediately.
		// The goroutine will complete eventually and release the mutex.
		return "", &ErrDependencyTimeout{
			Component: "transcriber",
			Operation: "Transcribe",
			Wrapped:   ctx.Err(),
		}
	case result := <-ch:
		return result.text, result.err
	}
}

func (t *whisperTranscriber) transcribeBlocking(audio []float32) (string, error) {
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

// validateCachedModel checks if a cached model file is valid.
// The pinned SHA-256 in modelManifest is the trusted root.
// A .sha256 sidecar file caches the result to avoid re-hashing on every startup.
// Returns true if the model is ready to use. Quarantines bad models.
func validateCachedModel(modelPath string, modelSize string, logger *slog.Logger) bool {
	spec, hasSpec := modelManifest[modelSize]
	info, err := os.Stat(modelPath)
	if err != nil {
		return false // file doesn't exist
	}

	// Exact size validation against manifest
	if hasSpec && info.Size() != spec.exactLen {
		logger.Warn("model file size mismatch",
			"operation", "validateCachedModel",
			"size", info.Size(), "expected", spec.exactLen, "model_size", modelSize)
		os.Remove(modelPath)
		return false
	}

	if !hasSpec {
		// Unknown model size — no manifest entry, cannot verify
		logger.Warn("no manifest entry for model size, cannot verify",
			"operation", "validateCachedModel", "model_size", modelSize)
		return false
	}

	// Fast path: if sidecar hash matches the pinned manifest hash, skip re-hashing.
	// The sidecar is a cache, not an authority — the manifest is the trusted root.
	hashPath := modelPath + ".sha256"
	if savedHash, readErr := os.ReadFile(hashPath); readErr == nil {
		if strings.TrimSpace(string(savedHash)) == spec.sha256 {
			logger.Info("model verified (cached hash matches manifest)",
				"operation", "validateCachedModel", "sha256", spec.sha256)
			return true
		}
		// Sidecar disagrees with manifest — fall through to full hash
		logger.Warn("cached hash does not match manifest, re-verifying",
			"operation", "validateCachedModel")
	}

	// Slow path: compute hash from file content, verify against pinned manifest
	f, openErr := os.Open(modelPath)
	if openErr != nil {
		logger.Error("cannot open model for hash verification",
			"operation", "validateCachedModel", "error", openErr)
		return false
	}
	h := sha256.New()
	if _, copyErr := io.Copy(h, f); copyErr != nil {
		f.Close()
		logger.Error("failed to hash model",
			"operation", "validateCachedModel", "error", copyErr)
		return false
	}
	f.Close()
	currentHash := hex.EncodeToString(h.Sum(nil))

	if currentHash != spec.sha256 {
		logger.Warn("model hash does not match pinned manifest, quarantining",
			"operation", "validateCachedModel",
			"pinned", spec.sha256, "got", currentHash)
		badPath := modelPath + ".bad"
		os.Rename(modelPath, badPath)
		os.Remove(hashPath)
		return false
	}

	// Hash matches manifest — cache it for future fast-path
	if writeErr := os.WriteFile(hashPath, []byte(currentHash), 0644); writeErr != nil {
		logger.Error("failed to write hash cache file",
			"operation", "validateCachedModel", "error", writeErr)
	}

	logger.Info("model verified (full hash against manifest)",
		"operation", "validateCachedModel", "sha256", currentHash)
	return true
}

// ensureModel checks if the model file exists and downloads it if not.
func ensureModel(modelPath string, modelSize string, logger *slog.Logger) error {
	if validateCachedModel(modelPath, modelSize, logger) {
		return nil
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

	hash := hex.EncodeToString(hashWriter.Sum(nil))

	// Verify downloaded content against pinned manifest (trusted root)
	if spec, ok := modelManifest[modelSize]; ok {
		if written != spec.exactLen {
			os.Remove(tmpPath)
			return fmt.Errorf("transcriber.downloadModelWithProgress: size mismatch: got %d bytes, expected %d", written, spec.exactLen)
		}
		if hash != spec.sha256 {
			os.Remove(tmpPath)
			return fmt.Errorf("transcriber.downloadModelWithProgress: sha256 mismatch: got %s, expected %s", hash, spec.sha256)
		}
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		return fmt.Errorf("transcriber.downloadModelWithProgress: rename: %w", err)
	}

	// Cache hash for fast-path verification on future startups
	hashPath := modelPath + ".sha256"
	if writeErr := os.WriteFile(hashPath, []byte(hash), 0644); writeErr != nil {
		logger.Error("failed to write hash cache",
			"operation", "downloadModelWithProgress", "error", writeErr)
	}

	logger.Info("model downloaded and verified", "operation", "downloadModelWithProgress",
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
