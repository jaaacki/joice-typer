package main

import (
	"log/slog"
	"sync/atomic"
)

// App is the orchestrator that wires hotkey events to the
// record -> transcribe -> paste pipeline.
type App struct {
	recorder      Recorder
	transcriber   Transcriber
	paster        Paster
	sound         *Sound
	logger        *slog.Logger
	busy          int32 // atomic flag: 1 = transcribing
	onStateChange func(AppState)
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
	a.onStateChange = fn
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
	a.onStateChange(StateRecording)

	if err := a.recorder.Start(); err != nil {
		a.logger.Error("failed to start recording",
			"operation", "handlePress", "error", err)
		a.sound.PlayError()
		a.onStateChange(StateReady)
	}
}

func (a *App) handleRelease() {
	a.logger.Debug("trigger released", "operation", "handleRelease")

	audio, err := a.recorder.Stop()
	if err != nil {
		a.logger.Error("failed to stop recording",
			"operation", "handleRelease", "error", err)
		a.sound.PlayError()
		a.onStateChange(StateReady)
		return
	}

	a.sound.PlayStop()
	a.onStateChange(StateTranscribing)

	if len(audio) == 0 {
		a.logger.Warn("no audio captured", "operation", "handleRelease")
		a.onStateChange(StateReady)
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
		a.onStateChange(StateReady)
		return
	}

	if text == "" {
		a.logger.Warn("no speech detected", "operation", "transcribeAndPaste")
		a.onStateChange(StateReady)
		return
	}

	if err := a.paster.Paste(text); err != nil {
		a.logger.Error("paste failed",
			"operation", "transcribeAndPaste", "error", err)
		a.sound.PlayError()
		a.onStateChange(StateReady)
		return
	}

	a.logger.Info("text pasted", "operation", "transcribeAndPaste",
		"text_length", len(text))
	a.onStateChange(StateReady)
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
