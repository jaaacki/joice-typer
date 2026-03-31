package main

import (
	"log/slog"
	"testing"
	"time"
)

func TestNewRecorder_UsesNinetySecondCap(t *testing.T) {
	recorder, ok := NewRecorder(16000, "", slog.Default()).(*portaudioRecorder)
	if !ok {
		t.Fatal("expected *portaudioRecorder")
	}

	want := 16000 * 90
	if recorder.maxSamples != want {
		t.Fatalf("expected maxSamples %d, got %d", want, recorder.maxSamples)
	}
}

func TestDurationLimits_UseNinetySeconds(t *testing.T) {
	if clipboardTranscribeTimeout != 90*time.Second {
		t.Fatalf("expected clipboardTranscribeTimeout 90s, got %s", clipboardTranscribeTimeout)
	}
	if transcribeTimeout != 90*time.Second {
		t.Fatalf("expected transcribeTimeout 90s, got %s", transcribeTimeout)
	}
	if maxTranscribeSeconds != 90 {
		t.Fatalf("expected maxTranscribeSeconds 90, got %d", maxTranscribeSeconds)
	}
}
