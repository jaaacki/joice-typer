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
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const (
	maxTranscribeSeconds  = 60               // reject audio longer than 60s
	maxTranscribeSegments = 500              // cap whisper segments to prevent runaway
	maxTranscribeBytes    = 50000            // cap output text to ~50KB
	transcribeTimeout     = 30 * time.Second // hard deadline for whisper_full

	downloadMaxRetries    = 3
	downloadRetryBaseWait = 2 * time.Second
	downloadMaxRedirects  = 5
	maxDownloadBytes      = 2 * 1024 * 1024 * 1024 // 2GB safety cap
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
	mu         sync.Mutex
	ctx        *C.struct_whisper_context
	lang       string
	sampleRate int
	logger     *slog.Logger
}

func NewTranscriber(ctx context.Context, modelPath string, modelSize string, language string, sampleRate int, logger *slog.Logger) (Transcriber, error) {
	l := logger.With("component", "transcriber")

	if err := ensureModel(ctx, modelPath, modelSize, l); err != nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: %w", err)
	}

	l.Info("loading model", "operation", "NewTranscriber", "model_path", modelPath)

	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	cparams := C.whisper_context_default_params()
	wctx := C.whisper_init_from_file_with_params(cPath, cparams)
	if wctx == nil {
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
			C.whisper_free(wctx)
			return nil, fmt.Errorf("transcriber.NewTranscriber: unsupported language %q", language)
		}
	}

	l.Info("model loaded", "operation", "NewTranscriber")
	return &whisperTranscriber{ctx: wctx, lang: language, sampleRate: sampleRate, logger: l}, nil
}

type transcribeResult struct {
	text string
	err  error
}

func (t *whisperTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("transcriber.Transcribe: empty audio buffer")
	}

	// Cap input audio duration using the actual configured sample rate
	rate := t.sampleRate
	if rate <= 0 {
		rate = 16000
	}
	maxSamples := rate * maxTranscribeSeconds
	if len(audio) > maxSamples {
		return "", &ErrBadPayload{
			Component: "transcriber",
			Operation: "Transcribe",
			Detail:    fmt.Sprintf("audio too long: %d samples (%ds at %dHz), max %ds", len(audio), len(audio)/rate, rate, maxTranscribeSeconds),
		}
	}

	// Apply default deadline if caller didn't set one
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, transcribeTimeout)
		defer cancel()
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

	n := int(C.whisper_full_n_segments(t.ctx))
	if n > maxTranscribeSegments {
		t.logger.Warn("segment count capped", "operation", "Transcribe", "actual", n, "max", maxTranscribeSegments)
		n = maxTranscribeSegments
	}

	var sb strings.Builder
	for i := 0; i < n; i++ {
		cText := C.whisper_full_get_segment_text(t.ctx, C.int(i))
		if cText == nil {
			continue // skip nil segments instead of crashing
		}
		sb.WriteString(C.GoString(cText))
		if sb.Len() > maxTranscribeBytes {
			t.logger.Warn("output size capped", "operation", "Transcribe", "bytes", sb.Len(), "max", maxTranscribeBytes)
			break
		}
	}

	text := strings.TrimSpace(sb.String())
	t.logger.Info("transcribed", "operation", "Transcribe",
		"segments", n, "text_length", len(text))
	return text, nil
}

func (t *whisperTranscriber) Close() error {
	t.logger.Info("closing", "operation", "Close")

	// TryLock with timeout — a hung transcribeBlocking holds the mutex
	// and we must not block shutdown forever waiting for CGO to return.
	// The timedOut flag coordinates between the helper goroutine and the
	// select: if the timeout fires first, the helper must unlock after
	// it eventually acquires the lock (otherwise the mutex is poisoned).
	var timedOut atomic.Bool
	locked := make(chan struct{})
	go func() {
		t.mu.Lock()
		if timedOut.Load() {
			// Timeout already fired — we acquired the lock too late.
			// Unlock immediately to avoid poisoning the mutex.
			t.mu.Unlock()
			return
		}
		close(locked)
	}()

	select {
	case <-locked:
		// Got the lock in time — safe to free
		if t.ctx != nil {
			C.whisper_free(t.ctx)
			t.ctx = nil
		}
		t.mu.Unlock()
	case <-time.After(5 * time.Second):
		timedOut.Store(true)
		t.logger.Error("close timed out waiting for transcription lock — whisper context leaked",
			"component", "transcriber", "operation", "Close")
		return &ErrDependencyTimeout{
			Component: "transcriber",
			Operation: "Close",
		}
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
func ensureModel(ctx context.Context, modelPath string, modelSize string, logger *slog.Logger) error {
	if validateCachedModel(modelPath, modelSize, logger) {
		return nil
	}

	var lastPct int
	return downloadModelWithProgress(ctx, modelPath, modelSize, func(progress float64, downloaded, total int64) {
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
func downloadModelWithProgress(ctx context.Context, modelPath string, modelSize string, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	modelFile := filepath.Base(modelPath)
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + modelFile

	logger.Warn("downloading model from network",
		"operation", "downloadModelWithProgress", "model_size", modelSize)

	dir := filepath.Dir(modelPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("transcriber.downloadModel: create dir: %w", err)
	}

	dlCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	transport := &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= downloadMaxRedirects {
				return fmt.Errorf("too many redirects (%d)", len(via))
			}
			return nil
		},
	}

	// HEAD preflight — verify remote file exists and get expected size
	headReq, err := http.NewRequestWithContext(dlCtx, "HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("transcriber.downloadModel: HEAD request: %w", err)
	}
	headResp, err := client.Do(headReq)
	if err != nil {
		return &ErrDependencyUnavailable{Component: "transcriber", Operation: "downloadModel", Wrapped: err}
	}
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		return &ErrDependencyUnavailable{
			Component: "transcriber",
			Operation: "downloadModel",
			Wrapped:   fmt.Errorf("HEAD returned %d", headResp.StatusCode),
		}
	}
	expectedSize := headResp.ContentLength
	if expectedSize > maxDownloadBytes {
		return &ErrBadPayload{
			Component: "transcriber",
			Operation: "downloadModel",
			Detail:    fmt.Sprintf("remote file too large: %d bytes, max %d", expectedSize, maxDownloadBytes),
		}
	}

	tmpPath := modelPath + ".tmp"

	// Retry loop with jittered exponential backoff
	var lastErr error
	for attempt := 0; attempt <= downloadMaxRetries; attempt++ {
		if attempt > 0 {
			// Jittered exponential backoff
			wait := downloadRetryBaseWait * time.Duration(1<<(attempt-1))
			jitter := time.Duration(rand.Int63n(int64(wait / 2)))
			select {
			case <-dlCtx.Done():
				return dlCtx.Err()
			case <-time.After(wait + jitter):
			}
			logger.Info("retrying download",
				"component", "transcriber", "operation", "downloadModel",
				"attempt", attempt+1, "max", downloadMaxRetries+1)
		}

		lastErr = doDownload(dlCtx, client, url, tmpPath, expectedSize, onProgress, logger)
		if lastErr == nil {
			break // success
		}
		logger.Warn("download attempt failed",
			"component", "transcriber", "operation", "downloadModel",
			"attempt", attempt+1, "error", lastErr)
	}
	if lastErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModel: all %d attempts failed: %w", downloadMaxRetries+1, lastErr)
	}

	// Hash the completed download for integrity verification
	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModel: open for hash: %w", err)
	}
	hashWriter := sha256.New()
	written, err := io.Copy(hashWriter, tmpFile)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("transcriber.downloadModel: hash: %w", err)
	}
	hash := hex.EncodeToString(hashWriter.Sum(nil))

	// Verify downloaded content against pinned manifest (trusted root)
	if spec, ok := modelManifest[modelSize]; ok {
		if written != spec.exactLen {
			os.Remove(tmpPath)
			return fmt.Errorf("transcriber.downloadModel: size mismatch: got %d bytes, expected %d", written, spec.exactLen)
		}
		if hash != spec.sha256 {
			os.Remove(tmpPath)
			return fmt.Errorf("transcriber.downloadModel: sha256 mismatch: got %s, expected %s", hash, spec.sha256)
		}
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		return fmt.Errorf("transcriber.downloadModel: rename: %w", err)
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

// doDownload performs a single GET download attempt with range resume support.
// It writes to tmpPath and verifies the size matches expectedSize.
func doDownload(ctx context.Context, client *http.Client, url string, tmpPath string, expectedSize int64, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	var startByte int64
	if info, err := os.Stat(tmpPath); err == nil {
		startByte = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if startByte > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
		logger.Info("resuming download", "component", "transcriber", "operation", "doDownload", "from_byte", startByte)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept 200 (full) or 206 (partial)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("GET returned %d", resp.StatusCode)
	}

	// Reject non-binary content types: HTML error pages, JSON errors, proxied junk
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "application/json") {
		return fmt.Errorf("unexpected content type %q", ct)
	}

	// If server doesn't support range and returns full body, start from scratch
	if startByte > 0 && resp.StatusCode == http.StatusOK {
		startByte = 0
	}

	var f *os.File
	if startByte > 0 && resp.StatusCode == http.StatusPartialContent {
		f, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND, 0644)
	} else {
		f, err = os.Create(tmpPath)
	}
	if err != nil {
		return err
	}

	// Cap download at maxDownloadBytes to prevent abuse
	limitedBody := io.LimitReader(resp.Body, maxDownloadBytes-startByte)

	totalSize := expectedSize
	if totalSize <= 0 {
		totalSize = resp.ContentLength
	}

	pr := &callbackProgressReader{
		reader:     limitedBody,
		total:      totalSize,
		written:    startByte,
		onProgress: onProgress,
	}

	n, copyErr := io.Copy(f, pr)
	if syncErr := f.Sync(); syncErr != nil && copyErr == nil {
		copyErr = syncErr
	}
	closeErr := f.Close()

	if copyErr != nil {
		return fmt.Errorf("write: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close: %w", closeErr)
	}

	totalWritten := startByte + n

	// Validate total size matches expected (detects truncation)
	if expectedSize > 0 && totalWritten != expectedSize {
		return fmt.Errorf("truncated: got %d bytes, expected %d", totalWritten, expectedSize)
	}

	// Detect hitting the safety cap
	if totalWritten >= maxDownloadBytes {
		return fmt.Errorf("exceeded %d byte limit", maxDownloadBytes)
	}

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
