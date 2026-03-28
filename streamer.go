package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"
)

// commonPrefixLen returns the byte length of the longest common rune prefix between a and b.
// Compares rune-by-rune to avoid splitting multi-byte UTF-8 codepoints.
func commonPrefixLen(a, b string) int {
	byteOffset := 0
	for byteOffset < len(a) && byteOffset < len(b) {
		ra, sza := utf8.DecodeRuneInString(a[byteOffset:])
		rb, szb := utf8.DecodeRuneInString(b[byteOffset:])
		if ra != rb || sza != szb {
			break
		}
		if ra == utf8.RuneError && sza == 1 {
			break // invalid UTF-8
		}
		byteOffset += sza
	}
	return byteOffset
}

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

	text, err := s.transcriber.Transcribe(audio)
	if err != nil {
		s.logger.Error("streaming transcription failed", "operation", "tick", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if text == s.lastText {
		return
	}

	// Diff: find common prefix, backspace old suffix, type new suffix
	prefixLen := commonPrefixLen(s.lastText, text)
	newSuffix := text[prefixLen:]
	oldRunes := []rune(s.lastText[prefixLen:])

	s.logger.Debug("streaming update", "operation", "tick",
		"prev_len", len(s.lastText), "new_len", len(text),
		"backspace", len(oldRunes), "type_len", len(newSuffix))

	if err := s.typer.ReplaceAll(len(oldRunes), newSuffix); err != nil {
		s.logger.Error("streaming type failed", "operation", "tick", "error", err)
		return
	}

	s.lastText = text
}

// Stop stops the streaming loop. Call Finalize after this.
func (s *Streamer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	<-s.done
	s.logger.Info("streaming stopped", "operation", "Stop")
}

// LastText returns the most recently streamed text (thread-safe).
func (s *Streamer) LastText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastText
}

// Finalize runs a final transcription on the complete audio and applies corrections.
// Returns the final text.
func (s *Streamer) Finalize(audio []float32) (string, error) {
	if len(audio) == 0 {
		return s.LastText(), nil
	}

	finalText, err := s.transcriber.Transcribe(audio)
	if err != nil {
		return "", fmt.Errorf("streamer.Finalize: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if finalText != s.lastText {
		prefixLen := commonPrefixLen(s.lastText, finalText)
		oldRunes := []rune(s.lastText[prefixLen:])
		newSuffix := finalText[prefixLen:]

		if err := s.typer.ReplaceAll(len(oldRunes), newSuffix); err != nil {
			return "", fmt.Errorf("streamer.Finalize: %w", err)
		}
	}

	s.logger.Info("finalized", "operation", "Finalize", "text_length", len(finalText))
	return finalText, nil
}
