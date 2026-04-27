//go:build darwin

package launcher

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

type fakeReloadHotkey struct{ id string }

func (h *fakeReloadHotkey) WaitForPermissions(ctx context.Context, onUpdate func(bool, bool)) error {
	return nil
}
func (h *fakeReloadHotkey) RunMainLoopOnly()                           {}
func (h *fakeReloadHotkey) Start(events chan apppkg.HotkeyEvent) error { return nil }
func (h *fakeReloadHotkey) Stop() error                                { return nil }

type fakeReloadRecorder struct {
	id         string
	closeCalls int
}

func (r *fakeReloadRecorder) Warm()                           {}
func (r *fakeReloadRecorder) Start(ctx context.Context) error { return nil }
func (r *fakeReloadRecorder) Stop() ([]float32, error)        { return nil, nil }
func (r *fakeReloadRecorder) Snapshot() []float32             { return nil }
func (r *fakeReloadRecorder) RefreshDevices() error           { return nil }
func (r *fakeReloadRecorder) MarkStale(reason string)         {}
func (r *fakeReloadRecorder) Close() error                    { r.closeCalls++; return nil }

type fakeReloadTranscriber struct {
	id         string
	closeCalls int
	vocabulary []string
}

func (t *fakeReloadTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	return "", nil
}
func (t *fakeReloadTranscriber) SetVocabulary(vocab string) {
	t.vocabulary = append(t.vocabulary, vocab)
}
func (t *fakeReloadTranscriber) Close() error { t.closeCalls++; return nil }

type noopReloadPaster struct{}

func (noopReloadPaster) Paste(text string) error { return nil }

func testReloadLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestApplyReloadedConfig_RollsBackOnTranscriberFailure(t *testing.T) {
	logger := testReloadLogger()
	oldHotkey := &fakeReloadHotkey{id: "old-hotkey"}
	oldRecorder := &fakeReloadRecorder{id: "old-recorder"}
	oldTranscriber := &fakeReloadTranscriber{id: "old-transcriber"}
	app := apppkg.NewApp(oldRecorder, oldTranscriber, noopReloadPaster{}, nil, logger)

	runtime := &runtimeConfigState{
		cfg: configpkg.Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			InputDevice:     "mic-a",
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
			Vocabulary:      "old vocab",
		},
		hotkey:      oldHotkey,
		recorder:    oldRecorder,
		transcriber: oldTranscriber,
	}

	setActiveCalled := 0
	setRecorderCalled := 0
	statusStates := []apppkg.AppState{}
	newRecorder := &fakeReloadRecorder{id: "new-recorder"}
	newHotkey := &fakeReloadHotkey{id: "new-hotkey"}

	err := applyReloadedConfig(runtime, app, configpkg.Config{
		TriggerKey:      []string{"ctrl", "shift"},
		ModelSize:       "medium",
		Language:        "en",
		SampleRate:      16000,
		InputDevice:     "mic-b",
		DecodeMode:      "greedy",
		PunctuationMode: "opinionated",
		Vocabulary:      "new vocab",
	}, logger, runtimeReloadDeps{
		newHotkey:        func(keys []string, logger *slog.Logger) apppkg.HotkeyListener { return newHotkey },
		newRecorder:      func(sampleRate int, device string, logger *slog.Logger) apppkg.Recorder { return newRecorder },
		defaultModelPath: func(modelSize string) (string, error) { return "/tmp/model.bin", nil },
		newTranscriber: func(ctx context.Context, modelPath, modelSize, language string, sampleRate int, decodeMode, punctuationMode, outputMode string, logger *slog.Logger) (apppkg.Transcriber, error) {
			return nil, errors.New("load failed")
		},
		setActiveHotkey:     func(apppkg.HotkeyListener) { setActiveCalled++ },
		setSettingsRecorder: func(apppkg.Recorder) { setRecorderCalled++ },
		updateStatusBar:     func(state apppkg.AppState) { statusStates = append(statusStates, state) },
		postNotification:    func(string, string) {},
		formatHotkeyDisplay: func(keys []string) string { return "ctrl+shift" },
		setStatusBarText:    func(string) {},
	})
	if err == nil {
		t.Fatal("expected transcriber reload failure")
	}

	if runtime.hotkey != oldHotkey {
		t.Fatal("hotkey should remain unchanged on failed reload")
	}
	if runtime.recorder != oldRecorder {
		t.Fatal("recorder should remain unchanged on failed reload")
	}
	if runtime.transcriber != oldTranscriber {
		t.Fatal("transcriber should remain unchanged on failed reload")
	}
	if setActiveCalled != 0 {
		t.Fatalf("expected no hotkey activation on failed reload, got %d", setActiveCalled)
	}
	if setRecorderCalled != 0 {
		t.Fatalf("expected no recorder swap on failed reload, got %d", setRecorderCalled)
	}
	if oldRecorder.closeCalls != 0 {
		t.Fatalf("old recorder should not close on failed reload, got %d", oldRecorder.closeCalls)
	}
	if oldTranscriber.closeCalls != 0 {
		t.Fatalf("old transcriber should not close on failed reload, got %d", oldTranscriber.closeCalls)
	}
	if len(oldTranscriber.vocabulary) != 0 {
		t.Fatalf("old transcriber vocabulary should not change on failed reload, got %v", oldTranscriber.vocabulary)
	}
	if len(statusStates) != 2 || statusStates[0] != apppkg.StateLoading || statusStates[1] != apppkg.StateReady {
		t.Fatalf("expected loading->ready status rollback, got %v", statusStates)
	}
}

func TestApplyReloadedConfig_ReappliesUnchangedVocabularyToNewTranscriber(t *testing.T) {
	logger := testReloadLogger()
	oldHotkey := &fakeReloadHotkey{id: "old-hotkey"}
	oldRecorder := &fakeReloadRecorder{id: "old-recorder"}
	oldTranscriber := &fakeReloadTranscriber{id: "old-transcriber"}
	app := apppkg.NewApp(oldRecorder, oldTranscriber, noopReloadPaster{}, nil, logger)

	runtime := &runtimeConfigState{
		cfg: configpkg.Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			InputDevice:     "mic-a",
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
			Vocabulary:      "joice, typer",
		},
		hotkey:      oldHotkey,
		recorder:    oldRecorder,
		transcriber: oldTranscriber,
	}

	newTranscriber := &fakeReloadTranscriber{id: "new-transcriber"}
	newCfg := runtime.cfg
	newCfg.ModelSize = "medium"

	if err := applyReloadedConfig(runtime, app, newCfg, logger, runtimeReloadDeps{
		newHotkey:        func(keys []string, logger *slog.Logger) apppkg.HotkeyListener { return oldHotkey },
		newRecorder:      func(sampleRate int, device string, logger *slog.Logger) apppkg.Recorder { return oldRecorder },
		defaultModelPath: func(modelSize string) (string, error) { return "/tmp/model.bin", nil },
		newTranscriber: func(ctx context.Context, modelPath, modelSize, language string, sampleRate int, decodeMode, punctuationMode, outputMode string, logger *slog.Logger) (apppkg.Transcriber, error) {
			return newTranscriber, nil
		},
		setActiveHotkey:     func(apppkg.HotkeyListener) {},
		setSettingsRecorder: func(apppkg.Recorder) {},
		updateStatusBar:     func(apppkg.AppState) {},
		postNotification:    func(string, string) {},
		formatHotkeyDisplay: func(keys []string) string { return "fn + shift" },
		setStatusBarText:    func(string) {},
	}); err != nil {
		t.Fatalf("applyReloadedConfig: %v", err)
	}

	if runtime.transcriber != newTranscriber {
		t.Fatal("expected transcriber to swap to new instance")
	}
	if len(newTranscriber.vocabulary) != 1 || newTranscriber.vocabulary[0] != "joice, typer" {
		t.Fatalf("expected unchanged vocabulary applied to rebuilt transcriber, got %v", newTranscriber.vocabulary)
	}
}

func TestApplyReloadedConfig_AppliesAtomicallyOnSuccess(t *testing.T) {
	logger := testReloadLogger()
	oldHotkey := &fakeReloadHotkey{id: "old-hotkey"}
	oldRecorder := &fakeReloadRecorder{id: "old-recorder"}
	oldTranscriber := &fakeReloadTranscriber{id: "old-transcriber"}
	app := apppkg.NewApp(oldRecorder, oldTranscriber, noopReloadPaster{}, nil, logger)

	runtime := &runtimeConfigState{
		cfg: configpkg.Config{
			TriggerKey:      []string{"fn", "shift"},
			ModelSize:       "small",
			Language:        "en",
			SampleRate:      16000,
			InputDevice:     "mic-a",
			DecodeMode:      "beam",
			PunctuationMode: "conservative",
			Vocabulary:      "old vocab",
		},
		hotkey:      oldHotkey,
		recorder:    oldRecorder,
		transcriber: oldTranscriber,
	}

	newHotkey := &fakeReloadHotkey{id: "new-hotkey"}
	newRecorder := &fakeReloadRecorder{id: "new-recorder"}
	newTranscriber := &fakeReloadTranscriber{id: "new-transcriber"}
	var activeHotkey apppkg.HotkeyListener
	var settingsRecorder apppkg.Recorder
	var hotkeyText string
	statusStates := []apppkg.AppState{}

	newCfg := configpkg.Config{
		TriggerKey:      []string{"ctrl", "shift"},
		ModelSize:       "medium",
		Language:        "en",
		SampleRate:      16000,
		InputDevice:     "mic-b",
		DecodeMode:      "greedy",
		PunctuationMode: "opinionated",
		Vocabulary:      "new vocab",
	}
	if err := applyReloadedConfig(runtime, app, newCfg, logger, runtimeReloadDeps{
		newHotkey:        func(keys []string, logger *slog.Logger) apppkg.HotkeyListener { return newHotkey },
		newRecorder:      func(sampleRate int, device string, logger *slog.Logger) apppkg.Recorder { return newRecorder },
		defaultModelPath: func(modelSize string) (string, error) { return "/tmp/model.bin", nil },
		newTranscriber: func(ctx context.Context, modelPath, modelSize, language string, sampleRate int, decodeMode, punctuationMode, outputMode string, logger *slog.Logger) (apppkg.Transcriber, error) {
			return newTranscriber, nil
		},
		setActiveHotkey:     func(h apppkg.HotkeyListener) { activeHotkey = h },
		setSettingsRecorder: func(r apppkg.Recorder) { settingsRecorder = r },
		updateStatusBar:     func(state apppkg.AppState) { statusStates = append(statusStates, state) },
		postNotification:    func(string, string) {},
		formatHotkeyDisplay: func(keys []string) string { return "ctrl + shift" },
		setStatusBarText:    func(text string) { hotkeyText = text },
	}); err != nil {
		t.Fatalf("applyReloadedConfig: %v", err)
	}

	if !reflect.DeepEqual(runtime.cfg, newCfg) {
		t.Fatalf("expected runtime cfg to update, got %+v", runtime.cfg)
	}
	if runtime.hotkey != newHotkey || activeHotkey != newHotkey {
		t.Fatal("expected hotkey to swap to new instance")
	}
	if runtime.recorder != newRecorder || settingsRecorder != newRecorder {
		t.Fatal("expected recorder to swap to new instance")
	}
	if runtime.transcriber != newTranscriber {
		t.Fatal("expected transcriber to swap to new instance")
	}
	if oldRecorder.closeCalls != 1 {
		t.Fatalf("expected old recorder close once, got %d", oldRecorder.closeCalls)
	}
	if oldTranscriber.closeCalls != 1 {
		t.Fatalf("expected old transcriber close once, got %d", oldTranscriber.closeCalls)
	}
	if len(newTranscriber.vocabulary) != 1 || newTranscriber.vocabulary[0] != "new vocab" {
		t.Fatalf("expected new transcriber vocabulary applied before swap, got %v", newTranscriber.vocabulary)
	}
	if hotkeyText != "ctrl+shift" {
		t.Fatalf("expected compact hotkey display, got %q", hotkeyText)
	}
	if len(statusStates) != 2 || statusStates[0] != apppkg.StateLoading || statusStates[1] != apppkg.StateReady {
		t.Fatalf("expected loading->ready status sequence, got %v", statusStates)
	}
}
