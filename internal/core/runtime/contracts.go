package app

import (
	"context"
	"fmt"
)

// These interfaces define the cross-package behavior expected by the runtime.
// Platform implementations may differ internally, but must satisfy these
// behavioral contracts.

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
	// both granted. Calls onUpdate on each poll so the caller can update UI.
	// Returns error on context cancellation.
	WaitForPermissions(ctx context.Context, onUpdate func(accessibility, inputMonitoring bool)) error
	RunMainLoopOnly()
	Start(events chan HotkeyEvent) error
	Stop() error
}

// Recorder captures audio from the configured input device, or the system
// default input device when no explicit device is configured.
// Start begins capture. Stop ends capture and returns the audio buffer.
type Recorder interface {
	Warm() // pre-open audio stream for instant Start
	Start(ctx context.Context) error
	Stop() ([]float32, error)
	Snapshot() []float32     // copy of audio captured so far, without stopping
	RefreshDevices() error   // safely re-init PortAudio to pick up new devices
	MarkStale(reason string) // invalidate warmed/backend state until refreshed
	Close() error
}

// Transcriber converts audio samples to text using whisper.cpp.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []float32) (string, error)
	SetVocabulary(vocab string)
	Close() error
}

// Paster inserts text at the current cursor position.
type Paster interface {
	Paste(text string) error
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
