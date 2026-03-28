package main

import (
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockRecorder struct {
	mu          sync.Mutex
	startCalled bool
	stopCalled  bool
	closeCalled bool
	audio       []float32
	startErr    error
	stopErr     error
}

func (m *mockRecorder) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	return m.startErr
}
func (m *mockRecorder) Stop() ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
	return m.audio, m.stopErr
}
func (m *mockRecorder) Snapshot() []float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.audio
}
func (m *mockRecorder) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

type mockTranscriber struct {
	mu            sync.Mutex
	text          string
	err           error
	closeCalled   bool
	receivedAudio []float32
}

func (m *mockTranscriber) Transcribe(audio []float32) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receivedAudio = audio
	return m.text, m.err
}
func (m *mockTranscriber) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

type mockPaster struct {
	mu         sync.Mutex
	pastedText string
	err        error
}

func (m *mockPaster) Paste(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pastedText = text
	return m.err
}

type mockTyper struct {
	mu         sync.Mutex
	typed      string
	backspaced int
}

func (m *mockTyper) Type(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typed += text
	return nil
}

func (m *mockTyper) Backspace(count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backspaced += count
	runes := []rune(m.typed)
	if count > len(runes) {
		count = len(runes)
	}
	m.typed = string(runes[:len(runes)-count])
	return nil
}

func (m *mockTyper) ReplaceAll(oldLen int, newText string) error {
	if err := m.Backspace(oldLen); err != nil {
		return err
	}
	return m.Type(newText)
}

func (m *mockTyper) getText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.typed
}

// --- Tests ---

func TestApp_HappyPath(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}
	trans := &mockTranscriber{text: "hello world"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, nil, snd, "clipboard", logger)

	var states []AppState
	var statesMu sync.Mutex
	app.SetStateCallback(func(s AppState) {
		statesMu.Lock()
		states = append(states, s)
		statesMu.Unlock()
	})

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
	// Wait for async transcription goroutine to complete
	time.Sleep(200 * time.Millisecond)

	// Shut down
	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	rec.mu.Lock()
	startCalled := rec.startCalled
	stopCalled := rec.stopCalled
	rec.mu.Unlock()

	if !startCalled {
		t.Error("recorder.Start was not called")
	}
	if !stopCalled {
		t.Error("recorder.Stop was not called")
	}

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "hello world" {
		t.Errorf("expected pasted text 'hello world', got %q", got)
	}

	// Verify state transitions: should start with Recording and end with Ready
	statesMu.Lock()
	if len(states) < 2 || states[0] != StateRecording || states[len(states)-1] != StateReady {
		t.Errorf("expected states [Recording...Ready], got %v", states)
	}
	statesMu.Unlock()
}

func TestApp_TranscriptionError_ContinuesListening(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{err: fmt.Errorf("whisper failed")}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, nil, snd, "clipboard", logger)

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
	// Wait for async transcription goroutine to complete
	time.Sleep(200 * time.Millisecond)

	// Second attempt -- should still work (continues listening)
	rec.mu.Lock()
	rec.audio = []float32{0.3, 0.4}
	rec.mu.Unlock()

	trans.mu.Lock()
	trans.err = nil
	trans.text = "recovered"
	trans.mu.Unlock()

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	// Wait for async transcription goroutine to complete
	time.Sleep(200 * time.Millisecond)

	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "recovered" {
		t.Errorf("expected pasted text 'recovered' after error recovery, got %q", got)
	}
}

func TestApp_EmptyAudio_NoPaste(t *testing.T) {
	rec := &mockRecorder{audio: nil}
	trans := &mockTranscriber{text: "should not be called"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, nil, snd, "clipboard", logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(200 * time.Millisecond)

	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "" {
		t.Errorf("expected no paste for empty audio, got %q", got)
	}
}

func TestApp_EmptyText_NoPaste(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{text: ""}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, nil, snd, "clipboard", logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})

	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	// Wait for async transcription goroutine to complete
	time.Sleep(200 * time.Millisecond)

	close(events)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after closing events channel")
	}

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "" {
		t.Errorf("expected no paste for empty transcription, got %q", got)
	}
}

func TestApp_Shutdown_ClosesBoth(t *testing.T) {
	rec := &mockRecorder{}
	trans := &mockTranscriber{}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, nil, snd, "clipboard", logger)
	app.Shutdown()

	rec.mu.Lock()
	recClosed := rec.closeCalled
	rec.mu.Unlock()
	if !recClosed {
		t.Error("recorder.Close was not called during Shutdown")
	}

	trans.mu.Lock()
	transClosed := trans.closeCalled
	trans.mu.Unlock()
	if !transClosed {
		t.Error("transcriber.Close was not called during Shutdown")
	}
}

func TestApp_StreamMode(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}
	trans := &mockTranscriber{text: "hello stream"}
	typer := &mockTyper{}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, typer, snd, "stream", logger)
	app.streamInterval = 50 * time.Millisecond

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})
	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerPressed
	time.Sleep(200 * time.Millisecond) // let streamer tick
	events <- TriggerReleased
	time.Sleep(200 * time.Millisecond) // wait for async finalize

	close(events)
	<-done

	got := typer.getText()
	if got == "" {
		t.Error("expected streamer to have typed text")
	}
}

func TestApp_StreamMode_ReleaseWithoutPress(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{text: "hello"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, nil, snd, "stream", logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})
	go func() {
		app.Run(events)
		close(done)
	}()

	// Release without press — should not crash or call recorder.Stop
	events <- TriggerReleased
	time.Sleep(50 * time.Millisecond)

	close(events)
	<-done

	rec.mu.Lock()
	stopped := rec.stopCalled
	rec.mu.Unlock()
	if stopped {
		t.Error("recorder.Stop should not be called on release without press")
	}
}

func TestApp_ClipboardMode_ReleaseWithoutPress(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{text: "hello"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, nil, snd, "clipboard", logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})
	go func() {
		app.Run(events)
		close(done)
	}()

	events <- TriggerReleased
	time.Sleep(50 * time.Millisecond)

	close(events)
	<-done

	rec.mu.Lock()
	stopped := rec.stopCalled
	rec.mu.Unlock()
	if stopped {
		t.Error("recorder.Stop should not be called on release without press")
	}
}
