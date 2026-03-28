package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

type portaudioRecorder struct {
	sampleRate   float64
	deviceName   string
	stream       *portaudio.Stream
	buffer       []float32
	chunks       [][]float32
	mu           sync.Mutex
	recording    bool
	done         chan struct{}
	sessionID    uint64
	logger       *slog.Logger
	maxSamples   int
	totalSamples int
}

func NewRecorder(sampleRate int, deviceName string, logger *slog.Logger) Recorder {
	return &portaudioRecorder{
		sampleRate: float64(sampleRate),
		deviceName: deviceName,
		logger:     logger.With("component", "recorder"),
		maxSamples: int(float64(sampleRate) * 30.0),
	}
}

func InitAudio() error {
	return portaudio.Initialize()
}

func TerminateAudio() error {
	return portaudio.Terminate()
}

// ListInputDevices prints available input devices to stdout.
// This is intentional CLI output for --list-devices, not application logging.
func ListInputDevices() error {
	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("recorder.ListInputDevices: %w", err)
	}
	fmt.Println("Available input devices:")
	for _, d := range devices {
		if d.MaxInputChannels > 0 {
			fmt.Printf("  %-50s  (channels: %d, rate: %.0f Hz)\n",
				d.Name, d.MaxInputChannels, d.DefaultSampleRate)
		}
	}
	fmt.Println("\nSet input_device in ~/.config/voicetype/config.yaml to use a specific device.")
	return nil
}

func findInputDevice(name string) (*portaudio.DeviceInfo, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, fmt.Errorf("recorder.findInputDevice: %w", err)
	}
	for _, d := range devices {
		if d.MaxInputChannels > 0 && d.Name == name {
			return d, nil
		}
	}
	return nil, fmt.Errorf("recorder.findInputDevice: input device %q not found", name)
}

func (r *portaudioRecorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("recorder.Start: already recording")
	}

	// Ensure previous readLoop has fully exited before starting new session.
	// If Stop timed out, the zombie readLoop still holds the old stream.
	// Refuse to start until it's done to prevent ownership conflicts.
	if r.done != nil {
		select {
		case <-r.done:
			// Previous session cleaned up
		default:
			return fmt.Errorf("recorder.Start: previous recording session still active")
		}
	}

	r.logger.Debug("starting", "operation", "Start",
		"sample_rate", r.sampleRate, "device", r.deviceName)

	r.chunks = nil
	r.totalSamples = 0
	r.recording = true
	r.sessionID++
	r.buffer = make([]float32, 1024)

	var stream *portaudio.Stream
	var err error

	if r.deviceName != "" {
		device, devErr := findInputDevice(r.deviceName)
		if devErr != nil {
			r.recording = false
			return fmt.Errorf("recorder.Start: %w", devErr)
		}
		params := portaudio.StreamParameters{
			Input: portaudio.StreamDeviceParameters{
				Device:   device,
				Channels: 1,
				Latency:  device.DefaultLowInputLatency,
			},
			SampleRate:      r.sampleRate,
			FramesPerBuffer: len(r.buffer),
		}
		stream, err = portaudio.OpenStream(params, r.buffer)
	} else {
		stream, err = portaudio.OpenDefaultStream(1, 0, r.sampleRate, len(r.buffer), r.buffer)
	}

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

	// Pass session state by value — readLoop owns its own stream, buffer,
	// and done channel. A zombie readLoop from a timed-out Stop cannot
	// interfere with a future session's resources.
	done := make(chan struct{})
	r.done = done
	go r.readLoop(stream, r.buffer, done, r.sessionID)

	r.logger.Debug("recording started", "operation", "Start")
	return nil
}

// readLoop reads audio chunks from the stream until stopped.
// Owns the stream reference passed by value — closes it on exit.
// Uses sessionID to detect if it has been superseded by a new session.
func (r *portaudioRecorder) readLoop(stream *portaudio.Stream, buffer []float32, done chan struct{}, sessionID uint64) {
	defer func() {
		stream.Close()
		close(done)
	}()
	for {
		if err := stream.Read(); err != nil {
			// stream.Read returns error when stream is stopped — expected
			return
		}
		r.mu.Lock()
		if !r.recording || r.sessionID != sessionID {
			r.mu.Unlock()
			return
		}
		chunk := make([]float32, len(buffer))
		copy(chunk, buffer)
		r.chunks = append(r.chunks, chunk)
		r.totalSamples += len(chunk)
		if r.totalSamples >= r.maxSamples {
			r.recording = false
			r.mu.Unlock()
			r.logger.Warn("max recording duration reached",
				"operation", "readLoop",
				"total_samples", r.totalSamples,
				"max_samples", r.maxSamples)
			return
		}
		r.mu.Unlock()
	}
}

func (r *portaudioRecorder) Snapshot() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := 0
	for _, chunk := range r.chunks {
		total += len(chunk)
	}
	if total == 0 {
		return nil
	}

	audio := make([]float32, 0, total)
	for _, chunk := range r.chunks {
		audio = append(audio, chunk...)
	}
	return audio
}

func (r *portaudioRecorder) Stop() ([]float32, error) {
	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return nil, fmt.Errorf("recorder.Stop: not recording")
	}
	r.recording = false
	chunks := r.chunks
	r.chunks = nil
	stream := r.stream // grab ref under lock before readLoop can close it
	done := r.done
	r.mu.Unlock()

	r.logger.Debug("stopping", "operation", "Stop")

	// Signal PortAudio to stop — readLoop will exit when Read returns error
	if stream != nil {
		if err := stream.Stop(); err != nil {
			r.logger.Error("failed to stop stream", "operation", "Stop", "error", err)
		}
	}

	// Wait for readLoop to finish and close the stream it owns
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		r.logger.Error("readLoop stop timed out", "operation", "Stop")
	}

	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	if total == 0 {
		r.logger.Warn("no audio captured", "operation", "Stop")
		return []float32{}, nil
	}

	audio := make([]float32, 0, total)
	for _, chunk := range chunks {
		audio = append(audio, chunk...)
	}
	r.logger.Debug("recording stopped", "operation", "Stop", "samples", len(audio),
		"duration_sec", float64(len(audio))/r.sampleRate)
	return audio, nil
}

func (r *portaudioRecorder) Close() error {
	r.logger.Info("closing", "operation", "Close")
	// Stream cleanup is handled by readLoop's defer.
	// Each readLoop owns its stream by value and closes it on exit.
	return nil
}
