package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const maxLogBytes int64 = 5 * 1024 * 1024 // 5MB

func SetupLogger(logDir string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: create dir: %w", err)
	}

	logPath := filepath.Join(logDir, "voicetype.log")

	if err := truncateIfNeeded(logPath, maxLogBytes); err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: truncate: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: open log file: %w", err)
	}

	level := slog.LevelInfo
	if os.Getenv("JOICE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(handler)
	cleanup := func() {
		f.Close()
	}

	return logger, cleanup, nil
}

func truncateIfNeeded(path string, maxBytes int64) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("logger.truncateIfNeeded: stat: %w", err)
	}
	if info.Size() <= maxBytes {
		return nil
	}

	// Keep the last 1MB of log data instead of destroying everything
	const keepBytes int64 = 1 * 1024 * 1024

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("logger.truncateIfNeeded: open: %w", err)
	}
	if _, err := f.Seek(-keepBytes, 2); err != nil {
		f.Close()
		// File smaller than keepBytes — shouldn't happen given size > maxBytes, but just truncate
		return os.Truncate(path, 0)
	}
	tail, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("logger.truncateIfNeeded: read tail: %w", err)
	}

	// Skip first partial line
	if idx := bytes.IndexByte(tail, '\n'); idx >= 0 {
		tail = tail[idx+1:]
	}

	return os.WriteFile(path, tail, 0644)
}
