package main

import (
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestCommonPrefixLen(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"hello", "hello world", 5},
		{"abc", "xyz", 0},
		{"I need to shed", "I need to schedule", 11},
		{"", "anything", 0},
		{"same", "same", 4},
		{"café", "cafö", 3},                   // diverge at multi-byte accented char
		{"日本語", "日本人", 6},                  // CJK: 2 shared 3-byte chars
		{"hello 😀", "hello 😂", 6},            // emoji divergence after ASCII prefix
		{"naïve", "naïvety", len("naïve")},    // full prefix is multi-byte substring
	}
	for _, tt := range tests {
		got := commonPrefixLen(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonPrefixLen(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

type sequenceTranscriber struct {
	mu      sync.Mutex
	results []string
	callIdx int
}

func (s *sequenceTranscriber) Transcribe(audio []float32) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callIdx >= len(s.results) {
		return s.results[len(s.results)-1], nil
	}
	text := s.results[s.callIdx]
	s.callIdx++
	return text, nil
}

func (s *sequenceTranscriber) Close() error { return nil }

func TestStreamer_ProgressiveCorrection(t *testing.T) {
	typer := &mockTyper{}
	trans := &sequenceTranscriber{results: []string{
		"I need to",
		"I need to shed",
		"I need to schedule a meeting",
	}}
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}

	logger := testLogger()
	s := NewStreamer(trans, typer, rec, logger, 50*time.Millisecond)
	s.Start()

	time.Sleep(500 * time.Millisecond)
	s.Stop()

	got := typer.getText()
	if got != "I need to schedule a meeting" {
		t.Errorf("expected final text 'I need to schedule a meeting', got %q", got)
	}
}

func TestStreamer_Finalize(t *testing.T) {
	typer := &mockTyper{}
	trans := &sequenceTranscriber{results: []string{
		"hello worl",
		"hello world and goodbye",
	}}
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}

	logger := testLogger()
	s := NewStreamer(trans, typer, rec, logger, 50*time.Millisecond)
	s.Start()

	time.Sleep(80 * time.Millisecond)
	s.Stop()

	finalText, err := s.Finalize([]float32{0.1, 0.2, 0.3})
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if finalText != "hello world and goodbye" {
		t.Errorf("expected 'hello world and goodbye', got %q", finalText)
	}

	got := typer.getText()
	if got != finalText {
		t.Errorf("typed text %q != final text %q", got, finalText)
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
