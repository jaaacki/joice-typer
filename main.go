package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
)

func main() {
	// The main goroutine must stay on the main OS thread for macOS CFRunLoop.
	runtime.LockOSThread()

	// --- Resolve default config path (may fail if $HOME is unset) ---
	defaultCfgPath, err := DefaultConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	configPath := flag.String("config", defaultCfgPath, "path to config file")
	listDevices := flag.Bool("list-devices", false, "list available audio input devices and exit")
	flag.Parse()

	// --- List devices mode ---
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

	// --- Load config ---
	cfg, err := LoadConfig(*configPath)
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
		"component", "main", "operation", "main",
		"config_path", *configPath,
		"model_size", cfg.ModelSize,
		"trigger_key", cfg.TriggerKey,
		"language", cfg.Language,
		"sample_rate", cfg.SampleRate,
	)

	// --- Init PortAudio ---
	if err := InitAudio(); err != nil {
		logger.Error("failed to initialize audio",
			"component", "main", "operation", "main", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := TerminateAudio(); err != nil {
			logger.Error("failed to terminate audio",
				"component", "main", "operation", "main", "error", err)
		}
	}()

	// --- Resolve model path ---
	modelPath, err := DefaultModelPath(cfg.ModelSize)
	if err != nil {
		logger.Error("failed to resolve model path",
			"component", "main", "operation", "main", "error", err)
		os.Exit(1)
	}

	// --- Init transcriber (loads model -- may download on first run) ---
	transcriber, err := NewTranscriber(modelPath, cfg.Language, logger)
	if err != nil {
		logger.Error("failed to initialize transcriber",
			"component", "main", "operation", "main", "error", err)
		os.Exit(1)
	}

	// --- Init recorder ---
	recorder := NewRecorder(cfg.SampleRate, cfg.InputDevice, logger)

	// --- Init paster ---
	paster := NewPaster(logger)

	// --- Init sound ---
	sound := NewSound(cfg.SoundFeedback, logger)

	// --- Create app ---
	app := NewApp(recorder, transcriber, paster, sound, logger)

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
		"component", "main", "operation", "main",
		"trigger_key", cfg.TriggerKey)

	// --- Start hotkey listener on main thread (blocks) ---
	if err := hotkey.Start(events); err != nil {
		logger.Error("hotkey listener failed",
			"component", "main", "operation", "main", "error", err)
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
		"component", "main", "operation", "main")
}
