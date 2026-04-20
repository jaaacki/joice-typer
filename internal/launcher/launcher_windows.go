//go:build windows

package launcher

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	audiopkg "voicetype/internal/core/audio"
	configpkg "voicetype/internal/core/config"
	loggingpkg "voicetype/internal/core/logging"
	apppkg "voicetype/internal/core/runtime"
	transcriptionpkg "voicetype/internal/core/transcription"
	versionpkg "voicetype/internal/core/version"
	platformpkg "voicetype/internal/platform"
)

func Main() {
	runtime.LockOSThread()

	defaultCfgPath, defaultCfgErr := configpkg.DefaultConfigPath()
	if defaultCfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not resolve default config path: %v\n", defaultCfgErr)
	}
	configPath := flag.String("config", defaultCfgPath, "path to config file")
	listDevices := flag.Bool("list-devices", false, "list available audio input devices and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(versionpkg.FormatVersion(versionpkg.Version))
		return
	}

	if *listDevices {
		if err := audiopkg.InitAudio(); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
		if err := audiopkg.ListInputDevices(); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			if tErr := audiopkg.TerminateAudio(); tErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to terminate audio: %v\n", tErr)
			}
			os.Exit(1)
		}
		if tErr := audiopkg.TerminateAudio(); tErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to terminate audio: %v\n", tErr)
		}
		return
	}

	runWindowsDesktopMode(*configPath)
}

func runWindowsDesktopMode(configPath string) {
	logDir, err := configpkg.DefaultConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	logger, logCleanup, err := loggingpkg.SetupLogger(logDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer logCleanup()

	platformpkg.SetSettingsLogger(logger.With("component", "settings"))
	logger.Info("app starting", "component", "main", "operation", "runWindowsDesktopMode", "version", versionpkg.Version)

	startupCtx, startupCancel := context.WithCancel(context.Background())
	defer startupCancel()

	var shutdownRequested atomic.Bool
	requestShutdown := func(reason string) {
		if shutdownRequested.Swap(true) {
			return
		}
		logger.Info("shutdown requested", "component", "main", "operation", "runWindowsDesktopMode", "reason", reason)
		startupCancel()
		if h := platformpkg.ActiveHotkey(); h != nil {
			if stopErr := h.Stop(); stopErr != nil {
				logger.Error("failed to stop hotkey", "component", "main", "operation", "requestShutdown", "error", stopErr)
			}
		}
	}
	platformpkg.SetQuitHandler(func() {
		requestShutdown("tray_quit")
	})
	defer platformpkg.SetQuitHandler(nil)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal", "component", "main", "operation", "signal", "signal", sig.String())
		requestShutdown("signal_" + sig.String())
	}()

	firstRun := platformpkg.IsFirstRun()
	logger.Info("first run check", "component", "main", "operation", "runWindowsDesktopMode", "first_run", firstRun)
	if firstRun {
		if _, setupErr := platformpkg.RunSetupWizard(startupCtx, logger); setupErr != nil {
			logger.Error("setup wizard failed", "component", "main", "operation", "runWindowsDesktopMode", "error", setupErr)
			os.Exit(1)
		}
	}

	platformpkg.InitStatusBar()
	platformpkg.InitPowerObserver()
	platformpkg.UpdateStatusBar(apppkg.StateLoading)

	for !shutdownRequested.Load() {
		cfg, err := configpkg.LoadConfig(configPath)
		if err != nil {
			logger.Error("failed to load config", "component", "main", "operation", "runWindowsDesktopMode", "error", err)
			platformpkg.PostNotification("JoiceTyper failed to load config", err.Error())
			platformpkg.UpdateStatusBar(apppkg.StateDependencyStuck)
			return
		}

		runtimeResult := startWindowsRuntimeCycle(startupCtx, cfg, logger)
		if runtimeResult.app == nil {
			platformpkg.UpdateStatusBar(apppkg.StateDependencyStuck)
			if runtimeResult.err != nil {
				platformpkg.PostNotification(
					"JoiceTyper Windows runtime unavailable",
					dependencyStatusBody(runtimeResult.err),
				)
			}
			runtimeResult.hotkey.RunMainLoopOnly()
		} else {
			runtimeResult.hotkey.Start(runtimeResult.events)
		}

		platformpkg.ClearHotkeyEvents()
		if runtimeResult.events != nil {
			close(runtimeResult.events)
		}
		if runtimeResult.wg != nil {
			runtimeResult.wg.Wait()
		}
		if runtimeResult.audioInitialized {
			if err := audiopkg.TerminateAudio(); err != nil {
				logger.Error("failed to terminate audio", "component", "main", "operation", "runWindowsDesktopMode", "error", err)
			}
		}
		if runtimeResult.transcriber != nil {
			if err := runtimeResult.transcriber.Close(); err != nil {
				logger.Error("failed to close transcriber", "component", "main", "operation", "runWindowsDesktopMode", "error", err)
			}
		}
		if runtimeResult.recorder != nil && runtimeResult.audioInitialized {
			if err := runtimeResult.recorder.Close(); err != nil {
				logger.Error("failed to close recorder", "component", "main", "operation", "runWindowsDesktopMode", "error", err)
			}
		}
		platformpkg.SetPowerEventHandler(nil)

		if shutdownRequested.Load() {
			break
		}
		select {
		case <-platformpkg.HotkeyRestartCh():
			logger.Info("restarting windows runtime with new config", "component", "main", "operation", "runWindowsDesktopMode")
			continue
		default:
			logger.Info("re-entering windows runtime loop", "component", "main", "operation", "runWindowsDesktopMode")
		}
	}

	logger.Info("joicetyper stopped", "component", "main", "operation", "runWindowsDesktopMode")
}

type windowsRuntimeCycle struct {
	hotkey           apppkg.HotkeyListener
	recorder         apppkg.Recorder
	transcriber      apppkg.Transcriber
	app              *apppkg.App
	events           chan apppkg.HotkeyEvent
	wg               *sync.WaitGroup
	audioInitialized bool
	err              error
}

func startWindowsRuntimeCycle(startupCtx context.Context, cfg configpkg.Config, logger *slog.Logger) windowsRuntimeCycle {
	hotkey := platformpkg.NewHotkeyListener(cfg.TriggerKey, logger)
	platformpkg.SetActiveHotkey(hotkey)
	hotkeyDisplay := platformpkg.FormatHotkeyDisplay(cfg.TriggerKey)
	platformpkg.SetStatusBarHotkeyText(strings.ReplaceAll(hotkeyDisplay, " + ", "+"))

	recorder := audiopkg.NewRecorder(cfg.SampleRate, cfg.InputDevice, logger)
	platformpkg.SetSettingsRecorder(recorder)

	audioInitialized := false
	if err := audiopkg.InitAudio(); err != nil {
		logger.Warn("audio backend unavailable", "component", "main", "operation", "startWindowsRuntimeCycle", "error", err)
		return windowsRuntimeCycle{
			hotkey:      hotkey,
			recorder:    recorder,
			err:         err,
			events:      nil,
			wg:          nil,
			app:         nil,
			transcriber: nil,
		}
	}
	audioInitialized = true

	modelPath, err := configpkg.DefaultModelPath(cfg.ModelSize)
	if err != nil {
		logger.Warn("model path resolution failed", "component", "main", "operation", "startWindowsRuntimeCycle", "error", err)
		return windowsRuntimeCycle{hotkey: hotkey, recorder: recorder, audioInitialized: audioInitialized, err: err}
	}

	transcriber, err := transcriptionpkg.NewTranscriber(startupCtx, modelPath, cfg.ModelSize, cfg.Language, cfg.SampleRate, cfg.DecodeMode, cfg.PunctuationMode, logger)
	if err != nil {
		logger.Warn("transcriber backend unavailable", "component", "main", "operation", "startWindowsRuntimeCycle", "error", err)
		return windowsRuntimeCycle{
			hotkey:           hotkey,
			recorder:         recorder,
			audioInitialized: audioInitialized,
			err:              err,
		}
	}
	if cfg.Vocabulary != "" {
		transcriber.SetVocabulary(cfg.Vocabulary)
	}

	recorder.Warm()
	paster := platformpkg.NewPaster(logger)
	sound := apppkg.NewSound(cfg.SoundFeedback, logger)
	app := apppkg.NewApp(recorder, transcriber, paster, sound, logger)
	app.SetStateCallback(func(state apppkg.AppState) {
		platformpkg.UpdateStatusBar(state)
	})
	platformpkg.SetPowerEventHandler(platformpkg.MakePowerEventHandler(app, func() apppkg.Recorder { return recorder }, logger))

	events := make(chan apppkg.HotkeyEvent, 32)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.Run(events)
		app.Shutdown()
	}()

	platformpkg.UpdateStatusBar(apppkg.StateReady)
	sound.PlayReady()
	platformpkg.PostNotification("JoiceTyper is ready", "Hold "+hotkeyDisplay+" to dictate.")
	logger.Info("ready", "component", "main", "operation", "startWindowsRuntimeCycle", "trigger_key", cfg.TriggerKey)

	return windowsRuntimeCycle{
		hotkey:           hotkey,
		recorder:         recorder,
		transcriber:      transcriber,
		app:              app,
		events:           events,
		wg:               wg,
		audioInitialized: audioInitialized,
	}
}

func dependencyStatusBody(err error) string {
	var depUnavailable *apppkg.ErrDependencyUnavailable
	var depTimeout *apppkg.ErrDependencyTimeout
	switch {
	case errors.As(err, &depUnavailable):
		return fmt.Sprintf("%s is not available yet: %v", depUnavailable.Component, depUnavailable.Wrapped)
	case errors.As(err, &depTimeout):
		return fmt.Sprintf("%s timed out: %v", depTimeout.Component, depTimeout.Wrapped)
	default:
		return err.Error()
	}
}
