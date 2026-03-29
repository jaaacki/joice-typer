package main

import (
	"context"
	"fmt"
)

// ============================================================================
// CONTRACTS — These interfaces are ABSOLUTE. Implementations must match exactly.
// ============================================================================

// --- Hotkey Events ---

// HotkeyEvent represents a trigger key state change.
type HotkeyEvent int

const (
	// TriggerPressed fires when all configured trigger keys are held down.
	TriggerPressed HotkeyEvent = iota
	// TriggerReleased fires when any configured trigger key is released.
	TriggerReleased
)

// --- Component Interfaces ---

// HotkeyListener monitors global key events and emits trigger press/release events.
// Start blocks (runs macOS CFRunLoop). Stop unblocks it.
type HotkeyListener interface {
	// WaitForPermissions polls until Accessibility and Input Monitoring are
	// both granted, prompting the user via macOS dialogs. Calls onUpdate on
	// each poll so the caller can update UI. Returns error on context cancellation.
	WaitForPermissions(ctx context.Context, onUpdate func(accessibility, inputMonitoring bool)) error
	RunMainLoopOnly()
	Start(events chan<- HotkeyEvent) error
	Stop() error
}

// Recorder captures audio from the default input device.
// Start begins capture. Stop ends capture and returns the audio buffer.
type Recorder interface {
	Warm()               // pre-open audio stream for instant Start
	Start(ctx context.Context) error
	Stop() ([]float32, error)
	Snapshot() []float32 // copy of audio captured so far, without stopping
	Close() error
}

// Transcriber converts audio samples to text using whisper.cpp.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []float32) (string, error)
	Close() error
}

// Paster inserts text at the current cursor position.
type Paster interface {
	Paste(text string) error
}

// Typer streams text at the cursor via simulated keystrokes.
type Typer interface {
	Type(text string) error
	Backspace(count int) error
	// ReplaceAll backspaces oldLen runes then types newText.
	// Non-atomic: if interrupted between backspace and type, text may be lost.
	ReplaceAll(oldLen int, newText string) error
}

// --- Typed Errors ---

// ErrDependencyTimeout indicates a blocking dependency exceeded its deadline.
type ErrDependencyTimeout struct {
	Component string
	Operation string
	Wrapped   error
}

func (e *ErrDependencyTimeout) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("%s.%s: dependency timeout: %v", e.Component, e.Operation, e.Wrapped)
	}
	return fmt.Sprintf("%s.%s: dependency timeout", e.Component, e.Operation)
}

func (e *ErrDependencyTimeout) Unwrap() error { return e.Wrapped }

// ErrDependencyUnavailable indicates a dependency could not be reached or initialized.
type ErrDependencyUnavailable struct {
	Component string
	Operation string
	Wrapped   error
}

func (e *ErrDependencyUnavailable) Error() string {
	return fmt.Sprintf("%s.%s: dependency unavailable: %v", e.Component, e.Operation, e.Wrapped)
}

func (e *ErrDependencyUnavailable) Unwrap() error { return e.Wrapped }

// ErrBadPayload indicates corrupted, oversized, or malformed data from a dependency.
type ErrBadPayload struct {
	Component string
	Operation string
	Detail    string
}

func (e *ErrBadPayload) Error() string {
	return fmt.Sprintf("%s.%s: bad payload: %s", e.Component, e.Operation, e.Detail)
}

// ErrPermissionDenied indicates a required macOS permission is not granted.
type ErrPermissionDenied struct {
	Permission string
}

func (e *ErrPermissionDenied) Error() string {
	return fmt.Sprintf("permission denied: %s — enable in System Settings → Privacy & Security", e.Permission)
}
