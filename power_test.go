package main

import (
	"log/slog"
	"sync/atomic"
	"testing"
)

func TestPowerEventHandler_SleepMarksRecorderStale(t *testing.T) {
	rec := &mockRecorder{}
	app := NewApp(rec, &mockTranscriber{}, &mockPaster{}, nil, NewSound(false, slog.Default()), "clipboard", slog.Default())

	handler := makePowerEventHandler(app, func() Recorder { return rec }, slog.Default())
	handler(PowerEventSleep)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !rec.markStaleCalled {
		t.Fatal("expected sleep event to mark recorder stale")
	}
}

func TestPowerEventHandler_WakeRefreshesWhenIdle(t *testing.T) {
	rec := &mockRecorder{}
	app := NewApp(rec, &mockTranscriber{}, &mockPaster{}, nil, NewSound(false, slog.Default()), "clipboard", slog.Default())

	handler := makePowerEventHandler(app, func() Recorder { return rec }, slog.Default())
	handler(PowerEventWake)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !rec.refreshCalled {
		t.Fatal("expected wake event to refresh recorder while idle")
	}
}

func TestPowerEventHandler_WakeSkipsRefreshWhenBusy(t *testing.T) {
	rec := &mockRecorder{}
	app := NewApp(rec, &mockTranscriber{}, &mockPaster{}, nil, NewSound(false, slog.Default()), "clipboard", slog.Default())
	atomic.StoreInt32(&app.busy, 1)

	handler := makePowerEventHandler(app, func() Recorder { return rec }, slog.Default())
	handler(PowerEventWake)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.refreshCalled {
		t.Fatal("did not expect wake event to refresh recorder while app is busy")
	}
}
