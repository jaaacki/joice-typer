//go:build darwin

package darwin

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestPowerEventHandler_SleepMarksRecorderStale(t *testing.T) {
	rec := &mockRecorder{}
	app := NewApp(rec, &mockTranscriber{}, &mockPaster{}, NewSound(false, slog.Default()), slog.Default())

	handler := MakePowerEventHandler(app, func() Recorder { return rec }, slog.Default())
	handler(PowerEventSleep)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !rec.markStaleCalled {
		t.Fatal("expected sleep event to mark recorder stale")
	}
}

func TestPowerEventHandler_WakeRefreshesWhenIdle(t *testing.T) {
	rec := &mockRecorder{}
	app := NewApp(rec, &mockTranscriber{}, &mockPaster{}, NewSound(false, slog.Default()), slog.Default())

	handler := MakePowerEventHandler(app, func() Recorder { return rec }, slog.Default())
	handler(PowerEventWake)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !rec.refreshCalled {
		t.Fatal("expected wake event to refresh recorder while idle")
	}
}

func TestPowerEventHandler_WakeSkipsRefreshWhenBusy(t *testing.T) {
	rec := &mockRecorder{audio: []float32{0.1, 0.2}}
	blockCh := make(chan struct{})
	trans := &mockTranscriber{
		transcribeFn: func(ctx context.Context, audio []float32) (string, error) {
			<-blockCh
			return "text", nil
		},
	}
	app := NewApp(rec, trans, &mockPaster{}, NewSound(false, slog.Default()), slog.Default())
	events := make(chan HotkeyEvent, 2)
	go app.Run(events)
	events <- TriggerPressed
	time.Sleep(25 * time.Millisecond)
	events <- TriggerReleased
	time.Sleep(100 * time.Millisecond)

	handler := MakePowerEventHandler(app, func() Recorder { return rec }, slog.Default())
	handler(PowerEventWake)

	close(blockCh)
	close(events)
	app.Shutdown()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.refreshCalled {
		t.Fatal("did not expect wake event to refresh recorder while app is busy")
	}
}
