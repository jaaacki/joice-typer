package main

import (
	"log/slog"
	"os/exec"
)

type Sound struct {
	enabled bool
	logger  *slog.Logger
}

func NewSound(enabled bool, logger *slog.Logger) *Sound {
	return &Sound{
		enabled: enabled,
		logger:  logger.With("component", "sound"),
	}
}

func (s *Sound) Play(name string) {
	if !s.enabled {
		return
	}
	go func() {
		path := "/System/Library/Sounds/" + name + ".aiff"
		cmd := exec.Command("afplay", path)
		if err := cmd.Run(); err != nil {
			s.logger.Error("failed to play sound",
				"operation", "Play",
				"sound", name,
				"path", path,
				"error", err,
			)
		}
	}()
}

func (s *Sound) PlayStart() {
	s.Play("Tink")
}

func (s *Sound) PlayStop() {
	s.Play("Pop")
}

func (s *Sound) PlayError() {
	s.Play("Basso")
}

func (s *Sound) PlayReady() {
	s.Play("Glass")
}
