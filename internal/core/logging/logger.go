package logging

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	maxLogBytes    int64 = 5 * 1024 * 1024 // 5MB per active log file
	rotatedKeep          = 2                // number of .1, .2 historical files to keep
	logFileName          = "voicetype.log"
	rotationCheckEvery   = 256              // check size every N writes (keeps fs.Stat out of hot path)
)

type WriteObserver func(path string)

var (
	writeObserversMu    sync.Mutex
	writeObservers      = map[int]WriteObserver{}
	nextWriteObserverID int
)

func SetupLogger(logDir string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: create dir: %w", err)
	}

	logPath := filepath.Join(logDir, logFileName)

	if err := truncateIfNeeded(logPath, maxLogBytes); err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: truncate: %w", err)
	}

	rw, err := newRotatingWriter(logPath, maxLogBytes, rotatedKeep)
	if err != nil {
		return nil, nil, fmt.Errorf("logger.SetupLogger: open log file: %w", err)
	}

	level := slog.LevelInfo
	if os.Getenv("JOICE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(notifyingWriter{Writer: rw, path: logPath}, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(handler)
	cleanup := func() {
		_ = rw.Close()
	}

	return logger, cleanup, nil
}

func RegisterWriteObserver(observer WriteObserver) func() {
	if observer == nil {
		return func() {}
	}
	writeObserversMu.Lock()
	id := nextWriteObserverID
	nextWriteObserverID++
	writeObservers[id] = observer
	writeObserversMu.Unlock()
	return func() {
		writeObserversMu.Lock()
		delete(writeObservers, id)
		writeObserversMu.Unlock()
	}
}

type notifyingWriter struct {
	io.Writer
	path string
}

func (w notifyingWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if n > 0 {
		notifyWriteObservers(w.path)
	}
	return n, err
}

func notifyWriteObservers(path string) {
	writeObserversMu.Lock()
	observers := make([]WriteObserver, 0, len(writeObservers))
	for _, observer := range writeObservers {
		observers = append(observers, observer)
	}
	writeObserversMu.Unlock()
	for _, observer := range observers {
		observer(path)
	}
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
