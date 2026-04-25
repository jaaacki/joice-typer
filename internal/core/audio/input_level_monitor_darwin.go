//go:build darwin && cgo

package audio

import (
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	bridgepkg "voicetype/internal/core/bridge"

	"github.com/gordonklaus/portaudio"
)

// darwinInputLevelMonitor reads RMS levels from the configured input device
// for the preferences mic-test meter. It deliberately does NOT call
// portaudio.Initialize/Terminate — those are owned by the launcher's
// InitAudio/TerminateAudio. PortAudio's reference count is shared with the
// recorder's warm stream, and toggling Terminate from this monitor on macOS
// has been observed to invalidate the recorder's pre-warmed stream, killing
// hotkey-triggered recording.
type darwinInputLevelMonitor struct {
	mu         sync.Mutex
	logger     *slog.Logger
	sampleRate float64
	deviceID   string
	stream     *portaudio.Stream
	buffer     []float32
	level      bridgepkg.InputLevelSnapshot
	stopCh     chan struct{}
	doneCh     chan struct{}
}

func NewInputLevelMonitor(sampleRate int, deviceID string, logger *slog.Logger) (InputLevelMonitor, error) {
	if logger == nil {
		logger = slog.Default()
	}
	m := &darwinInputLevelMonitor{
		logger:     logger.With("component", "input-monitor"),
		sampleRate: float64(sampleRate),
		deviceID:   deviceID,
		buffer:     make([]float32, 512),
		level:      bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"},
	}
	if err := m.start(); err != nil {
		return nil, err
	}
	m.logger.Info("input monitor started", "operation", "NewInputLevelMonitor", "device", deviceID)
	return m, nil
}

func (m *darwinInputLevelMonitor) start() error {
	device, err := m.resolveDevice()
	if err != nil {
		return err
	}
	params := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   device,
			Channels: 1,
			Latency:  device.DefaultLowInputLatency,
		},
		SampleRate:      m.sampleRate,
		FramesPerBuffer: len(m.buffer),
	}
	stream, err := portaudio.OpenStream(params, m.buffer)
	if err != nil {
		return fmt.Errorf("input monitor open stream: %w", err)
	}
	if err := stream.Start(); err != nil {
		stream.Close()
		return fmt.Errorf("input monitor start stream: %w", err)
	}
	m.stream = stream
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	m.logger.Info("input monitor stream started", "operation", "start", "device", m.deviceID, "sample_rate", params.SampleRate, "frames_per_buffer", params.FramesPerBuffer)
	go m.readLoop(stream, m.stopCh, m.doneCh)
	return nil
}

func (m *darwinInputLevelMonitor) resolveDevice() (*portaudio.DeviceInfo, error) {
	if m.deviceID == "" {
		return portaudio.DefaultInputDevice()
	}
	return findInputDeviceByID(m.deviceID)
}

func (m *darwinInputLevelMonitor) readLoop(stream *portaudio.Stream, stopCh <-chan struct{}, doneCh chan<- struct{}) {
	// readLoop owns the stream — it closes it on exit so Core Audio releases
	// the input device. Uses non-blocking polling (AvailableToRead + sleep) so
	// it can react to stopCh within ~20ms instead of being stuck inside a
	// blocking Pa_ReadStream cgo call.
	defer func() {
		if err := stream.Close(); err != nil {
			m.logger.Warn("input monitor close failed", "operation", "readLoop", "error", err)
		}
		close(doneCh)
	}()
	for {
		select {
		case <-stopCh:
			m.logger.Info("input monitor stopping", "operation", "readLoop")
			return
		default:
		}
		avail, err := stream.AvailableToRead()
		if err != nil {
			m.logger.Warn("input monitor available-to-read failed", "operation", "readLoop", "error", err)
			return
		}
		if avail < len(m.buffer) {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if err := stream.Read(); err != nil {
			m.logger.Warn("input monitor read failed", "operation", "readLoop", "error", err)
			return
		}
		var sumSq float64
		for _, s := range m.buffer {
			sumSq += float64(s) * float64(s)
		}
		level := 0.0
		if len(m.buffer) > 0 {
			level = math.Sqrt(sumSq/float64(len(m.buffer))) * 12
		}
		// Gate out the mic noise floor — electronic noise + fan hum reads
		// non-zero even in a silent room. Below this threshold treat as 0.
		if level < 0.04 {
			level = 0
		}
		if level > 1 {
			level = 1
		}
		quality := "poor"
		if level >= 0.35 {
			quality = "good"
		} else if level >= 0.12 {
			quality = "acceptable"
		}
		m.mu.Lock()
		m.level = bridgepkg.InputLevelSnapshot{Level: level, Quality: quality}
		m.mu.Unlock()
	}
}

func (m *darwinInputLevelMonitor) Snapshot() bridgepkg.InputLevelSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.level
}

func (m *darwinInputLevelMonitor) SetInputDevice(deviceID string) error {
	m.mu.Lock()
	if deviceID == m.deviceID {
		m.mu.Unlock()
		return nil
	}
	if m.stream != nil {
		stream := m.stream
		stopCh := m.stopCh
		doneCh := m.doneCh
		m.stream = nil
		m.stopCh = nil
		m.doneCh = nil
		m.deviceID = deviceID
		m.level = bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"}
		m.mu.Unlock()
		shutdownStreamAsync(stream, stopCh, doneCh, m.logger, "SetInputDevice")
	} else {
		m.deviceID = deviceID
		m.level = bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"}
		m.mu.Unlock()
	}
	m.logger.Info("switching monitored input device", "operation", "SetInputDevice", "device", deviceID)
	return m.start()
}

func (m *darwinInputLevelMonitor) Close() error {
	m.mu.Lock()
	if m.stream == nil {
		m.mu.Unlock()
		return nil
	}
	stream := m.stream
	stopCh := m.stopCh
	doneCh := m.doneCh
	m.stream = nil
	m.stopCh = nil
	m.doneCh = nil
	m.mu.Unlock()
	shutdownStreamAsync(stream, stopCh, doneCh, m.logger, "Close")
	return nil
}

// shutdownStreamAsync signals the readLoop to stop. readLoop owns the stream
// and closes it in its defer, which is what tells Core Audio to release the
// device. Returns immediately so the caller (typically the WKWebView main
// thread) is never blocked. We do not touch the global Pa_Initialize/Terminate
// refcount because that is shared with the recorder's warm stream.
func shutdownStreamAsync(stream *portaudio.Stream, stopCh chan struct{}, doneCh chan struct{}, logger *slog.Logger, operation string) {
	close(stopCh)
	go func() {
		select {
		case <-doneCh:
			// readLoop exited cleanly; stream was closed in its defer.
		case <-time.After(2 * time.Second):
			logger.Warn("readLoop did not stop within timeout; leaking PortAudio stream",
				"operation", operation)
		}
	}()
}
