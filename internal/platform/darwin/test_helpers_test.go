//go:build darwin

package darwin

import (
	"context"
	"sync"
)

type mockRecorder struct {
	mu              sync.Mutex
	startCalled     bool
	stopCalled      bool
	closeCalled     bool
	refreshCalled   bool
	markStaleCalled bool
	audio           []float32
	startErr        error
	stopErr         error
	refreshErr      error
	startFn         func(ctx context.Context) error
	stopFn          func() ([]float32, error)
	refreshFn       func() error
	markStaleFn     func(reason string)
}

func (m *mockRecorder) Warm() {}

func (m *mockRecorder) RefreshDevices() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshCalled = true
	if m.refreshFn != nil {
		return m.refreshFn()
	}
	return m.refreshErr
}

func (m *mockRecorder) MarkStale(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markStaleCalled = true
	if m.markStaleFn != nil {
		m.markStaleFn(reason)
	}
}

func (m *mockRecorder) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	if m.startFn != nil {
		return m.startFn(ctx)
	}
	return m.startErr
}

func (m *mockRecorder) Stop() ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
	if m.stopFn != nil {
		return m.stopFn()
	}
	return m.audio, m.stopErr
}

func (m *mockRecorder) Snapshot() []float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.audio
}

func (m *mockRecorder) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

type mockTranscriber struct {
	mu            sync.Mutex
	text          string
	err           error
	closeCalled   bool
	receivedAudio []float32
	transcribeFn  func(ctx context.Context, audio []float32) (string, error)
}

func (m *mockTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	m.mu.Lock()
	fn := m.transcribeFn
	m.receivedAudio = audio
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, audio)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text, m.err
}

func (m *mockTranscriber) SetVocabulary(_ string) {}

func (m *mockTranscriber) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

type mockPaster struct {
	mu         sync.Mutex
	pastedText string
	err        error
}

func (m *mockPaster) Paste(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pastedText = text
	return m.err
}
