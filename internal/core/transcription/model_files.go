package transcription

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
	"time"

	apppkg "voicetype/internal/core/runtime"
)

const (
	downloadMaxRetries    = 3
	downloadRetryBaseWait = 2 * time.Second
	downloadMaxRedirects  = 5
	maxDownloadBytes      = 2 * 1024 * 1024 * 1024 // 2GB safety cap
	downloadResumeStaleAfter = 2 * time.Minute
)

// modelSpec defines expected properties for each whisper model size.
// SHA-256 hashes are pinned from the Git LFS OIDs on huggingface.co/ggerganov/whisper.cpp.
// These are the trusted root — not derived from downloaded content.
type modelSpec struct {
	sha256   string
	exactLen int64
}

var modelManifest = map[string]modelSpec{
	// Multilingual models — support all languages + translation
	"tiny":   {sha256: "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21", exactLen: 77691713},
	"base":   {sha256: "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe", exactLen: 147951465},
	"small":  {sha256: "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b", exactLen: 487601967},
	"medium": {sha256: "6c14d5adee5f86394037b4e4e8b59f1673b6cee10e3cf0b11bbdbee79c156208", exactLen: 1533763059},
	// English-only models — higher accuracy for English, no translation support
	"tiny.en":   {sha256: "921e4cf8686fdd993dcd081a5da5b6c365bfde1162e72b08d75ac75289920b1f", exactLen: 77704715},
	"base.en":   {sha256: "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002", exactLen: 147964211},
	"small.en":  {sha256: "c6138d6d58ecc8322097e0f987c32f1be8bb0a18532a3f88f734d1bbf9c41e5d", exactLen: 487614201},
	"medium.en": {sha256: "cc37e93478338ec7700281a7ac30a10128929eb8f427dda2e865faa8f6da4356", exactLen: 1533774781},
}

var (
	renameFile = os.Rename
	removeFile = os.Remove
)

// DownloadProgressFunc is called during model download with progress info.
type DownloadProgressFunc func(progress float64, bytesDownloaded, bytesTotal int64)

func DownloadModelWithProgress(ctx context.Context, modelPath string, modelSize string, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	return downloadModelWithProgress(ctx, modelPath, modelSize, onProgress, logger)
}

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

	info, err := os.Stat(modelPath)
	if err != nil {
		return false
	}
	if info.Size() != spec.exactLen {
		l.Warn("model size mismatch", "expected", spec.exactLen, "actual", info.Size())
		quarantineModel(modelPath, modelPath+".sha256", l, "validateCachedModel")
		return false
	}

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
		l.Warn("model hash mismatch — quarantining", "expected", spec.sha256, "actual", currentHash)
		quarantineModel(modelPath, hashPath, l, "validateCachedModel")
		return false
	}

	if err := os.WriteFile(hashPath, []byte(currentHash), 0644); err != nil {
		l.Warn("failed to write hash cache", "error", err)
	}
	l.Info("model verified", "model_size", modelSize)
	return true
}

func quarantineModel(modelPath string, hashPath string, logger *slog.Logger, operation string) {
	badPath := modelPath + ".bad"
	if err := renameFile(modelPath, badPath); err != nil {
		logger.Warn("failed to quarantine bad model", "operation", operation, "model_path", modelPath, "bad_path", badPath, "error", err)
	} else {
		logger.Warn("quarantined bad model", "operation", operation, "bad_path", filepath.Base(badPath))
	}
	if hashPath != "" {
		if err := removeFile(hashPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to remove model hash cache", "operation", operation, "hash_path", hashPath, "error", err)
		}
	}
}

func ensureModel(ctx context.Context, modelPath string, modelSize string, logger *slog.Logger) error {
	if validateCachedModel(modelPath, modelSize, logger) {
		return nil
	}

	var lastPct int
	return downloadModelWithProgress(ctx, modelPath, modelSize, func(progress float64, downloaded, total int64) {
		pct := int(progress * 100)
		if pct/10 > lastPct/10 {
			logger.Info("downloading model", "operation", "ensureModel", "progress_pct", pct, "bytes_written", downloaded, "bytes_total", total)
			lastPct = pct
		}
	}, logger)
}

func downloadModelWithProgress(ctx context.Context, modelPath string, modelSize string, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	modelFile := filepath.Base(modelPath)
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + modelFile

	logger.Warn("downloading model from network", "operation", "downloadModelWithProgress", "model_size", modelSize)

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
	allowedHosts := map[string]bool{
		"huggingface.co":              true,
		"cdn-lfs.huggingface.co":      true,
		"cdn-lfs-us-1.huggingface.co": true,
		"cdn-lfs-eu-1.huggingface.co": true,
		"cas-bridge.xethub.hf.co":     true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= downloadMaxRedirects {
				return fmt.Errorf("too many redirects (%d)", len(via))
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-HTTPS scheme %q", req.URL.Scheme)
			}
			if !allowedHosts[req.URL.Hostname()] {
				return fmt.Errorf("redirect to untrusted host %q", req.URL.Hostname())
			}
			return nil
		},
	}

	spec, ok := modelManifest[modelSize]
	if !ok {
		return &apppkg.ErrBadPayload{Component: "transcriber", Operation: "downloadModel", Detail: fmt.Sprintf("unknown model size %q — no manifest entry", modelSize)}
	}
	expectedSize := spec.exactLen

	headReq, err := http.NewRequestWithContext(dlCtx, "HEAD", url, nil)
	if err == nil {
		headResp, headErr := client.Do(headReq)
		if headErr == nil {
			headResp.Body.Close()
			if headResp.StatusCode == http.StatusOK && headResp.ContentLength > 0 && headResp.ContentLength != expectedSize {
				logger.Warn("HEAD Content-Length differs from manifest, using manifest", "component", "transcriber", "operation", "downloadModel", "head_size", headResp.ContentLength, "manifest_size", expectedSize)
			}
		}
		if headErr != nil {
			logger.Info("HEAD preflight failed, proceeding with GET", "component", "transcriber", "operation", "downloadModel", "error", headErr)
		}
	}

	if expectedSize > maxDownloadBytes {
		return &apppkg.ErrBadPayload{Component: "transcriber", Operation: "downloadModel", Detail: fmt.Sprintf("remote file too large: %d bytes, max %d", expectedSize, maxDownloadBytes)}
	}

	tmpPath := modelPath + ".tmp"

	var lastErr error
	for attempt := 0; attempt <= downloadMaxRetries; attempt++ {
		if attempt > 0 {
			wait := downloadRetryBaseWait * time.Duration(1<<(attempt-1))
			jitter := time.Duration(rand.Int63n(int64(wait / 2)))
			select {
			case <-dlCtx.Done():
				return &apppkg.ErrDependencyTimeout{Component: "transcriber", Operation: "downloadModel", Wrapped: dlCtx.Err()}
			case <-time.After(wait + jitter):
			}
			logger.Info("retrying download", "component", "transcriber", "operation", "downloadModel", "attempt", attempt+1, "max", downloadMaxRetries+1)
		}

		lastErr = doDownload(dlCtx, client, url, tmpPath, expectedSize, onProgress, logger)
		if lastErr == nil {
			break
		}
		logger.Warn("download attempt failed", "component", "transcriber", "operation", "downloadModel", "attempt", attempt+1, "error", lastErr)
	}
	if lastErr != nil {
		os.Remove(tmpPath)
		return &apppkg.ErrDependencyUnavailable{Component: "transcriber", Operation: "downloadModel", Wrapped: fmt.Errorf("all %d attempts failed: %w", downloadMaxRetries+1, lastErr)}
	}

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

	if spec, ok := modelManifest[modelSize]; ok {
		if written != spec.exactLen {
			os.Remove(tmpPath)
			logger.Error("size mismatch", "operation", "downloadModel", "expected", spec.exactLen, "actual", written)
			return &apppkg.ErrBadPayload{Component: "transcriber", Operation: "downloadModel", Detail: "size mismatch"}
		}
		if hash != spec.sha256 {
			os.Remove(tmpPath)
			logger.Error("hash mismatch", "operation", "downloadModel", "expected", spec.sha256, "actual", hash)
			return &apppkg.ErrBadPayload{Component: "transcriber", Operation: "downloadModel", Detail: "hash mismatch"}
		}
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		return fmt.Errorf("transcriber.downloadModel: rename: %w", err)
	}

	hashPath := modelPath + ".sha256"
	if dlInfo, statErr := os.Stat(modelPath); statErr == nil {
		sidecar := fmt.Sprintf("%s:%d:%d", hash, dlInfo.Size(), dlInfo.ModTime().Unix())
		if writeErr := os.WriteFile(hashPath, []byte(sidecar), 0644); writeErr != nil {
			logger.Error("failed to write hash cache", "operation", "downloadModelWithProgress", "error", writeErr)
		}
	}

	logger.Info("model downloaded and verified", "operation", "downloadModelWithProgress", "bytes", written, "sha256", hash)
	return nil
}

func doDownload(ctx context.Context, client *http.Client, url string, tmpPath string, expectedSize int64, onProgress DownloadProgressFunc, logger *slog.Logger) error {
	startByte, err := resolveDownloadStartByte(tmpPath, expectedSize, logger)
	if err != nil {
		return err
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return &apppkg.ErrDependencyUnavailable{Component: "transcriber", Operation: "doDownload", Wrapped: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	if startByte > 0 && resp.StatusCode == http.StatusOK {
		startByte = 0
	}

	if startByte > 0 && resp.StatusCode == http.StatusPartialContent {
		cr := resp.Header.Get("Content-Range")
		expectedPrefix := fmt.Sprintf("bytes %d-", startByte)
		if !strings.HasPrefix(cr, expectedPrefix) {
			logger.Warn("Content-Range mismatch, restarting download", "component", "transcriber", "operation", "doDownload", "expected_prefix", expectedPrefix, "got", cr)
			resp.Body.Close()
			os.Remove(tmpPath)
			startByte = 0
			req2, err2 := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err2 != nil {
				return fmt.Errorf("transcriber.doDownload: restart request: %w", err2)
			}
			resp, err = client.Do(req2)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return &apppkg.ErrDependencyUnavailable{Component: "transcriber", Operation: "doDownload", Wrapped: fmt.Errorf("restart GET returned HTTP %d", resp.StatusCode)}
			}
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

	readLimit := expectedSize - startByte
	if readLimit <= 0 {
		readLimit = maxDownloadBytes
	}
	limitedBody := io.LimitReader(resp.Body, readLimit+1)

	totalSize := expectedSize
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
		return fmt.Errorf("transcriber.doDownload: write tmp: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("transcriber.doDownload: close tmp: %w", closeErr)
	}

	totalWritten := startByte + n
	if totalWritten > expectedSize {
		return &apppkg.ErrBadPayload{Component: "transcriber", Operation: "doDownload", Detail: "upstream sent more bytes than manifest expects"}
	}
	if expectedSize > 0 && totalWritten != expectedSize {
		return &apppkg.ErrBadPayload{Component: "transcriber", Operation: "doDownload", Detail: "download truncated"}
	}

	return nil
}

func resolveDownloadStartByte(tmpPath string, expectedSize int64, logger *slog.Logger) (int64, error) {
	info, err := os.Stat(tmpPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("transcriber.resolveDownloadStartByte: stat tmp: %w", err)
	}

	startByte := info.Size()
	if startByte <= 0 {
		return 0, nil
	}

	if startByte >= expectedSize {
		logger.Warn("stale .tmp file is already >= expected size, deleting", "component", "transcriber", "operation", "resolveDownloadStartByte", "tmp_size", startByte, "expected", expectedSize)
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("transcriber.resolveDownloadStartByte: remove oversized tmp: %w", err)
		}
		return 0, nil
	}

	if downloadResumeStaleAfter > 0 && time.Since(info.ModTime()) > downloadResumeStaleAfter {
		logger.Warn("stale partial download found, restarting from scratch", "component", "transcriber", "operation", "resolveDownloadStartByte", "tmp_size", startByte, "stale_after", downloadResumeStaleAfter.String(), "tmp_age", time.Since(info.ModTime()).String())
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("transcriber.resolveDownloadStartByte: remove stale tmp: %w", err)
		}
		return 0, nil
	}

	return startByte, nil
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
