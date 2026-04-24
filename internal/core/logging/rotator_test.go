package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterRotatesWhenExceeded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	w, err := newRotatingWriter(path, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Write enough to exceed 64 bytes -> should rotate at least once
	line := []byte(strings.Repeat("x", 40) + "\n")
	for i := 0; i < 5; i++ {
		if _, err := w.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()

	// After rotation, expect base + .1 (and possibly .2) to exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("base file missing: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("first rotation file missing: %v", err)
	}
}

func TestRotatingWriterKeepsGenerations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	w, err := newRotatingWriter(path, 40, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	line := []byte(strings.Repeat("y", 50) + "\n")
	for i := 0; i < 8; i++ {
		if _, err := w.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf(".1 should exist: %v", err)
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Errorf(".2 should exist: %v", err)
	}
	// .3 should not exist — keep=2
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf(".3 should NOT exist (keep=2): %v", err)
	}
}

func TestRotatingWriterPreservesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	w, err := newRotatingWriter(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	payload := bytes.Repeat([]byte("a"), 40)
	payload = append(payload, '\n')
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("content mismatch: got %d bytes, expected %d", len(content), len(payload))
	}
}
