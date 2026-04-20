package transcription

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveDownloadStartByte_RemovesStalePartialDownload(t *testing.T) {
	tmpPath := filepath.Join(t.TempDir(), "ggml-medium.bin.tmp")
	if err := os.WriteFile(tmpPath, []byte("partial"), 0644); err != nil {
		t.Fatalf("write temp download: %v", err)
	}

	staleTime := time.Now().Add(-(downloadResumeStaleAfter + time.Minute))
	if err := os.Chtimes(tmpPath, staleTime, staleTime); err != nil {
		t.Fatalf("mark temp download stale: %v", err)
	}

	startByte, err := resolveDownloadStartByte(tmpPath, 1024, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("resolve download start byte: %v", err)
	}
	if startByte != 0 {
		t.Fatalf("expected stale temp download to restart from 0, got %d", startByte)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale temp download to be removed, got stat err=%v", err)
	}
}

func TestResolveDownloadStartByte_KeepsFreshPartialDownload(t *testing.T) {
	tmpPath := filepath.Join(t.TempDir(), "ggml-medium.bin.tmp")
	content := []byte("partial-download")
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		t.Fatalf("write temp download: %v", err)
	}

	startByte, err := resolveDownloadStartByte(tmpPath, 1024, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("resolve download start byte: %v", err)
	}
	if startByte != int64(len(content)) {
		t.Fatalf("expected fresh temp download to resume from %d, got %d", len(content), startByte)
	}
}

