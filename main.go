package main

import (
	"context"
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

// activeHotkey holds the current hotkey listener for stop/restart.
// Protected by activeHotkeyMu. Used by signalHotkeyRestart() in settings.go
// and the signal handler.
var (
	activeHotkeyMu sync.Mutex
	activeHotkey   HotkeyListener
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

	// Create a context cancelled by SIGTERM — used for permissions and startup
	startupCtx, startupCancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal", "component", "main", "operation", "signal", "signal", sig.String())
		startupCancel() // cancel permission polling and setup wizard
		activeHotkeyMu.Lock()
		h := activeHotkey
		activeHotkeyMu.Unlock()
		if h != nil {
			if stopErr := h.Stop(); stopErr != nil {
				logger.Error("failed to stop hotkey", "component", "main", "operation", "signal", "error", stopErr)
			}
		}
	}()

	// First-run: show setup wizard
	firstRun := IsFirstRun()
	if firstRun {
		selectedDevice, setupErr := RunSetupWizard(startupCtx, logger)
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

	transcriber, err := NewTranscriber(startupCtx, modelPath, cfg.ModelSize, cfg.Language, cfg.SampleRate, logger)
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

	activeHotkeyMu.Lock()
	activeHotkey = hotkey
	activeHotkeyMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.Run(events)
		app.Shutdown()
	}()

	// Always validate permissions before starting hotkey — not just on first run.
	// Ad-hoc signing means every rebuild/reinstall invalidates old grants.
	UpdateStatusBar(StateNoPermission)
	if err := hotkey.WaitForPermissions(startupCtx, func(acc, inp bool) {
		if acc && inp {
			return
		}
		UpdateStatusBar(StateNoPermission)
	}); err != nil {
		logger.Error("permission wait cancelled", "component", "main", "operation", "runAppMode", "error", err)
		hotkeyMu.Lock()
		hotkeyEvents = nil
		hotkeyMu.Unlock()
		close(events)
		wg.Wait()
		return
	}

	UpdateStatusBar(StateReady)
	sound.PlayReady()

	PostNotification("JoiceTyper is ready", "Hold Fn+Shift to dictate.")

	logger.Info("ready", "component", "main", "operation", "runAppMode", "trigger_key", cfg.TriggerKey)

	// Hotkey start/restart loop.
	// hotkey.Start() blocks on [NSApp run]. It returns when Stop() is called
	// — either from the signal handler (shutdown) or from signalHotkeyRestart()
	// in settings.go (preferences changed). We check hotkeyRestartCh to
	// distinguish the two cases.
	for {
		if err := hotkey.Start(events); err != nil {
			logger.Error("hotkey listener failed", "component", "main", "operation", "runAppMode", "error", err)
			break
		}

		// hotkey.Start() returned — CFRunLoopStop was called.
		// Either from signal handler (shutdown) or signalHotkeyRestart (prefs).
		select {
		case <-hotkeyRestartCh:
			logger.Info("restarting hotkey with new config", "component", "main", "operation", "runAppMode")
			// Reload config and recreate listener
			oldCfg := cfg
			cfg, err = LoadConfig(cfgPath)
			if err != nil {
				logger.Error("failed to reload config, keeping current hotkey",
					"component", "main", "operation", "runAppMode", "error", err)
				continue // retry with existing hotkey
			}
			hotkey = NewHotkeyListener(cfg.TriggerKey, logger)
			activeHotkeyMu.Lock()
			activeHotkey = hotkey
			activeHotkeyMu.Unlock()

			// Recreate recorder if input device changed
			if oldCfg.InputDevice != cfg.InputDevice {
				if closeErr := recorder.Close(); closeErr != nil {
					logger.Error("failed to close old recorder", "component", "main", "operation", "runAppMode", "error", closeErr)
				}
				recorder = NewRecorder(cfg.SampleRate, cfg.InputDevice, logger)
				app.SetRecorder(recorder)
				logger.Info("recorder updated", "component", "main", "operation", "runAppMode", "device", cfg.InputDevice)
			}

			// Recreate transcriber if language changed.
			// Create new BEFORE closing old — if creation fails, keep the working one.
			if oldCfg.Language != cfg.Language {
				newModelPath, pathErr := DefaultModelPath(cfg.ModelSize)
				if pathErr != nil {
					logger.Error("failed to resolve model path for transcriber reload",
						"component", "main", "operation", "runAppMode", "error", pathErr)
					continue
				}
				newTranscriber, tErr := NewTranscriber(context.Background(), newModelPath, cfg.ModelSize, cfg.Language, cfg.SampleRate, logger)
				if tErr != nil {
					logger.Error("failed to recreate transcriber, keeping old",
						"component", "main", "operation", "runAppMode", "error", tErr)
				} else {
					oldTranscriber := transcriber
					transcriber = newTranscriber
					app.SetTranscriber(newTranscriber)
					logger.Info("transcriber updated", "component", "main", "operation", "runAppMode", "language", cfg.Language)
					if closeErr := oldTranscriber.Close(); closeErr != nil {
						logger.Error("failed to close old transcriber", "component", "main", "operation", "runAppMode", "error", closeErr)
					}
				}
			}

			UpdateStatusBar(StateReady)
			continue
		default:
			// Normal shutdown — signal handler called Stop
		}
		break
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
		"type_mode", cfg.TypeMode,
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

	// --- Signal handling (before model init so SIGTERM can cancel download) ---
	startupCtx, startupCancel := context.WithCancel(context.Background())
	events := make(chan HotkeyEvent, 10)
	hotkey := NewHotkeyListener(cfg.TriggerKey, logger)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal",
			"component", "main", "operation", "signal",
			"signal", sig.String())
		startupCancel()
		if err := hotkey.Stop(); err != nil {
			logger.Error("failed to stop hotkey listener",
				"component", "main", "operation", "signal", "error", err)
		}
	}()

	// --- Init transcriber (loads model -- may download on first run) ---
	transcriber, err := NewTranscriber(startupCtx, modelPath, cfg.ModelSize, cfg.Language, cfg.SampleRate, logger)
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
