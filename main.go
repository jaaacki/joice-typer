package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
)

func main() {
	// The main goroutine must stay on the main OS thread for macOS CFRunLoop.
	runtime.LockOSThread()

	defaultCfgPath, _ := DefaultConfigPath()
	configPath := flag.String("config", defaultCfgPath, "path to config file")
	listDevices := flag.Bool("list-devices", false, "list available audio input devices and exit")
	flag.Parse()

	if *listDevices {
		if err := InitAudio(); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
		if err := ListInputDevices(); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			TerminateAudio()
			os.Exit(1)
		}
		TerminateAudio()
		return
	}

	if isAppMode() {
		runAppMode()
	} else {
		runTerminalMode(*configPath)
	}
}

// isAppMode returns true when running inside a macOS .app bundle.
func isAppMode() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(exe, ".app/Contents/MacOS")
}

// suppressStderr redirects file descriptor 2 to a log file so whisper.cpp
// stderr noise does not appear in app mode.
func suppressStderr(logDir string) {
	logPath := filepath.Join(logDir, "whisper-stderr.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return // best effort
	}
	syscall.Dup2(int(f.Fd()), 2)
}

// runAppMode is the entry point when running inside a .app bundle.
func runAppMode() {
	logDir, err := DefaultConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// Suppress whisper.cpp stderr spam
	suppressStderr(logDir)

	logger, logCleanup, err := SetupLogger(logDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer logCleanup()

	// Init PortAudio early (needed for device listing in setup wizard)
	if err := InitAudio(); err != nil {
		logger.Error("failed to initialize audio", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}
	defer func() {
		if tErr := TerminateAudio(); tErr != nil {
			logger.Error("failed to terminate audio", "component", "main", "operation", "runAppMode", "error", tErr)
		}
	}()

	// First-run: show setup wizard
	firstRun := IsFirstRun()
	if firstRun {
		selectedDevice, setupErr := RunSetupWizard(logger)
		if setupErr != nil {
			logger.Error("setup wizard failed", "component", "main", "operation", "runAppMode", "error", setupErr)
			os.Exit(1)
		}
		logger.Info("setup complete", "component", "main", "operation", "runAppMode", "device", selectedDevice)
	}

	// Load config (now exists after setup)
	cfgPath, err := DefaultConfigPath()
	if err != nil {
		logger.Error("failed to resolve config path", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}

	// Load model
	modelPath, err := DefaultModelPath(cfg.ModelSize)
	if err != nil {
		logger.Error("failed to resolve model path", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}

	// Init status bar
	InitStatusBar()
	UpdateStatusBar(StateLoading)

	transcriber, err := NewTranscriber(modelPath, cfg.Language, logger)
	if err != nil {
		logger.Error("failed to initialize transcriber", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}

	recorder := NewRecorder(cfg.SampleRate, cfg.InputDevice, logger)
	paster := NewPaster(logger)
	sound := NewSound(cfg.SoundFeedback, logger)

	var typer Typer
	if cfg.TypeMode == "stream" {
		typer = NewTyper(logger)
	}

	app := NewApp(recorder, transcriber, paster, typer, sound, cfg.TypeMode, logger)
	app.SetStateCallback(func(state AppState) {
		UpdateStatusBar(state)
	})

	events := make(chan HotkeyEvent, 10)
	hotkey := NewHotkeyListener(cfg.TriggerKey, logger)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal", "component", "main", "operation", "signal", "signal", sig.String())
		if stopErr := hotkey.Stop(); stopErr != nil {
			logger.Error("failed to stop hotkey", "component", "main", "operation", "signal", "error", stopErr)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.Run(events)
		app.Shutdown()
	}()

	UpdateStatusBar(StateReady)
	sound.PlayReady()

	if firstRun {
		PostNotification("JoiceTyper is ready", "Hold Fn+Shift to dictate.")
	}

	logger.Info("ready", "component", "main", "operation", "runAppMode", "trigger_key", cfg.TriggerKey)

	if err := hotkey.Start(events); err != nil {
		logger.Error("hotkey listener failed", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}

	hotkeyMu.Lock()
	hotkeyEvents = nil
	hotkeyMu.Unlock()
	close(events)
	wg.Wait()

	logger.Info("voicetype stopped", "component", "main", "operation", "runAppMode")
}

// runTerminalMode is the entry point for CLI invocations.
func runTerminalMode(configPath string) {
	// --- Load config ---
	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// --- Resolve log directory ---
	logDir, err := DefaultConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// --- Setup logger ---
	logger, logCleanup, err := SetupLogger(logDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer logCleanup()

	logger.Info("starting voicetype",
		"component", "main", "operation", "runTerminalMode",
		"config_path", configPath,
		"model_size", cfg.ModelSize,
		"trigger_key", cfg.TriggerKey,
		"language", cfg.Language,
		"sample_rate", cfg.SampleRate,
	)

	// --- Init PortAudio ---
	if err := InitAudio(); err != nil {
		logger.Error("failed to initialize audio",
			"component", "main", "operation", "runTerminalMode", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := TerminateAudio(); err != nil {
			logger.Error("failed to terminate audio",
				"component", "main", "operation", "runTerminalMode", "error", err)
		}
	}()

	// --- Resolve model path ---
	modelPath, err := DefaultModelPath(cfg.ModelSize)
	if err != nil {
		logger.Error("failed to resolve model path",
			"component", "main", "operation", "runTerminalMode", "error", err)
		os.Exit(1)
	}

	// --- Init transcriber (loads model -- may download on first run) ---
	transcriber, err := NewTranscriber(modelPath, cfg.Language, logger)
	if err != nil {
		logger.Error("failed to initialize transcriber",
			"component", "main", "operation", "runTerminalMode", "error", err)
		os.Exit(1)
	}

	// --- Init recorder ---
	recorder := NewRecorder(cfg.SampleRate, cfg.InputDevice, logger)

	// --- Init paster ---
	paster := NewPaster(logger)

	// --- Init sound ---
	sound := NewSound(cfg.SoundFeedback, logger)

	// --- Init typer (stream mode only) ---
	var typer Typer
	if cfg.TypeMode == "stream" {
		typer = NewTyper(logger)
	}

	// --- Create app ---
	app := NewApp(recorder, transcriber, paster, typer, sound, cfg.TypeMode, logger)

	// --- Signal handling ---
	events := make(chan HotkeyEvent, 10)
	hotkey := NewHotkeyListener(cfg.TriggerKey, logger)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal",
			"component", "main", "operation", "signal",
			"signal", sig.String())
		if err := hotkey.Stop(); err != nil {
			logger.Error("failed to stop hotkey listener",
				"component", "main", "operation", "signal", "error", err)
		}
	}()

	// --- Start event processing goroutine ---
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.Run(events)
		app.Shutdown()
	}()

	// --- Ready ---
	sound.PlayReady()
	logger.Info("ready -- hold trigger key to record, release to transcribe",
		"component", "main", "operation", "runTerminalMode",
		"trigger_key", cfg.TriggerKey)

	// --- Start hotkey listener on main thread (blocks) ---
	if err := hotkey.Start(events); err != nil {
		logger.Error("hotkey listener failed",
			"component", "main", "operation", "runTerminalMode", "error", err)
		os.Exit(1)
	}

	// Nil the global channel to prevent late C callbacks from sending on closed channel
	hotkeyMu.Lock()
	hotkeyEvents = nil
	hotkeyMu.Unlock()
	close(events)

	// Wait for the app goroutine to finish processing and shut down
	wg.Wait()

	logger.Info("voicetype stopped",
		"component", "main", "operation", "runTerminalMode")
}
