package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

const clipboardTranscribeTimeout = 90 * time.Second

// App is the orchestrator that wires hotkey events to the
// record -> transcribe -> paste pipeline.
type App struct {
	recorder      Recorder
	transcriber   Transcriber
	paster        Paster
	componentMu   sync.RWMutex // guards recorder, transcriber, and paster
	sound         *Sound
	logger        *slog.Logger
	baseLogger    *slog.Logger
	busy          int32 // atomic flag: 1 = transcribing/finalizing
	recording     int32 // atomic flag: 1 = recording in progress
	stateMu       sync.RWMutex
	onStateChange func(AppState)
	wg            sync.WaitGroup
}

// NewApp creates an App with all components pre-constructed.
func NewApp(
	recorder Recorder,
	transcriber Transcriber,
	paster Paster,
	sound *Sound,
	logger *slog.Logger,
) *App {
	return &App{
		recorder:      recorder,
		transcriber:   transcriber,
		paster:        paster,
		sound:         sound,
		baseLogger:    logger,
		logger:        logger.With("component", "app"),
		onStateChange: func(AppState) {}, // no-op default
	}
}

// Run processes hotkey events until the events channel is closed.
func (a *App) Run(events <-chan HotkeyEvent) {
	a.logger.Info("event loop started", "operation", "Run")

	for event := range events {
		switch event {
		case TriggerPressed:
			a.handlePress()
		case TriggerReleased:
			a.handleRelease()
		}
	}

	a.logger.Info("event loop stopped", "operation", "Run")
}

// SetStateCallback sets a function called on every state transition.
func (a *App) SetStateCallback(fn func(AppState)) {
	a.stateMu.Lock()
	a.onStateChange = fn
	a.stateMu.Unlock()
}

// emitState calls the state callback safely under a read lock.
func (a *App) emitState(state AppState) {
	a.stateMu.RLock()
	fn := a.onStateChange
	a.stateMu.RUnlock()
	fn(state)
}

func (a *App) handlePress() {
	pressTime := time.Now()
	if atomic.LoadInt32(&a.busy) == 1 {
		a.logger.Warn("still transcribing, ignoring press",
			"operation", "handlePress")
		a.sound.PlayError()
		return
	}
	a.logger.Debug("trigger pressed", "operation", "handlePress")

	a.componentMu.RLock()
	rec := a.recorder
	a.componentMu.RUnlock()

	startErr := rec.Start(context.Background())
	var depErr *ErrDependencyUnavailable
	if errors.As(startErr, &depErr) {
		if recovered, retryErr := a.tryRecoverRecorderAndRetry(rec); recovered {
			startErr = retryErr
		}
	}
	if startErr != nil {
		a.logger.Error("failed to start recording",
			"operation", "handlePress", "error", startErr)
		a.sound.PlayError()
		a.emitState(a.failureState(startErr))
		return
	}

	a.logger.Debug("recording started after press",
		"operation", "handlePress",
		"press_to_record_ms", time.Since(pressTime).Milliseconds())

	a.sound.PlayStart()
	a.emitState(StateRecording)
	atomic.StoreInt32(&a.recording, 1)
}

func (a *App) tryRecoverRecorderAndRetry(rec Recorder) (bool, error) {
	refreshErr := rec.RefreshDevices()
	if refreshErr != nil {
		a.logger.Error("failed to refresh recorder after start failure",
			"component", "app", "operation", "tryRecoverRecorderAndRetry", "error", refreshErr)
		return true, &ErrDependencyUnavailable{
			Component: "app",
			Operation: "tryRecoverRecorderAndRetry",
			Wrapped:   refreshErr,
		}
	}

	retryErr := rec.Start(context.Background())
	if retryErr != nil {
		a.logger.Error("recorder start retry failed after refresh",
			"component", "app", "operation", "tryRecoverRecorderAndRetry", "error", retryErr)
		return true, retryErr
	}

	a.logger.Info("recorder recovered after refresh",
		"component", "app", "operation", "tryRecoverRecorderAndRetry")
	return true, nil
}

func (a *App) failureState(err error) AppState {
	var depUnavailable *ErrDependencyUnavailable
	var depTimeout *ErrDependencyTimeout
	switch {
	case errors.As(err, &depUnavailable), errors.As(err, &depTimeout):
		return StateDependencyStuck
	default:
		return StateReady
	}
}

func (a *App) handleRelease() {
	if atomic.LoadInt32(&a.recording) == 0 {
		a.logger.Debug("not recording, ignoring release", "operation", "handleRelease")
		return
	}
	a.logger.Debug("trigger released", "operation", "handleRelease")
	a.handleReleaseClipboard()
}

func (a *App) handleReleaseClipboard() {
	a.componentMu.RLock()
	rec := a.recorder
	a.componentMu.RUnlock()

	audio, err := rec.Stop()
	atomic.StoreInt32(&a.recording, 0)
	if err != nil {
		a.logger.Error("failed to stop recording",
			"operation", "handleReleaseClipboard", "error", err)
		a.sound.PlayError()
		a.emitState(a.failureState(err))
		return
	}

	a.sound.PlayStop()
	a.emitState(StateTranscribing)

	if len(audio) == 0 {
		a.logger.Warn("no audio captured", "operation", "handleReleaseClipboard")
		a.emitState(StateReady)
		return
	}

	// Process transcription async so event loop stays responsive
	a.wg.Add(1)
	go a.transcribeAndPaste(audio)
}

func (a *App) transcribeAndPaste(audio []float32) {
	defer a.wg.Done()

	a.componentMu.RLock()
	trans := a.transcriber
	pst := a.paster
	a.componentMu.RUnlock()

	if !atomic.CompareAndSwapInt32(&a.busy, 0, 1) {
		a.logger.Warn("transcription already in progress — audio from this session discarded",
			"component", "app", "operation", "transcribeAndPaste")
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}
	defer atomic.StoreInt32(&a.busy, 0)

	transcribeCtx, cancel := context.WithTimeout(context.Background(), clipboardTranscribeTimeout)
	defer cancel()
	text, err := trans.Transcribe(transcribeCtx, audio)
	if err != nil {
		var timeoutErr *ErrDependencyTimeout
		if errors.As(err, &timeoutErr) {
			a.logger.Error("transcription timed out — speech engine may be stuck",
				"component", "app", "operation", "transcribeAndPaste", "error", err)
			// Show dependency-stuck state — distinct from permission issues
			a.emitState(StateDependencyStuck)
			time.Sleep(2 * time.Second)
		} else {
			a.logger.Error("transcription failed",
				"operation", "transcribeAndPaste", "error", err)
		}
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	if text == "" {
		a.logger.Warn("no speech detected", "operation", "transcribeAndPaste")
		a.emitState(StateReady)
		return
	}

	// Add a separator only for sentence-like output; raw text should pass through unchanged.
	if err := pst.Paste(formatPasteText(text)); err != nil {
		a.logger.Error("paste failed",
			"operation", "transcribeAndPaste", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	a.logger.Info("text pasted", "operation", "transcribeAndPaste",
		"text_length", len(text))
	a.emitState(StateReady)
}

func formatPasteText(text string) string {
	trimmed := strings.TrimRightFunc(text, unicode.IsSpace)
	if trimmed == "" {
		return text
	}
	if hasTerminalPunctuation(trimmed) {
		if trimmed != text {
			return text
		}
		return text + " "
	}
	return text
}

// IsIdle returns true if no recording, transcription, or native CGO work is in flight.
// Checks both the app-level busy flag AND the transcriber's bulkhead semaphore.
func (a *App) IsIdle() bool {
	if atomic.LoadInt32(&a.recording) != 0 || atomic.LoadInt32(&a.busy) != 0 {
		return false
	}
	a.componentMu.RLock()
	trans := a.transcriber
	a.componentMu.RUnlock()
	// Probe the transcriber bulkhead — if we can acquire and release, it's idle
	if t, ok := trans.(interface{ IsInflight() bool }); ok {
		if t.IsInflight() {
			return false
		}
	}
	return true
}

// SetRecorder replaces the recorder used by the app.
// Must only be called when no recording is in progress.
func (a *App) SetRecorder(r Recorder) {
	a.componentMu.Lock()
	a.recorder = r
	a.componentMu.Unlock()
}

// SetTranscriber replaces the transcriber used by the app.
// Must only be called when no transcription is in progress.
func (a *App) SetTranscriber(t Transcriber) {
	a.componentMu.Lock()
	a.transcriber = t
	a.componentMu.Unlock()
}

// Shutdown gracefully closes all components.
// Waits for in-flight background goroutines before freeing resources.
func (a *App) Shutdown() {
	a.logger.Info("shutting down, waiting for in-flight work",
		"component", "app", "operation", "Shutdown")

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info("shutdown complete", "component", "app", "operation", "Shutdown")
	case <-time.After(10 * time.Second):
		a.logger.Error("shutdown timed out, some goroutines may be leaked",
			"component", "app", "operation", "Shutdown")
	}

	a.componentMu.RLock()
	rec := a.recorder
	trans := a.transcriber
	a.componentMu.RUnlock()

	if rec != nil {
		if err := rec.Close(); err != nil {
			a.logger.Error("failed to close recorder",
				"component", "app", "operation", "Shutdown", "error", err)
		}
	}
	if trans != nil {
		if err := trans.Close(); err != nil {
			a.logger.Error("failed to close transcriber",
				"component", "app", "operation", "Shutdown", "error", err)
		}
	}
}
