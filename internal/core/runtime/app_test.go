package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockRecorder struct {
	mu              sync.Mutex
	startCalled     bool
	stopCalled      bool
	closeCalled     bool
	refreshCalled   bool
	markStaleCalled bool
	audio           []float32
	startErr        error
	stopErr         error
	refreshErr      error
	startFn         func(ctx context.Context) error
	stopFn          func() ([]float32, error)
	refreshFn       func() error
	markStaleFn     func(reason string)
}

func (m *mockRecorder) Warm() {}
func (m *mockRecorder) RefreshDevices() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshCalled = true
	if m.refreshFn != nil {
		return m.refreshFn()
	}
	return m.refreshErr
}
func (m *mockRecorder) MarkStale(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markStaleCalled = true
	if m.markStaleFn != nil {
		m.markStaleFn(reason)
	}
}
func (m *mockRecorder) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	if m.startFn != nil {
		return m.startFn(ctx)
	}
	return m.startErr
}
func (m *mockRecorder) Stop() ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
	if m.stopFn != nil {
		return m.stopFn()
	}
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
	transcribeFn  func(ctx context.Context, audio []float32) (string, error)
}

func (m *mockTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	m.mu.Lock()
	fn := m.transcribeFn
	m.receivedAudio = audio
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, audio)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text, m.err
}
func (m *mockTranscriber) SetVocabulary(_ string) {}
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

// --- Tests ---

func TestApp_HappyPath(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2, 0.3}}
	trans := &mockTranscriber{text: "hello world"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

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

func TestFormatPasteText_AppendsSpaceAfterSentence(t *testing.T) {
	if got := formatPasteText("Hello world."); got != "Hello world. " {
		t.Fatalf("expected trailing separator after sentence, got %q", got)
	}
}

func TestFormatPasteText_PreservesRawTextWithoutTrailingSpace(t *testing.T) {
	if got := formatPasteText("ls -la"); got != "ls -la" {
		t.Fatalf("expected raw text without forced trailing space, got %q", got)
	}
}

func TestFormatPasteText_DoesNotDoubleAppendAfterTrailingWhitespace(t *testing.T) {
	if got := formatPasteText("Hello world. "); got != "Hello world. " {
		t.Fatalf("expected existing trailing separator to be preserved, got %q", got)
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

func TestApp_TranscribeDrop_ResetsStateToReady(t *testing.T) {
	rec := &mockRecorder{}
	trans := &mockTranscriber{}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec, trans, paste, snd, logger)

	var states []AppState
	var statesMu sync.Mutex
	app.SetStateCallback(func(s AppState) {
		statesMu.Lock()
		states = append(states, s)
		statesMu.Unlock()
	})

	// Simulate the release path having already moved the UI into Transcribing,
	// then losing the transcription because another job already owns the busy flag.
	app.emitState(StateTranscribing)
	atomic.StoreInt32(&app.busy, 1)
	app.wg.Add(1)
	app.transcribeAndPaste([]float32{0.1})

	statesMu.Lock()
	defer statesMu.Unlock()
	if len(states) < 2 {
		t.Fatalf("expected at least 2 state transitions, got %v", states)
	}
	if states[len(states)-1] != StateReady {
		t.Fatalf("expected final state Ready after dropped transcription, got %v", states[len(states)-1])
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

func TestApp_ReleaseWithoutPress(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	trans := &mockTranscriber{text: "hello"}
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

func TestApp_TranscriberTimeout_DoesNotHang(t *testing.T) {
	// Simulate a NON-COOPERATIVE hung CGO call — ignores ctx entirely,
	// blocks for 120 seconds. This is what a real stuck whisper_full does.
	hangingTranscriber := &mockTranscriber{
		transcribeFn: func(_ context.Context, audio []float32) (string, error) {
			time.Sleep(120 * time.Second) // does NOT check ctx.Done()
			return "should not reach", nil
		},
	}

	rec := &mockRecorder{audio: make([]float32, 16000)}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)
	app := NewApp(rec, hangingTranscriber, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	go app.Run(events)

	// Press and release to trigger transcription
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased

	// Wait for the transcription goroutine to start and get stuck
	time.Sleep(200 * time.Millisecond)

	// Shutdown should NOT hang forever — app.Shutdown has a 10s wg timeout
	// and transcriber.Close has a 5s TryLock timeout. Total worst case ~15s.
	done := make(chan struct{})
	go func() {
		close(events)
		app.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// Good — shutdown completed despite non-cooperative transcriber
	case <-time.After(20 * time.Second):
		t.Fatal("Shutdown hung with hanging transcriber — expected timeout")
	}
}

func TestApp_IsIdle_FalseWhileBusy(t *testing.T) {
	blockCh := make(chan struct{})
	transcriber := &mockTranscriber{
		transcribeFn: func(ctx context.Context, audio []float32) (string, error) {
			<-blockCh
			return "text", nil
		},
	}

	recorder := &mockRecorder{audio: make([]float32, 16000)}
	sound := NewSound(false, slog.Default())
	app := NewApp(recorder, transcriber, &mockPaster{}, sound, slog.Default())

	if !app.IsIdle() {
		t.Fatal("expected idle initially")
	}

	events := make(chan HotkeyEvent, 10)
	go app.Run(events)

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	// busy flag should be set while transcriber is blocked
	if app.IsIdle() {
		t.Fatal("expected not idle while transcribing")
	}

	close(blockCh)
	time.Sleep(300 * time.Millisecond)

	if !app.IsIdle() {
		t.Fatal("expected idle after transcription completes")
	}

	close(events)
	app.Shutdown()
}

func TestApp_SetRecorder_WhileIdle(t *testing.T) {
	rec1 := &mockRecorder{audio: []float32{0.1}}
	rec2 := &mockRecorder{audio: []float32{0.9}}
	trans := &mockTranscriber{text: "from rec2"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)

	app := NewApp(rec1, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	done := make(chan struct{})
	go func() {
		app.Run(events)
		close(done)
	}()

	// Swap recorder while idle
	app.SetRecorder(rec2)

	// Press+release should use rec2
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(200 * time.Millisecond)

	close(events)
	<-done

	rec1.mu.Lock()
	r1Started := rec1.startCalled
	rec1.mu.Unlock()

	rec2.mu.Lock()
	r2Started := rec2.startCalled
	rec2.mu.Unlock()

	if r1Started {
		t.Error("rec1.Start should NOT have been called after SetRecorder(rec2)")
	}
	if !r2Started {
		t.Error("rec2.Start should have been called after SetRecorder(rec2)")
	}

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "from rec2" {
		t.Errorf("expected 'from rec2', got %q", got)
	}
}

func TestApp_RecorderStartFails_ContinuesListening(t *testing.T) {
	var callCount int
	var countMu sync.Mutex

	rec := &mockRecorder{
		audio: make([]float32, 16000),
		startFn: func(ctx context.Context) error {
			countMu.Lock()
			callCount++
			n := callCount
			countMu.Unlock()
			if n == 1 {
				return fmt.Errorf("device busy")
			}
			return nil
		},
	}
	trans := &mockTranscriber{
		transcribeFn: func(ctx context.Context, audio []float32) (string, error) {
			return "hello", nil
		},
	}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)
	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	go app.Run(events)

	// First press — recorder.Start fails
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	// Second press — should succeed
	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(300 * time.Millisecond)

	close(events)
	app.Shutdown()

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "hello" {
		t.Errorf("expected 'hello' after retry, got %q", got)
	}
}

func TestApp_RecorderStartFailure_AutoRefreshAndRetry(t *testing.T) {
	var startCalls int

	rec := &mockRecorder{
		audio: []float32{0.1, 0.2, 0.3},
		startFn: func(ctx context.Context) error {
			startCalls++
			if startCalls == 1 {
				return &ErrDependencyUnavailable{
					Component: "recorder",
					Operation: "Start",
					Wrapped:   fmt.Errorf("Internal PortAudio error"),
				}
			}
			return nil
		},
	}
	trans := &mockTranscriber{text: "recovered"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)
	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	go app.Run(events)

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(300 * time.Millisecond)

	close(events)
	app.Shutdown()

	rec.mu.Lock()
	refreshCalled := rec.refreshCalled
	rec.mu.Unlock()

	if !refreshCalled {
		t.Fatal("expected recorder refresh after dependency-unavailable start failure")
	}
	if startCalls != 2 {
		t.Fatalf("expected 2 recorder start attempts, got %d", startCalls)
	}

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "recovered" {
		t.Fatalf("expected recovered transcription after retry, got %q", got)
	}
}

func TestApp_RecorderStartFailure_RefreshFails_RemainsUsable(t *testing.T) {
	var startCalls int
	var refreshCalls int

	rec := &mockRecorder{
		audio: []float32{0.1, 0.2, 0.3},
		startFn: func(ctx context.Context) error {
			startCalls++
			if startCalls == 1 {
				return &ErrDependencyUnavailable{
					Component: "recorder",
					Operation: "Start",
					Wrapped:   fmt.Errorf("Internal PortAudio error"),
				}
			}
			return nil
		},
		refreshFn: func() error {
			refreshCalls++
			return fmt.Errorf("refresh failed")
		},
	}
	trans := &mockTranscriber{text: "later success"}
	paste := &mockPaster{}
	logger := slog.Default()
	snd := NewSound(false, logger)
	app := NewApp(rec, trans, paste, snd, logger)

	events := make(chan HotkeyEvent, 10)
	go app.Run(events)

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(150 * time.Millisecond)

	events <- TriggerPressed
	time.Sleep(50 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(300 * time.Millisecond)

	close(events)
	app.Shutdown()

	if refreshCalls != 1 {
		t.Fatalf("expected 1 refresh attempt, got %d", refreshCalls)
	}
	if startCalls != 2 {
		t.Fatalf("expected 2 total start attempts across both presses, got %d", startCalls)
	}

	paste.mu.Lock()
	got := paste.pastedText
	paste.mu.Unlock()
	if got != "later success" {
		t.Fatalf("expected later press to still succeed, got %q", got)
	}
}

func TestApp_RecorderStartFailure_RefreshFails_EmitsDependencyStuck(t *testing.T) {
	rec := &mockRecorder{
		startFn: func(ctx context.Context) error {
			return &ErrDependencyUnavailable{
				Component: "recorder",
				Operation: "Start",
				Wrapped:   fmt.Errorf("Internal PortAudio error"),
			}
		},
		refreshFn: func() error {
			return fmt.Errorf("refresh failed")
		},
	}
	logger := slog.Default()
	snd := NewSound(false, logger)
	app := NewApp(rec, &mockTranscriber{text: "unused"}, &mockPaster{}, snd, logger)

	var states []AppState
	var statesMu sync.Mutex
	app.SetStateCallback(func(s AppState) {
		statesMu.Lock()
		states = append(states, s)
		statesMu.Unlock()
	})

	events := make(chan HotkeyEvent, 10)
	go app.Run(events)

	events <- TriggerPressed
	time.Sleep(100 * time.Millisecond)

	close(events)
	app.Shutdown()

	statesMu.Lock()
	defer statesMu.Unlock()
	if len(states) == 0 {
		t.Fatal("expected at least one state emission")
	}
	if states[len(states)-1] != StateDependencyStuck {
		t.Fatalf("expected final state %v, got %v", StateDependencyStuck, states[len(states)-1])
	}
}

func TestApp_RecorderStartFailure_DoesNotEmitRecordingState(t *testing.T) {
	rec := &mockRecorder{
		startFn: func(ctx context.Context) error {
			return &ErrDependencyUnavailable{
				Component: "recorder",
				Operation: "Start",
				Wrapped:   fmt.Errorf("Internal PortAudio error"),
			}
		},
		refreshFn: func() error {
			return fmt.Errorf("refresh failed")
		},
	}
	logger := slog.Default()
	snd := NewSound(false, logger)
	app := NewApp(rec, &mockTranscriber{text: "unused"}, &mockPaster{}, snd, logger)

	var states []AppState
	var statesMu sync.Mutex
	app.SetStateCallback(func(s AppState) {
		statesMu.Lock()
		states = append(states, s)
		statesMu.Unlock()
	})

	events := make(chan HotkeyEvent, 10)
	go app.Run(events)

	events <- TriggerPressed
	time.Sleep(100 * time.Millisecond)

	close(events)
	app.Shutdown()

	statesMu.Lock()
	defer statesMu.Unlock()
	for _, state := range states {
		if state == StateRecording {
			t.Fatalf("did not expect recording state on failed start, got %v", states)
		}
	}
}
