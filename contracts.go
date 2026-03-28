package main

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
	Start(events chan<- HotkeyEvent) error
	Stop() error
}

// Recorder captures audio from the default input device.
// Start begins capture. Stop ends capture and returns the audio buffer.
type Recorder interface {
	Start() error
	Stop() ([]float32, error)
	Snapshot() []float32 // copy of audio captured so far, without stopping
	Close() error
}

// Transcriber converts audio samples to text using whisper.cpp.
type Transcriber interface {
	Transcribe(audio []float32) (string, error)
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
