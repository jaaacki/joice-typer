package logging

import (
	"fmt"
	"os"
	"sync"
)

// rotatingWriter appends to logPath and, when the file grows past maxBytes,
// renames the current file to logPath.1 (shifting logPath.1 → logPath.2, etc.
// up to keep generations) and opens a fresh logPath. Writes are serialized by
// mu so rotation cannot race with concurrent writes.
//
// Rotation is checked every rotationCheckEvery writes, not on every write,
// to keep os.Stat out of the hot path.
type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int

	f            *os.File
	writesSince  int
	currentBytes int64
}

func newRotatingWriter(path string, maxBytes int64, keep int) (*rotatingWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &rotatingWriter{
		path:         path,
		maxBytes:     maxBytes,
		keep:         keep,
		f:            f,
		currentBytes: info.Size(),
	}, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.f.Write(p)
	w.currentBytes += int64(n)
	w.writesSince++
	if w.writesSince >= rotationCheckEvery || w.currentBytes > w.maxBytes {
		w.writesSince = 0
		if w.currentBytes > w.maxBytes {
			if rotErr := w.rotateLocked(); rotErr != nil {
				// Rotation failure is non-fatal: keep writing to the current file
				// rather than drop log lines. We log the failure to stderr (the
				// only channel we can use from inside the log writer without
				// recursing).
				fmt.Fprintf(os.Stderr, "log rotation failed: %v\n", rotErr)
			}
		}
	}
	return n, err
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

func (w *rotatingWriter) rotateLocked() error {
	// Close the current file first so the rename on Windows works and there's
	// no in-flight write on macOS at the moment of rename.
	if err := w.f.Close(); err != nil {
		return fmt.Errorf("close current: %w", err)
	}
	w.f = nil

	// Shift generations: .N-1 -> .N, ..., base -> .1. Missing sources are
	// tolerated (skip and continue).
	for i := w.keep; i >= 1; i-- {
		src := generationPath(w.path, i-1)
		dst := generationPath(w.path, i)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("stat %s: %w", src, err)
		}
		// Remove any stale dst first to make rename atomic cross-platform.
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", src, dst, err)
		}
	}

	// Open a fresh base file.
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("reopen base: %w", err)
	}
	w.f = f
	w.currentBytes = 0
	return nil
}

func generationPath(base string, gen int) string {
	if gen == 0 {
		return base
	}
	return fmt.Sprintf("%s.%d", base, gen)
}
