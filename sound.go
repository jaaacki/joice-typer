package main

import (
	"context"
	"log/slog"
	"os/exec"
	"time"
)

type Sound struct {
	enabled bool
	logger  *slog.Logger
	sem     chan struct{} // limits concurrent afplay processes
}

func NewSound(enabled bool, logger *slog.Logger) *Sound {
	return &Sound{
		enabled: enabled,
		logger:  logger.With("component", "sound"),
		sem:     make(chan struct{}, 3), // max 3 concurrent sounds
	}
}

func (s *Sound) Play(name string) {
	if !s.enabled {
		return
	}
	select {
	case s.sem <- struct{}{}:
		go func() {
			defer func() { <-s.sem }()
			path := "/System/Library/Sounds/" + name + ".aiff"
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "afplay", path)
			if err := cmd.Run(); err != nil {
				s.logger.Error("failed to play sound",
					"operation", "Play",
					"sound", name,
					"path", path,
					"error", err,
				)
			}
		}()
	default:
		s.logger.Debug("sound skipped, too many concurrent",
			"operation", "Play", "sound", name)
	}
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
