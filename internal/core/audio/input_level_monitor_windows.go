//go:build windows && cgo

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

type windowsInputLevelMonitor struct {
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
	m := &windowsInputLevelMonitor{
		logger:     logger.With("component", "input-monitor"),
		sampleRate: float64(sampleRate),
		deviceID:   deviceID,
		buffer:     make([]float32, 512),
		level:      bridgepkg.InputLevelSnapshot{Level: 0, Quality: "raw"},
	}
	if err := m.start(); err != nil {
		return nil, err
	}
	m.logger.Info("input monitor started", "operation", "NewInputLevelMonitor", "device", deviceID)
	return m, nil
}

func (m *windowsInputLevelMonitor) start() error {
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
		SampleRate:      device.DefaultSampleRate,
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

func (m *windowsInputLevelMonitor) resolveDevice() (*portaudio.DeviceInfo, error) {
	if m.deviceID == "" {
		return portaudio.DefaultInputDevice()
	}
	return findInputDeviceByID(m.deviceID)
}

func (m *windowsInputLevelMonitor) readLoop(stream *portaudio.Stream, stopCh <-chan struct{}, doneCh chan<- struct{}) {
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
			level = math.Sqrt(sumSq/float64(len(m.buffer))) * 4
		}
		if level > 1 {
			level = 1
		}
		m.mu.Lock()
		m.level = bridgepkg.InputLevelSnapshot{Level: level, Quality: "raw"}
		m.mu.Unlock()
	}
}

func (m *windowsInputLevelMonitor) Snapshot() bridgepkg.InputLevelSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.level
}

func (m *windowsInputLevelMonitor) SetInputDevice(deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if deviceID == m.deviceID {
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
		m.mu.Unlock()
		shutdownWindowsInputMonitorStream(stream, doneCh, m.logger, "SetInputDevice")
		m.mu.Lock()
	}
	m.deviceID = deviceID
	m.level = bridgepkg.InputLevelSnapshot{Level: 0, Quality: "raw"}
	m.logger.Info("switching monitored input device", "operation", "SetInputDevice", "device", deviceID)
	return m.start()
}

func (m *windowsInputLevelMonitor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stream == nil {
		return nil
	}
	stream := m.stream
	stopCh := m.stopCh
	doneCh := m.doneCh
	m.stream = nil
	m.stopCh = nil
	m.doneCh = nil
	close(stopCh)
	m.mu.Unlock()
	shutdownWindowsInputMonitorStream(stream, doneCh, m.logger, "Close")
	m.mu.Lock()
	return nil
}

func shutdownWindowsInputMonitorStream(stream *portaudio.Stream, doneCh chan struct{}, logger *slog.Logger, operation string) {
	go func() {
		select {
		case <-doneCh:
		case <-time.After(2 * time.Second):
			logger.Warn("input monitor shutdown timed out; aborting stream", "operation", operation)
			stream.Abort()
			<-doneCh
		}
	}()
}
