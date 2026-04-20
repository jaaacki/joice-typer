//go:build windows

package windows

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

var (
	hotkeyMu     sync.Mutex
	hotkeyEvents chan<- HotkeyEvent
)

type hotkeyListener struct {
	triggerKeys []string
	logger      *slog.Logger
}

func NewHotkeyListener(triggerKeys []string, logger *slog.Logger) HotkeyListener {
	if logger == nil {
		logger = slog.Default()
	}
	return &hotkeyListener{
		triggerKeys: append([]string(nil), triggerKeys...),
		logger:      logger.With("component", "hotkey"),
	}
}

func (h *hotkeyListener) WaitForPermissions(ctx context.Context, onUpdate func(accessibility, inputMonitoring bool)) error {
	if onUpdate != nil {
		onUpdate(true, true)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (h *hotkeyListener) RunMainLoopOnly() {}

func (h *hotkeyListener) Start(events chan<- HotkeyEvent) error {
	hotkeyMu.Lock()
	hotkeyEvents = events
	hotkeyMu.Unlock()
	SetActiveHotkey(h)
	return nil
}

func (h *hotkeyListener) Stop() error {
	hotkeyMu.Lock()
	hotkeyEvents = nil
	hotkeyMu.Unlock()
	if current := ActiveHotkey(); current != nil {
		if listener, ok := current.(*hotkeyListener); ok && listener == h {
			SetActiveHotkey(nil)
		}
	}
	return nil
}

func FormatHotkeyDisplay(keys []string) string {
	return strings.Join(keys, " + ")
}

func hotkeyEventString(e HotkeyEvent) string {
	switch e {
	case TriggerPressed:
		return "TriggerPressed"
	case TriggerReleased:
		return "TriggerReleased"
	default:
		return fmt.Sprintf("HotkeyEvent(%d)", int(e))
	}
}
