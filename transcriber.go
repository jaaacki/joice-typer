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
	inflight   chan struct{} // semaphore: capacity 1
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
		return nil, &ErrBadPayload{Component: "transcriber", Operation: "NewTranscriber", Detail: "model corrupt or incompatible"}
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
	return &whisperTranscriber{
		ctx: wctx, lang: language, sampleRate: sampleRate, logger: l,
		inflight: make(chan struct{}, 1),
	}, nil
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

	// Bulkhead: only 1 in-flight transcription. If a previous CGO call
	// is still running (timed out but goroutine alive), reject immediately
	// instead of stacking goroutines behind the mutex.
	select {
	case t.inflight <- struct{}{}:
		// acquired
	default:
		return "", &ErrDependencyTimeout{
			Component: "transcriber",
			Operation: "Transcribe",
			Wrapped:   fmt.Errorf("previous transcription still in flight"),
		}
	}

	ch := make(chan transcribeResult, 1)
	go func() {
		defer func() { <-t.inflight }() // release semaphore when done
		text, err := t.transcribeBlocking(audio)
		ch <- transcribeResult{text, err}
	}()

	select {
	case <-ctx.Done():
		// Don't release semaphore here — goroutine still running
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

	// Poll TryLock with a hard deadline. No helper goroutine, no race.
	// TryLock is non-blocking — if the mutex is held by a hung CGO call,
	// it returns false immediately. We poll every 100ms up to 5s.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if t.mu.TryLock() {
			if t.ctx != nil {
				C.whisper_free(t.ctx)
				t.ctx = nil
			}
			t.mu.Unlock()
			return nil
		}
		if time.Now().After(deadline) {
			t.logger.Error("close timed out waiting for transcription lock — whisper context leaked",
				"component", "transcriber", "operation", "Close")
			return &ErrDependencyTimeout{
				Component: "transcriber",
				Operation: "Close",
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// DownloadProgressFunc is called during model download with progress info.
type DownloadProgressFunc func(progress float64, bytesDownloaded, bytesTotal int64)

// validateCachedModel checks if a cached model file is valid.
// The pinned SHA-256 in modelManifest is the trusted root.
// A .sha256 sidecar file caches the result to avoid re-hashing on every startup.
// Returns true if the model is ready to use. Quarantines bad models.
func validateCachedModel(modelPath string, modelSize string, logger *slog.Logger) bool {
	l := logger.With("operation", "validateCachedModel")
	spec, ok := modelManifest[modelSize]
	if !ok {
		l.Error("unknown model size", "model_size", modelSize)
		return false
	}

	// Check file exists and size matches
	info, err := os.Stat(modelPath)
	if err != nil {
		return false
	}
	if info.Size() != spec.exactLen {
		l.Warn("model size mismatch", "expected", spec.exactLen, "actual", info.Size())
		os.Rename(modelPath, modelPath+".bad")
		return false
	}

	// Always hash the actual file — never trust the sidecar alone.
	// A tampered model with an intact sidecar must not pass verification.
	hashPath := modelPath + ".sha256"
	l.Info("hashing model file", "model_size", modelSize, "size", info.Size())
	f, err := os.Open(modelPath)
	if err != nil {
		l.Error("failed to open model for hashing", "error", err)
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		l.Error("failed to hash model", "error", err)
		return false
	}
	currentHash := hex.EncodeToString(h.Sum(nil))

	if currentHash != spec.sha256 {
		l.Warn("model hash mismatch — quarantining",
			"expected", spec.sha256, "actual", currentHash)
		os.Rename(modelPath, modelPath+".bad")
		os.Remove(hashPath)
		return false
	}

	// Hash matches — update sidecar cache
	if err := os.WriteFile(hashPath, []byte(currentHash), 0644); err != nil {
		l.Warn("failed to write hash cache", "error", err)
	}
	l.Info("model verified", "model_size", modelSize)
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

	// Use manifest's expected size as default; HEAD may refine it
	var expectedSize int64
	if spec, ok := modelManifest[modelSize]; ok {
		expectedSize = spec.exactLen
	}

	// HEAD preflight — advisory, not mandatory
	headReq, err := http.NewRequestWithContext(dlCtx, "HEAD", url, nil)
	if err == nil {
		headResp, headErr := client.Do(headReq)
		if headErr == nil {
			headResp.Body.Close()
			if headResp.StatusCode == http.StatusOK && headResp.ContentLength > 0 {
				expectedSize = headResp.ContentLength
			}
		}
		if headErr != nil {
			logger.Info("HEAD preflight failed, proceeding with GET",
				"component", "transcriber", "operation", "downloadModel", "error", headErr)
		}
	}

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
				return &ErrDependencyTimeout{Component: "transcriber", Operation: "downloadModel", Wrapped: dlCtx.Err()}
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
		return &ErrDependencyUnavailable{Component: "transcriber", Operation: "downloadModel", Wrapped: fmt.Errorf("all %d attempts failed: %w", downloadMaxRetries+1, lastErr)}
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
			logger.Error("size mismatch", "operation", "downloadModel",
				"expected", spec.exactLen, "actual", written)
			return &ErrBadPayload{Component: "transcriber", Operation: "downloadModel", Detail: "size mismatch"}
		}
		if hash != spec.sha256 {
			os.Remove(tmpPath)
			logger.Error("hash mismatch", "operation", "downloadModel",
				"expected", spec.sha256, "actual", hash)
			return &ErrBadPayload{Component: "transcriber", Operation: "downloadModel", Detail: "hash mismatch"}
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
		return &ErrDependencyUnavailable{Component: "transcriber", Operation: "doDownload", Wrapped: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	// If server doesn't support range and returns full body, start from scratch
	if startByte > 0 && resp.StatusCode == http.StatusOK {
		startByte = 0
	}

	// Validate Content-Range header on 206 to ensure server resumed correctly
	if startByte > 0 && resp.StatusCode == http.StatusPartialContent {
		cr := resp.Header.Get("Content-Range")
		expectedPrefix := fmt.Sprintf("bytes %d-", startByte)
		if !strings.HasPrefix(cr, expectedPrefix) {
			// Server didn't resume from where we asked — start over
			logger.Warn("Content-Range mismatch, restarting download",
				"component", "transcriber", "operation", "doDownload",
				"expected_prefix", expectedPrefix, "got", cr)
			resp.Body.Close()
			os.Remove(tmpPath)
			startByte = 0
			req2, err2 := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err2 != nil {
				return fmt.Errorf("restart GET: %w", err2)
			}
			resp, err = client.Do(req2)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
		}
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
		return &ErrBadPayload{Component: "transcriber", Operation: "doDownload", Detail: "download truncated"}
	}

	// Detect hitting the safety cap
	if totalWritten >= maxDownloadBytes {
		return &ErrBadPayload{Component: "transcriber", Operation: "doDownload", Detail: "exceeded download size limit"}
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
