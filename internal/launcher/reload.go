//go:build darwin

package launcher

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	apppkg "voicetype/internal/app"
	configpkg "voicetype/internal/config"
)

type runtimeConfigState struct {
	cfg         configpkg.Config
	hotkey      apppkg.HotkeyListener
	recorder    apppkg.Recorder
	transcriber apppkg.Transcriber
}

type runtimeReloadDeps struct {
	newHotkey           func([]string, *slog.Logger) apppkg.HotkeyListener
	newRecorder         func(int, string, *slog.Logger) apppkg.Recorder
	defaultModelPath    func(string) (string, error)
	newTranscriber      func(context.Context, string, string, string, int, string, string, *slog.Logger) (apppkg.Transcriber, error)
	setActiveHotkey     func(apppkg.HotkeyListener)
	setSettingsRecorder func(apppkg.Recorder)
	updateStatusBar     func(apppkg.AppState)
	postNotification    func(string, string)
	formatHotkeyDisplay func([]string) string
	setStatusBarText    func(string)
}

func applyReloadedConfig(runtime *runtimeConfigState, app *apppkg.App, newCfg configpkg.Config, logger *slog.Logger, deps runtimeReloadDeps) error {
	oldCfg := runtime.cfg
	hotkeyChanged := !slices.Equal(oldCfg.TriggerKey, newCfg.TriggerKey)
	recorderChanged := oldCfg.InputDevice != newCfg.InputDevice
	transcriberChanged := oldCfg.Language != newCfg.Language ||
		oldCfg.ModelSize != newCfg.ModelSize ||
		oldCfg.DecodeMode != newCfg.DecodeMode ||
		oldCfg.PunctuationMode != newCfg.PunctuationMode
	vocabularyChanged := oldCfg.Vocabulary != newCfg.Vocabulary

	stagedHotkey := runtime.hotkey
	if hotkeyChanged {
		stagedHotkey = deps.newHotkey(newCfg.TriggerKey, logger)
	}

	stagedRecorder := runtime.recorder
	if recorderChanged {
		stagedRecorder = deps.newRecorder(newCfg.SampleRate, newCfg.InputDevice, logger)
	}

	stagedTranscriber := runtime.transcriber
	if transcriberChanged {
		modelPath, err := deps.defaultModelPath(newCfg.ModelSize)
		if err != nil {
			return err
		}
		deps.updateStatusBar(apppkg.StateLoading)
		deps.postNotification("JoiceTyper", "Loading speech model...")
		reloadCtx, reloadCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer reloadCancel()

		stagedTranscriber, err = deps.newTranscriber(reloadCtx, modelPath, newCfg.ModelSize, newCfg.Language, newCfg.SampleRate, newCfg.DecodeMode, newCfg.PunctuationMode, logger)
		if err != nil {
			deps.updateStatusBar(apppkg.StateReady)
			return err
		}
		if vocabularyChanged {
			stagedTranscriber.SetVocabulary(newCfg.Vocabulary)
		}
	}

	if hotkeyChanged {
		runtime.hotkey = stagedHotkey
		deps.setActiveHotkey(stagedHotkey)
	}

	if recorderChanged {
		oldRecorder := runtime.recorder
		runtime.recorder = stagedRecorder
		deps.setSettingsRecorder(stagedRecorder)
		app.SetRecorder(stagedRecorder)
		logger.Info("recorder updated", "component", "main", "operation", "applyReloadedConfig", "device", newCfg.InputDevice)
		if closeErr := oldRecorder.Close(); closeErr != nil {
			logger.Error("failed to close old recorder", "component", "main", "operation", "applyReloadedConfig", "error", closeErr)
		}
	}

	if transcriberChanged {
		oldTranscriber := runtime.transcriber
		runtime.transcriber = stagedTranscriber
		app.SetTranscriber(stagedTranscriber)
		logger.Info("transcriber updated", "component", "main", "operation", "applyReloadedConfig",
			"language", newCfg.Language, "decode_mode", newCfg.DecodeMode, "punctuation_mode", newCfg.PunctuationMode)
		if closeErr := oldTranscriber.Close(); closeErr != nil {
			logger.Error("failed to close old transcriber", "component", "main", "operation", "applyReloadedConfig", "error", closeErr)
		}
	} else if vocabularyChanged {
		runtime.transcriber.SetVocabulary(newCfg.Vocabulary)
		logger.Info("vocabulary updated", "component", "main", "operation", "applyReloadedConfig")
	}

	runtime.cfg = newCfg
	hotkeyDisplay := deps.formatHotkeyDisplay(newCfg.TriggerKey)
	deps.setStatusBarText(strings.ReplaceAll(hotkeyDisplay, " + ", "+"))
	deps.updateStatusBar(apppkg.StateReady)
	return nil
}
