//go:build darwin

package audio

import (
	"log/slog"
	"testing"
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
