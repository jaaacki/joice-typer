package app

import (
	"errors"
	"strings"
	"testing"
)

func TestUnsupportedDependencyError_IsTyped(t *testing.T) {
	err := UnsupportedDependencyError("recorder", "Start", "audio recording", "windows", "amd64")

	var unavailable *ErrDependencyUnavailable
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected ErrDependencyUnavailable, got %T", err)
	}
	if unavailable.Component != "recorder" || unavailable.Operation != "Start" {
		t.Fatalf("expected recorder.Start metadata, got %+v", unavailable)
	}
	if unavailable.Wrapped == nil || !strings.Contains(unavailable.Wrapped.Error(), "bootstrap build") {
		t.Fatalf("expected wrapped bootstrap-build detail, got %+v", unavailable)
	}
}

func TestUnsupportedDependencyError_CloseOperationMetadata(t *testing.T) {
	err := UnsupportedDependencyError("transcriber", "Close", "transcription", "windows", "amd64")

	var unavailable *ErrDependencyUnavailable
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected ErrDependencyUnavailable, got %T", err)
	}
	if unavailable.Component != "transcriber" || unavailable.Operation != "Close" {
		t.Fatalf("expected transcriber.Close metadata, got %+v", unavailable)
	}
}
