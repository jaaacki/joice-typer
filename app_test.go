package main

import (
	"fmt"
	"log/slog"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockRecorder struct {
	startCalled bool
	stopCalled  bool
	closeCalled bool
	audio       []float32
	startErr    error
	stopErr     error
}

func (m *mockRecorder) Start() error {
	m.startCalled = true
	return m.startErr
}
func (m *mockRecorder) Stop() ([]float32, error) {
	m.stopCalled = true
	return m.audio, m.stopErr
}
func (m *mockRecorder) Close() error {
	m.closeCalled = true
	return nil
}

type mockTranscriber struct {
	text          string
	err           error
	closeCalled   bool
	receivedAudio []float32
}

func (m *mockTranscriber) Transcribe(audio []float32) (string, error) {
	m.receivedAudio = audio
	return m.text, m.err
}
func (m *mockTranscriber) Close() error {
	m.closeCalled = true
	return nil
}

type mockPaster struct {
	pastedText string
	err        error
}

func (m *mockPaster) Paste(text string) error {
	m.pastedText = text
	return m.err
}

// --- Tests ---

func TestApp_HappyPath(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}
	trans := &mockTranscriber{text: "hello world"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	// Simulate press -> release
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	// Shut down
	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	if !rec.startCalled {
		t.Error("recorder.Start was not called")
	}
	if !rec.stopCalled {
		t.Error("recorder.Stop was not called")
	}
	if paste.pastedText != "hello world" {
		t.Errorf("expected pasted text 'hello world', got %q", paste.pastedText)
	}
}

func TestApp_TranscriptionError_ContinuesListening(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{err: fmt.Errorf("whisper failed")}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	// First attempt -- will fail transcription
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	// Second attempt -- should still work (continues listening)
	rec.audio = []float32{0.3, 0.4}
	trans.err = nil
	trans.text = "recovered"

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	if paste.pastedText != "recovered" {
		t.Errorf("expected pasted text 'recovered' after error recovery, got %q", paste.pastedText)
	}
}

func TestApp_EmptyAudio_NoPaste(t *testing.T) {
	rec := &mockRecorder{audio: nil}
	trans := &mockTranscriber{text: "should not be called"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	if paste.pastedText != "" {
		t.Errorf("expected no paste for empty audio, got %q", paste.pastedText)
	}
}

func TestApp_EmptyText_NoPaste(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{text: ""}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	if paste.pastedText != "" {
		t.Errorf("expected no paste for empty transcription, got %q", paste.pastedText)
	}
}

func TestApp_Shutdown_ClosesBoth(t *testing.T) {
	rec := &mockRecorder{}
	trans := &mockTranscriber{}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)
	app.Shutdown()

	if !rec.closeCalled {
		t.Error("recorder.Close was not called during Shutdown")
	}
	if !trans.closeCalled {
		t.Error("transcriber.Close was not called during Shutdown")
	}
}
