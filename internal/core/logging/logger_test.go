package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupLogger_CreatesLogFile(t *testing.T) {
	dir := t.TempDir()

	logger, cleanup, err := SetupLogger(dir)
	if err != nil {
		t.Fatalf("SetupLogger: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("SetupLogger returned nil logger")
	}

	logPath := filepath.Join(dir, "voicetype.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("log file not created at %s", logPath)
	}
}

func TestSetupLogger_WritesToFile(t *testing.T) {
	dir := t.TempDir()

	logger, cleanup, err := SetupLogger(dir)
	if err != nil {
		t.Fatalf("SetupLogger: %v", err)
	}
	defer cleanup()

	logger.Info("test message", "component", "test", "operation", "TestWrite")

	logPath := filepath.Join(dir, "voicetype.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("log file is empty after writing")
	}
	if !strings.Contains(content, "test message") {
		t.Errorf("log file missing message, got: %s", content)
	}
	if !strings.Contains(content, `"component"`) {
		t.Errorf("log file missing component field, got: %s", content)
	}
}

func TestTruncateIfNeeded_TruncatesLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Create a file larger than 1KB but smaller than keepBytes (1MB).
	// Since file < keepBytes, Seek fails and it falls back to full truncate.
	data := make([]byte, 2048)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := truncateIfNeeded(path, 1024); err != nil {
		t.Fatalf("truncateIfNeeded: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("expected truncated file to be 0 bytes, got %d", info.Size())
	}
}

func TestTruncateIfNeeded_KeepsTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Build a file larger than keepBytes (1MB) so tail rotation kicks in.
	// First: 1.5MB of "old" lines, then a known "new" tail section.
	const keepBytes = 1024 * 1024 // must match the const in logger.go
	oldLines := bytes.Repeat([]byte("old log line\n"), (keepBytes+500000)/13)
	newLines := bytes.Repeat([]byte("new log line\n"), keepBytes/13)
	data := append(oldLines, newLines...)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// maxBytes smaller than file size to trigger rotation
	if err := truncateIfNeeded(path, int64(len(data)-1)); err != nil {
		t.Fatalf("truncateIfNeeded: %v", err)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}

	// Result should be smaller than original
	if int64(len(result)) >= int64(len(data)) {
		t.Errorf("expected rotated file to be smaller than original (%d), got %d", len(data), len(result))
	}

	// Result should be roughly keepBytes (minus partial first line)
	if int64(len(result)) < keepBytes-100 {
		t.Errorf("expected rotated file to be ~%d bytes, got %d", keepBytes, len(result))
	}

	// Result should contain "new log line" entries
	if !bytes.Contains(result, []byte("new log line")) {
		t.Error("rotated file missing expected tail content")
	}

	// Result should start at a line boundary (no partial first line)
	if len(result) > 0 && result[0] == '\n' {
		t.Error("rotated file starts with newline, should start at line boundary")
	}
}

func TestTruncateIfNeeded_LeavesSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	data := []byte("small content")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := truncateIfNeeded(path, 1024); err != nil {
		t.Fatalf("truncateIfNeeded: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(data)) {
		t.Errorf("expected file size %d, got %d", len(data), info.Size())
	}
}

func TestTruncateIfNeeded_NonexistentFile(t *testing.T) {
	err := truncateIfNeeded("/nonexistent/path/file.log", 1024)
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}
}
