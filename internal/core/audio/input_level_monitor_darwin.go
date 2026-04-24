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
	paInited   bool
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
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("input monitor portaudio initialize: %w", err)
	}
	m.paInited = true
	device, err := m.resolveDevice()
	if err != nil {
		_ = portaudio.Terminate()
		m.paInited = false
		return err
	}
	params := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   device,
			Channels: 1,
			Latency:  device.DefaultLowInputLatency,
		},
		SampleRate:      device.DefaultSampleRate,
		FramesPerBuffer: len(m.buffer),
	}
	stream, err := portaudio.OpenStream(params, m.buffer)
	if err != nil {
		_ = portaudio.Terminate()
		m.paInited = false
		return fmt.Errorf("input monitor open stream: %w", err)
	}
	if err := stream.Start(); err != nil {
		stream.Close()
		_ = portaudio.Terminate()
		m.paInited = false
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
		close(stopCh)
		// Stop (not Abort) signals the stream to drain and causes the blocking
		// Pa_ReadStream inside readLoop to return an error, unblocking doneCh.
		_ = stream.Stop()
		m.mu.Unlock()
		waitForReadLoop(doneCh, m.logger, "SetInputDevice")
		m.mu.Lock()
	}
	if m.paInited {
		_ = portaudio.Terminate()
		m.paInited = false
	}
	m.deviceID = deviceID
	m.level = bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"}
	m.mu.Unlock()
	m.logger.Info("switching monitored input device", "operation", "SetInputDevice", "device", deviceID)
	return m.start()
}

func (m *darwinInputLevelMonitor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stream == nil {
		if m.paInited {
			_ = portaudio.Terminate()
			m.paInited = false
		}
		return nil
	}
	stream := m.stream
	stopCh := m.stopCh
	doneCh := m.doneCh
	m.stream = nil
	m.stopCh = nil
	m.doneCh = nil
	close(stopCh)
	// Stop (not Abort) signals the stream to drain and causes the blocking
	// Pa_ReadStream inside readLoop to return an error, unblocking doneCh.
	_ = stream.Stop()
	m.mu.Unlock()
	waitForReadLoop(doneCh, m.logger, "Close")
	m.mu.Lock()
	if m.paInited {
		_ = portaudio.Terminate()
		m.paInited = false
	}
	return nil
}

// waitForReadLoop waits for the readLoop goroutine to finish after the stream
// has been stopped. The 3-second timeout is a safety net: if Pa_StopStream did
// not unblock the blocking Pa_ReadStream call (implementation-dependent on
// some PortAudio backends), we log a warning rather than hanging forever.
func waitForReadLoop(doneCh <-chan struct{}, logger *slog.Logger, operation string) {
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		logger.Warn("readLoop did not stop within timeout; possible goroutine leak",
			"operation", operation)
	}
}
