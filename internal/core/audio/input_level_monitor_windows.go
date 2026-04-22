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
		level:      bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"},
	}
	if err := m.start(); err != nil {
		return nil, err
	}
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
	go m.readLoop()
	return nil
}

func (m *windowsInputLevelMonitor) resolveDevice() (*portaudio.DeviceInfo, error) {
	if m.deviceID == "" {
		return portaudio.DefaultInputDevice()
	}
	return findInputDeviceByID(m.deviceID)
}

func (m *windowsInputLevelMonitor) readLoop() {
	defer close(m.doneCh)
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}
		if err := m.stream.Read(); err != nil {
			return
		}
		var sumSq float64
		for _, s := range m.buffer {
			sumSq += float64(s) * float64(s)
		}
		level := 0.0
		if len(m.buffer) > 0 {
			level = math.Sqrt(sumSq / float64(len(m.buffer))) * 12
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
		time.Sleep(50 * time.Millisecond)
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
		close(m.stopCh)
		m.stream.Abort()
		<-m.doneCh
		m.stream.Close()
		m.stream = nil
	}
	m.deviceID = deviceID
	m.level = bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"}
	return m.start()
}

func (m *windowsInputLevelMonitor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stream == nil {
		return nil
	}
	close(m.stopCh)
	m.stream.Abort()
	<-m.doneCh
	err := m.stream.Close()
	m.stream = nil
	return err
}
