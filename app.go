package main

import (
	"log/slog"
)

// App is the orchestrator that wires hotkey events to the
// record -> transcribe -> paste pipeline.
type App struct {
	recorder    Recorder
	transcriber Transcriber
	paster      Paster
	sound       *Sound
	logger      *slog.Logger
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
	a.logger.Debug("trigger pressed", "operation", "handlePress")
	a.sound.PlayStart()

	if err := a.recorder.Start(); err != nil {
		a.logger.Error("failed to start recording",
			"operation", "handlePress", "error", err)
		a.sound.PlayError()
	}
}

func (a *App) handleRelease() {
	a.logger.Debug("trigger released", "operation", "handleRelease")

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

	text, err := a.transcriber.Transcribe(audio)
	if err != nil {
		a.logger.Error("transcription failed",
			"operation", "handleRelease", "error", err)
		a.sound.PlayError()
		return
	}

	if text == "" {
		a.logger.Warn("no speech detected", "operation", "handleRelease")
		return
	}

	if err := a.paster.Paste(text); err != nil {
		a.logger.Error("paste failed",
			"operation", "handleRelease", "error", err)
		a.sound.PlayError()
		return
	}

	a.logger.Info("text pasted", "operation", "handleRelease",
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
