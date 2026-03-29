package main

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type sequenceTranscriber struct {
	mu      sync.Mutex
	results []string
	callIdx int
}

func (s *sequenceTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
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

func TestStreamer_WhisperOwnsContent(t *testing.T) {
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
		t.Errorf("expected 'I need to schedule a meeting', got %q", got)
	}
}

func TestStreamer_EmptyAudio(t *testing.T) {
	typer := &mockTyper{}
	trans := &sequenceTranscriber{results: []string{"hello"}}
	rec := &mockRecorder{audio: nil}

	logger := testLogger()
	s := NewStreamer(trans, typer, rec, logger, 50*time.Millisecond)
	s.Start()

	time.Sleep(200 * time.Millisecond)
	s.Stop()

	got := typer.getText()
	if got != "" {
		t.Errorf("expected no text for empty audio, got %q", got)
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
