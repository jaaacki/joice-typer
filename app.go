package main

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

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

	if err := a.recorder.Start(); err != nil {
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
	if a.streamer != nil {
		a.streamer.Stop()
	}

	audio, err := a.recorder.Stop()
	a.recording = false
	if err != nil {
		a.logger.Error("failed to stop recording",
			"operation", "handleReleaseStream", "error", err)
		a.sound.PlayError()
		a.emitState(StateReady)
		a.streamer = nil
		return
	}

	a.sound.PlayStop()
	a.emitState(StateTranscribing)

	streamer := a.streamer
	a.streamer = nil

	if streamer != nil && len(audio) > 0 {
		go a.finalizeStream(streamer, audio)
	} else {
		a.emitState(StateReady)
	}
}

func (a *App) finalizeStream(streamer *Streamer, audio []float32) {
	if !atomic.CompareAndSwapInt32(&a.busy, 0, 1) {
		a.logger.Warn("finalization already in progress",
			"operation", "finalizeStream")
		return
	}
	defer atomic.StoreInt32(&a.busy, 0)

	finalText, err := streamer.Finalize(audio)
	if err != nil {
		a.logger.Error("finalize failed",
			"operation", "finalizeStream",
			"error", err,
			"partial_text", streamer.LastText())
		a.sound.PlayError()
		a.emitState(StateReady)
		return
	}

	a.logger.Info("stream complete",
		"operation", "finalizeStream",
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
	go a.transcribeAndPaste(audio)
}

func (a *App) transcribeAndPaste(audio []float32) {
	if !atomic.CompareAndSwapInt32(&a.busy, 0, 1) {
		a.logger.Warn("transcription already in progress, dropping audio",
			"operation", "transcribeAndPaste")
		return
	}
	defer atomic.StoreInt32(&a.busy, 0)

	text, err := a.transcriber.Transcribe(audio)
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

// Shutdown gracefully closes all components.
func (a *App) Shutdown() {
	a.logger.Info("shutting down", "operation", "Shutdown")

	if err := a.recorder.Close(); err != nil {
		a.logger.Error("failed to close recorder",
			"operation", "Shutdown", "error", err)
	}
	if err := a.transcriber.Close(); err != nil {
		a.logger.Error("failed to close transcriber",
			"operation", "Shutdown", "error", err)
	}

	a.logger.Info("shutdown complete", "operation", "Shutdown")
}
