package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const streamTranscribeTimeout = 15 * time.Second

// Streamer runs a periodic transcription loop during recording,
// streaming partial results to the cursor via a Typer.
type Streamer struct {
	transcriber Transcriber
	typer       Typer
	recorder    Recorder
	logger      *slog.Logger
	interval    time.Duration

	mu       sync.Mutex
	lastText string
	running  bool
	stopCh   chan struct{}
	done     chan struct{}
}

func NewStreamer(
	transcriber Transcriber,
	typer Typer,
	recorder Recorder,
	logger *slog.Logger,
	interval time.Duration,
) *Streamer {
	return &Streamer{
		transcriber: transcriber,
		typer:       typer,
		recorder:    recorder,
		logger:      logger.With("component", "streamer"),
		interval:    interval,
	}
}

func (s *Streamer) Start() {
	s.mu.Lock()
	s.lastText = ""
	s.running = true
	s.stopCh = make(chan struct{})
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.loop()
	s.logger.Info("streaming started", "operation", "Start", "interval", s.interval)
}

func (s *Streamer) loop() {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Streamer) tick() {
	audio := s.recorder.Snapshot()
	if len(audio) == 0 {
		return
	}

	transcribeCtx, cancel := context.WithTimeout(context.Background(), streamTranscribeTimeout)
	defer cancel()
	text, err := s.transcriber.Transcribe(transcribeCtx, audio)
	if err != nil {
		var timeoutErr *ErrDependencyTimeout
		if errors.As(err, &timeoutErr) {
			// Previous transcription still running — skip this tick
			return
		}
		s.logger.Error("streaming transcription failed", "operation", "tick", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if text == s.lastText {
		return
	}

	// Whisper owns the cursor: clear previous output, type full new text.
	// Whisper naturally self-corrects as it gets more audio context.
	oldRunes := len([]rune(s.lastText))

	s.logger.Debug("streaming update", "operation", "tick",
		"clear", oldRunes, "type", len(text))

	if oldRunes > 0 {
		if err := s.typer.Backspace(oldRunes); err != nil {
			s.logger.Error("streaming backspace failed", "operation", "tick", "error", err)
			return
		}
	}

	if err := s.typer.Type(text); err != nil {
		s.logger.Error("streaming type failed", "operation", "tick", "error", err)
		return
	}

	s.lastText = text
}

// Stop stops the streaming loop.
func (s *Streamer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)

	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		s.logger.Error("streamer stop timed out", "operation", "Stop")
	}

	s.logger.Info("streaming stopped", "operation", "Stop")
}

// LastText returns the most recently streamed text (thread-safe).
func (s *Streamer) LastText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastText
}

