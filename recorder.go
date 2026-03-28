package main

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/gordonklaus/portaudio"
)

type portaudioRecorder struct {
	sampleRate  float64
	deviceName  string
	stream      *portaudio.Stream
	buffer      []float32
	chunks      [][]float32
	mu          sync.Mutex
	recording   bool
	done        chan struct{}
	logger      *slog.Logger
}

func NewRecorder(sampleRate int, deviceName string, logger *slog.Logger) Recorder {
	return &portaudioRecorder{
		sampleRate: float64(sampleRate),
		deviceName: deviceName,
		logger:     logger.With("component", "recorder"),
	}
}

func InitAudio() error {
	return portaudio.Initialize()
}

func TerminateAudio() error {
	return portaudio.Terminate()
}

// ListInputDevices prints all available input devices to stdout.
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

	r.logger.Debug("starting", "operation", "Start",
		"sample_rate", r.sampleRate, "device", r.deviceName)

	r.chunks = nil
	r.recording = true
	r.done = make(chan struct{})
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

	go r.readLoop()

	r.logger.Debug("recording started", "operation", "Start")
	return nil
}

func (r *portaudioRecorder) readLoop() {
	defer close(r.done)
	for {
		if err := r.stream.Read(); err != nil {
			// stream.Read returns error when stream is stopped — this is expected
			return
		}
		r.mu.Lock()
		if !r.recording {
			r.mu.Unlock()
			return
		}
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

	// Stop the stream first — this causes stream.Read() in readLoop to return
	// an error, breaking the loop. PortAudio's Pa_StopStream is safe to call
	// concurrently with Pa_ReadStream.
	var stopErr error
	if err := r.stream.Stop(); err != nil {
		stopErr = fmt.Errorf("recorder.Stop: stop stream: %w", err)
		r.logger.Error("failed to stop stream", "operation", "Stop", "error", err)
	}

	// Wait for readLoop to finish (now unblocked by stream.Stop)
	<-r.done

	if err := r.stream.Close(); err != nil {
		r.logger.Error("failed to close stream", "operation", "Stop", "error", err)
		if stopErr == nil {
			stopErr = fmt.Errorf("recorder.Stop: close stream: %w", err)
		}
	}
	r.stream = nil

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
