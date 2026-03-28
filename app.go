package main

import (
	"log/slog"
	"sync/atomic"
)

// App is the orchestrator that wires hotkey events to the
// record -> transcribe -> paste pipeline.
type App struct {
	recorder    Recorder
	transcriber Transcriber
	paster      Paster
	sound       *Sound
	logger      *slog.Logger
	busy        int32 // atomic flag: 1 = transcribing
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
		recorder:    recorder,
		transcriber: transcriber,
		paster:      paster,
		sound:       sound,
		logger:      logger.With("component", "app"),
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

func (a *App) handlePress() {
	if atomic.LoadInt32(&a.busy) == 1 {
		a.logger.Warn("still transcribing, ignoring press",
			"operation", "handlePress")
		a.sound.PlayError()
		return
	}
	a.logger.Info(">>> RECORDING", "operation", "handlePress")
	a.sound.PlayStart()

	if err := a.recorder.Start(); err != nil {
		a.logger.Error("failed to start recording",
			"operation", "handlePress", "error", err)
		a.sound.PlayError()
	}
}

func (a *App) handleRelease() {
	a.logger.Info(">>> RELEASED — stopping recorder", "operation", "handleRelease")

	audio, err := a.recorder.Stop()
	if err != nil {
		a.logger.Error("failed to stop recording",
			"operation", "handleRelease", "error", err)
		a.sound.PlayError()
		return
	}

	a.sound.PlayStop()

	if len(audio) == 0 {
		a.logger.Warn("no audio captured", "operation", "handleRelease")
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
		return
	}

	if text == "" {
		a.logger.Warn("no speech detected", "operation", "transcribeAndPaste")
		return
	}

	if err := a.paster.Paste(text); err != nil {
		a.logger.Error("paste failed",
			"operation", "transcribeAndPaste", "error", err)
		a.sound.PlayError()
		return
	}

	a.logger.Info("text pasted", "operation", "transcribeAndPaste",
		"text_length", len(text))
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
