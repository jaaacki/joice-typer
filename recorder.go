package main

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/gordonklaus/portaudio"
)

type portaudioRecorder struct {
	sampleRate float64
	stream     *portaudio.Stream
	buffer     []float32
	chunks     [][]float32
	mu         sync.Mutex
	recording  bool
	done       chan struct{}
	logger     *slog.Logger
}

func NewRecorder(sampleRate int, logger *slog.Logger) Recorder {
	return &portaudioRecorder{
		sampleRate: float64(sampleRate),
		logger:     logger.With("component", "recorder"),
	}
}

func InitAudio() error {
	return portaudio.Initialize()
}

func TerminateAudio() error {
	return portaudio.Terminate()
}

func (r *portaudioRecorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("recorder.Start: already recording")
	}

	r.logger.Debug("starting", "operation", "Start", "sample_rate", r.sampleRate)

	r.chunks = nil
	r.recording = true
	r.done = make(chan struct{})
	r.buffer = make([]float32, 1024)

	stream, err := portaudio.OpenDefaultStream(1, 0, r.sampleRate, len(r.buffer), r.buffer)
	if err != nil {
		r.recording = false
		return fmt.Errorf("recorder.Start: open stream: %w", err)
	}
	r.stream = stream

	if err := stream.Start(); err != nil {
		r.recording = false
		if closeErr := stream.Close(); closeErr != nil {
			r.logger.Error("failed to close stream after start error",
				"operation", "Start", "error", closeErr)
		}
		return fmt.Errorf("recorder.Start: start stream: %w", err)
	}

	// Read audio in a goroutine
	go r.readLoop()

	r.logger.Debug("recording started", "operation", "Start")
	return nil
}

func (r *portaudioRecorder) readLoop() {
	defer close(r.done)
	for {
		r.mu.Lock()
		recording := r.recording
		r.mu.Unlock()
		if !recording {
			return
		}
		if err := r.stream.Read(); err != nil {
			r.mu.Lock()
			isRecording := r.recording
			r.mu.Unlock()
			if !isRecording {
				return // expected: stream was stopped
			}
			r.logger.Error("read error", "operation", "readLoop", "error", err)
			return
		}
		r.mu.Lock()
		chunk := make([]float32, len(r.buffer))
		copy(chunk, r.buffer)
		r.chunks = append(r.chunks, chunk)
		r.mu.Unlock()
	}
}

func (r *portaudioRecorder) Stop() ([]float32, error) {
	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return nil, fmt.Errorf("recorder.Stop: not recording")
	}
	r.recording = false
	r.mu.Unlock()

	r.logger.Debug("stopping", "operation", "Stop")

	// Wait for readLoop to exit (it checks r.recording and exits)
	<-r.done

	// Now safe to stop and close the stream — readLoop is no longer reading
	var stopErr error
	if err := r.stream.Stop(); err != nil {
		stopErr = fmt.Errorf("recorder.Stop: stop stream: %w", err)
		r.logger.Error("failed to stop stream", "operation", "Stop", "error", err)
	}
	if err := r.stream.Close(); err != nil {
		r.logger.Error("failed to close stream", "operation", "Stop", "error", err)
		if stopErr == nil {
			stopErr = fmt.Errorf("recorder.Stop: close stream: %w", err)
		}
	}
	r.stream = nil

	// Flatten chunks
	r.mu.Lock()
	chunks := r.chunks
	r.chunks = nil
	r.mu.Unlock()

	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	if total == 0 {
		r.logger.Warn("no audio captured", "operation", "Stop")
		return []float32{}, stopErr
	}

	audio := make([]float32, 0, total)
	for _, chunk := range chunks {
		audio = append(audio, chunk...)
	}
	r.logger.Debug("recording stopped", "operation", "Stop", "samples", len(audio),
		"duration_sec", float64(len(audio))/r.sampleRate)
	return audio, stopErr
}

func (r *portaudioRecorder) Close() error {
	r.logger.Info("closing", "operation", "Close")
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stream != nil {
		if err := r.stream.Close(); err != nil {
			return fmt.Errorf("recorder.Close: %w", err)
		}
	}
	return nil
}
