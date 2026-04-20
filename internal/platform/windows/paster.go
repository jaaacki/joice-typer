//go:build windows

package windows

import (
	"log/slog"
)

type noopPaster struct {
	logger *slog.Logger
}

func NewPaster(logger *slog.Logger) Paster {
	if logger == nil {
		logger = slog.Default()
	}
	return &noopPaster{
		logger: logger.With("component", "paster"),
	}
}

func (p *noopPaster) Paste(text string) error {
	if p.logger != nil {
		p.logger.Debug("paste requested", "operation", "Paste", "text_length", len(text))
	}
	return nil
}
