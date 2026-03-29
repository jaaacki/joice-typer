package main

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const clipboardTranscribeTimeout = 30 * time.Second

// App is the orchestrator that wires hotkey events to the
// record -> transcribe -> paste pipeline.
type App struct {
	recorder       Recorder
	transcriber    Transcriber
	paster         Paster
	typer          Typer
	sound          *Sound
	logger         *slog.Logger
	baseLogger     *slog.Logger
	busy           int32 // atomic flag: 1 = transcribing/finalizing
	stateMu        sync.RWMutex
	onStateChange  func(AppState)
	typeMode       string
	recording      bool
	streamInterval time.Duration
	streamer       *Streamer
	wg             sync.WaitGroup
}

// NewApp creates an App with all components pre-constructed.
func NewApp(
	recorder Recorder,
	transcriber Transcriber,
	paster Paster,
	typer Typer,
	sound *Sound,
	typeMode string,
	logger *slog.Logger,
) *App {
	return &App{
		recorder:       recorder,
		transcriber:    transcriber,
		paster:         paster,
		typer:          typer,
		sound:          sound,
		baseLogger:     logger,
		logger:         logger.With("component", "app"),
		onStateChange:  func(AppState) {}, // no-op default
		typeMode:       typeMode,
		streamInterval: 1 * time.Second,
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
	if atomic.LoadInt32(&a.busy) == 1 {
		a.logger.Warn("still transcribing, ignoring press",
			"operation", "handlePress")
		a.sound.PlayError()
		return
	}
	a.logger.Debug("trigger pressed", "operation", "handlePress")
	a.sound.PlayStart()
	a.emitState(StateRecording)

	if err := a.recorder.Start(context.Background()); err != nil {
		a.logger.Error("failed to start recording",
			"operation", "handlePress", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	a.recording = true

	if a.typeMode == "stream" {
		a.streamer = NewStreamer(a.transcriber, a.typer, a.recorder, a.baseLogger, a.streamInterval)
		a.streamer.Start()
	}
}

func (a *App) handleRelease() {
	if !a.recording {
		a.logger.Debug("not recording, ignoring release", "operation", "handleRelease")
		return
	}
	a.logger.Debug("trigger released", "operation", "handleRelease")

	if a.typeMode == "stream" {
		a.handleReleaseStream()
	} else {
		a.handleReleaseClipboard()
	}
}

func (a *App) handleReleaseStream() {
	if !a.recording {
		a.logger.Debug("not recording, ignoring release", "operation", "handleReleaseStream")
		return
	}
	a.recording = false
	a.sound.PlayStop()

	lastText := ""
	if a.streamer != nil {
		lastText = a.streamer.LastText()
		a.streamer.Stop()
		a.streamer = nil
	}

	audio, err := a.recorder.Stop()
	if err != nil {
		a.logger.Error("failed to stop recorder", "component", "app", "operation", "handleReleaseStream", "error", err)
		a.emitState(StateReady)
		return
	}

	if len(audio) == 0 {
		a.logger.Warn("no audio captured", "component", "app", "operation", "handleReleaseStream")
		a.emitState(StateReady)
		return
	}

	// Run final transcription async — don't block the event loop
	a.wg.Add(1)
	go a.finalStreamTranscribe(audio, lastText)
}

func (a *App) finalStreamTranscribe(audio []float32, lastText string) {
	defer a.wg.Done()

	// Wait for the streamer's last tick to release both the busy flag
	// and the transcriber bulkhead (up to 5s each).
	acquired := false
	for i := 0; i < 50; i++ {
		if atomic.CompareAndSwapInt32(&a.busy, 0, 1) {
			acquired = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !acquired {
		a.logger.Warn("timed out waiting for busy flag",
			"component", "app", "operation", "finalStreamTranscribe")
		a.emitState(StateReady)
		return
	}
	defer atomic.StoreInt32(&a.busy, 0)

	// Also wait for the transcriber bulkhead to be free
	if t, ok := a.transcriber.(interface{ IsInflight() bool }); ok {
		for i := 0; i < 50; i++ {
			if !t.IsInflight() {
				break
			}
			if i == 49 {
				a.logger.Warn("transcriber bulkhead still occupied",
					"component", "app", "operation", "finalStreamTranscribe")
				a.emitState(StateReady)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	a.emitState(StateTranscribing)

	transcribeCtx, cancel := context.WithTimeout(context.Background(), clipboardTranscribeTimeout)
	defer cancel()

	finalText, err := a.transcriber.Transcribe(transcribeCtx, audio)
	if err != nil {
		a.logger.Error("final transcription failed",
			"component", "app", "operation", "finalStreamTranscribe", "error", err)
		a.emitState(StateReady)
		return
	}

	finalText = strings.TrimSpace(finalText)
	if finalText == "" {
		a.logger.Warn("no speech detected",
			"component", "app", "operation", "finalStreamTranscribe")
		a.emitState(StateReady)
		return
	}

	if a.typer != nil && lastText != "" {
		if err := a.typer.ReplaceAll(len([]rune(lastText)), finalText); err != nil {
			a.logger.Error("failed to replace with final text",
				"component", "app", "operation", "finalStreamTranscribe", "error", err)
		}
	} else if a.typer != nil {
		if err := a.typer.Type(finalText); err != nil {
			a.logger.Error("failed to type final text",
				"component", "app", "operation", "finalStreamTranscribe", "error", err)
		}
	}

	a.logger.Info("stream complete",
		"component", "app", "operation", "finalStreamTranscribe",
		"text_length", len(finalText))
	a.emitState(StateReady)
}

func (a *App) handleReleaseClipboard() {
	audio, err := a.recorder.Stop()
	a.recording = false
	if err != nil {
		a.logger.Error("failed to stop recording",
			"operation", "handleReleaseClipboard", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
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

	if !atomic.CompareAndSwapInt32(&a.busy, 0, 1) {
		a.logger.Warn("transcription already in progress, dropping audio",
			"operation", "transcribeAndPaste")
		// handleReleaseClipboard already moved the UI into Transcribing.
		// If we drop the job here, we must restore Ready or the app stays stuck.
		a.emitState(StateReady)
		return
	}
	defer atomic.StoreInt32(&a.busy, 0)

	transcribeCtx, cancel := context.WithTimeout(context.Background(), clipboardTranscribeTimeout)
	defer cancel()
	text, err := a.transcriber.Transcribe(transcribeCtx, audio)
	if err != nil {
		a.logger.Error("transcription failed",
			"operation", "transcribeAndPaste", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	if text == "" {
		a.logger.Warn("no speech detected", "operation", "transcribeAndPaste")
		a.emitState(StateReady)
		return
	}

	if err := a.paster.Paste(text); err != nil {
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

// IsIdle returns true if no recording, transcription, or native CGO work is in flight.
// Checks both the app-level busy flag AND the transcriber's bulkhead semaphore.
func (a *App) IsIdle() bool {
	if a.recording || atomic.LoadInt32(&a.busy) != 0 {
		return false
	}
	// Probe the transcriber bulkhead — if we can acquire and release, it's idle
	if t, ok := a.transcriber.(interface{ IsInflight() bool }); ok {
		if t.IsInflight() {
			return false
		}
	}
	return true
}

// SetRecorder replaces the recorder used by the app.
// Must only be called when no recording is in progress.
func (a *App) SetRecorder(r Recorder) {
	a.recorder = r
}

// SetTranscriber replaces the transcriber used by the app.
// Must only be called when no transcription is in progress.
func (a *App) SetTranscriber(t Transcriber) {
	a.transcriber = t
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

	if a.recorder != nil {
		if err := a.recorder.Close(); err != nil {
			a.logger.Error("failed to close recorder",
				"component", "app", "operation", "Shutdown", "error", err)
		}
	}
	if a.transcriber != nil {
		if err := a.transcriber.Close(); err != nil {
			a.logger.Error("failed to close transcriber",
				"component", "app", "operation", "Shutdown", "error", err)
		}
	}
}
