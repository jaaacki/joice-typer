# Ring Buffer Pre-Capture + Timing Instrumentation

**Date:** 2026-03-30

## Problem

Occasionally the first word of a recording is lost. The exact cause is unknown — could be PortAudio stream.Start() jitter, warm stream staleness, or macOS audio subsystem latency spikes. We need:
1. Timing instrumentation to diagnose the actual delay
2. A ring buffer that continuously captures audio so the first word is never lost regardless of latency

## Design

### Ring Buffer (recorder.go)

- `Warm()` opens the stream AND starts a goroutine that continuously reads into a circular buffer holding 500ms of audio (8000 samples at 16kHz mono)
- The ring buffer is a `[]float32` with a write cursor that wraps around
- When `Start()` is called: snapshot the ring buffer contents (ordered correctly), then continue recording normally with readLoop as today
- When `Stop()` is called, the snapshot is prepended to the recorded audio
- After `Stop()`, the next `Warm()` call resumes ring buffer capture
- The ring buffer goroutine exits cleanly on `Close()`

### Ring Buffer Data Structure

```
type ringBuffer struct {
    buf    []float32
    cursor int
    full   bool
    mu     sync.Mutex
}
```

- `Write(samples []float32)` — appends samples, wraps cursor
- `Snapshot() []float32` — returns a correctly-ordered copy of buffered audio

### Integration with Existing Flow

- `Warm()` → opens stream + starts ring buffer reader goroutine
- `Start()` → snapshots ring buffer, stops ring buffer goroutine, transitions stream to readLoop
- `Stop()` → prepends ring buffer snapshot to recorded chunks
- Re-`Warm()` after stop → new ring buffer, new reader goroutine
- `Close()` → stops ring buffer goroutine, closes stream

### Timing Instrumentation

- DEBUG log in `app.go handlePress()` with elapsed time from event receipt to `recorder.Start()` return
- DEBUG log in `recorder.Start()` with elapsed time for `stream.Start()` call
- DEBUG log for whether warm stream was used vs cold open

### What Doesn't Change

- Transcriber, App, contracts interfaces — `Start()`/`Stop()` signatures stay the same
- The audio `[]float32` returned by `Stop()` just has more samples at the front
- Config — no new settings, 500ms hardcoded
- No new files — ring buffer is a small struct in recorder.go
