package main

import (
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

	// Create a file larger than 1KB (using small limit for testing)
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
