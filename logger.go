package main

import (
	"fmt"
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
	if info.Size() > maxBytes {
		if err := os.Truncate(path, 0); err != nil {
			return fmt.Errorf("logger.truncateIfNeeded: truncate: %w", err)
		}
	}
	return nil
}
