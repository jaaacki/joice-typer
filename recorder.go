package main

import (
	"context"
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
	activeStream *portaudio.Stream // guarded by mu; used by Stop to force-abort on timeout
	warmStream   *portaudio.Stream // pre-opened stream for instant start
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

// RefreshAudioDevices re-initializes PortAudio to pick up newly
// connected devices (e.g. Bluetooth microphones).
func RefreshAudioDevices() error {
	if err := portaudio.Terminate(); err != nil {
		return fmt.Errorf("recorder.RefreshAudioDevices: terminate: %w", err)
	}
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("recorder.RefreshAudioDevices: initialize: %w", err)
	}
	return nil
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
		return nil, &ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDevice", Wrapped: err}
	}
	for _, d := range devices {
		if d.MaxInputChannels > 0 && d.Name == name {
			return d, nil
		}
	}
	return nil, &ErrDependencyUnavailable{Component: "recorder", Operation: "findInputDevice", Wrapped: fmt.Errorf("input device %q not found", name)}
}

// openStream creates and returns a PortAudio stream for the configured device.
func (r *portaudioRecorder) openStream(buf []float32) (*portaudio.Stream, error) {
	if r.deviceName != "" {
		device, devErr := findInputDevice(r.deviceName)
		if devErr != nil {
			r.logger.Warn("configured device not found, using default input",
				"operation", "openStream",
				"device", r.deviceName, "error", devErr)
			return portaudio.OpenDefaultStream(1, 0, r.sampleRate, len(buf), buf)
		}
		params := portaudio.StreamParameters{
			Input: portaudio.StreamDeviceParameters{
				Device:   device,
				Channels: 1,
				Latency:  device.DefaultLowInputLatency,
			},
			SampleRate:      r.sampleRate,
			FramesPerBuffer: len(buf),
		}
		return portaudio.OpenStream(params, buf)
	}
	return portaudio.OpenDefaultStream(1, 0, r.sampleRate, len(buf), buf)
}

// Warm pre-opens the audio stream so Start() is near-instant.
// Call after the app is ready. Safe to call multiple times.
func (r *portaudioRecorder) Warm() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.warmStream != nil {
		return // already warmed
	}

	buf := make([]float32, 512)
	stream, err := r.openStream(buf)
	if err != nil {
		r.logger.Warn("failed to pre-warm audio stream",
			"component", "recorder", "operation", "Warm", "error", err)
		return
	}
	r.warmStream = stream
	r.buffer = buf
	r.logger.Info("audio stream pre-warmed", "component", "recorder", "operation", "Warm")
}

func (r *portaudioRecorder) Start(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return fmt.Errorf("recorder.Start: already recording")
	}

	// Block new sessions until previous one is fully cleaned up
	if r.done != nil {
		select {
		case <-r.done:
			// Previous session exited
		default:
			return fmt.Errorf("recorder.Start: previous session still active, cannot start new recording")
		}
	}

	r.logger.Debug("starting", "operation", "Start",
		"sample_rate", r.sampleRate, "device", r.deviceName)

	r.chunks = nil
	r.totalSamples = 0
	r.recording = true
	r.sessionID++

	var stream *portaudio.Stream
	var err error

	// Use pre-warmed stream if available (near-instant start)
	if r.warmStream != nil {
		stream = r.warmStream
		r.warmStream = nil
		r.logger.Debug("using pre-warmed stream", "operation", "Start")
	} else {
		// Cold path: open stream now (slow — can take 1-2 seconds)
		r.buffer = make([]float32, 512)
		stream, err = r.openStream(r.buffer)
		if err != nil {
			r.recording = false
			return &ErrDependencyUnavailable{Component: "recorder", Operation: "Start", Wrapped: fmt.Errorf("open stream: %w", err)}
		}
	}

	r.stream = stream

	if err := stream.Start(); err != nil {
		r.recording = false
		if closeErr := stream.Close(); closeErr != nil {
			r.logger.Error("failed to close stream after start error",
				"operation", "Start", "error", closeErr)
		}
		return &ErrDependencyUnavailable{Component: "recorder", Operation: "Start", Wrapped: fmt.Errorf("start stream: %w", err)}
	}

	// Store stream reference so Stop can force-abort on timeout.
	r.activeStream = stream

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
		r.mu.Lock()
		// Only clear activeStream if this session still owns it.
		// A late zombie readLoop must not clobber a newer session's handle.
		if r.sessionID == sessionID {
			r.activeStream = nil
		}
		r.mu.Unlock()
		close(done)
	}()
	for {
		// Watchdog: if stream.Read() doesn't return in 5s, the OS audio callback is stuck.
		// One goroutine per iteration is safe — the loop is sequential (waits for Read to return).
		readDone := make(chan error, 1)
		go func() {
			readDone <- stream.Read()
		}()

		var readErr error
		select {
		case readErr = <-readDone:
			// normal path
		case <-time.After(5 * time.Second):
			r.logger.Error("stream.Read() hung for 5s, aborting recording",
				"component", "recorder", "operation", "readLoop")
			stream.Abort()
			return
		}

		if readErr != nil {
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
	r.recording = false
	chunks := r.chunks
	r.chunks = nil
	done := r.done
	r.mu.Unlock()

	if done == nil {
		return nil, fmt.Errorf("recorder.Stop: not recording")
	}

	r.logger.Debug("stopping", "operation", "Stop")

	// Don't call stream.Stop() — it deadlocks with stream.Read() in PortAudio.
	// readLoop checks r.recording after each Read() (~64ms buffer cycle) and exits.
	// readLoop's defer calls stream.Close() which implicitly stops and releases.
	select {
	case <-done:
		// readLoop exited cleanly
	case <-time.After(500 * time.Millisecond):
		r.logger.Warn("readLoop did not exit in time, force-aborting stream",
			"operation", "Stop")
		r.mu.Lock()
		if r.activeStream != nil {
			r.activeStream.Abort()
			r.activeStream = nil
		}
		r.mu.Unlock()
		// Wait a bit more for goroutine to notice
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			r.logger.Error("readLoop goroutine leaked after force-abort",
				"operation", "Stop")
			// Allow future sessions despite the leak — nil out done so Start() doesn't reject
			r.mu.Lock()
			r.done = nil
			r.mu.Unlock()
		}
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

	// Re-warm the stream in background so next recording starts instantly
	go r.Warm()

	return audio, nil
}

func (r *portaudioRecorder) Close() error {
	r.logger.Info("closing", "operation", "Close")
	// Stream cleanup is handled by readLoop's defer.
	// Each readLoop owns its stream by value and closes it on exit.
	return nil
}
