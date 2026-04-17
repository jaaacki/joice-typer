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
	"sync/atomic"
	"syscall"
	"time"

	config "voicetype/internal/config"
	version "voicetype/internal/version"
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

	defaultCfgPath, defaultCfgErr := config.DefaultConfigPath()
	if defaultCfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not resolve default config path: %v\n", defaultCfgErr)
	}
	configPath := flag.String("config", defaultCfgPath, "path to config file")
	listDevices := flag.Bool("list-devices", false, "list available audio input devices and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.FormatVersion(version.Version))
		return
	}

	if *listDevices {
		if err := InitAudio(); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			os.Exit(1)
		}
		if err := ListInputDevices(); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
			if tErr := TerminateAudio(); tErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to terminate audio: %v\n", tErr)
			}
			os.Exit(1)
		}
		if tErr := TerminateAudio(); tErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to terminate audio: %v\n", tErr)
		}
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
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return strings.Contains(resolved, ".app/Contents/MacOS")
}

// suppressStderr redirects fd 2 to a log file. Runs before the logger
// exists, so errors cannot be logged. Failure is acceptable: whisper.cpp
// noise appears in stderr instead of being captured.
func suppressStderr(logDir string) {
	logPath := filepath.Join(logDir, "whisper-stderr.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return // best-effort: runs before logger exists
	}
	if err := syscall.Dup2(int(f.Fd()), 2); err != nil {
		f.Close()
		return // best-effort: if this fails, whisper.cpp noise appears in stderr
	}
	// Dup2 succeeded — fd 2 now points to the file. Close the original
	// descriptor to avoid leaking it for the process lifetime.
	if closeErr := f.Close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to close original stderr fd: %v\n", closeErr)
	}
}

// runAppMode is the entry point when running inside a .app bundle.
func runAppMode() {
	logDir, err := config.DefaultConfigDir()
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

	settingsLogger = logger.With("component", "settings")
	logger.Info("app starting", "component", "main", "operation", "runAppMode", "version", version.Version)

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

	var shutdownRequested atomic.Bool
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal", "component", "main", "operation", "signal", "signal", sig.String())
		shutdownRequested.Store(true)
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
	logger.Info("first run check", "component", "main", "operation", "runAppMode", "first_run", firstRun)
	if firstRun {
		selectedDevice, setupErr := RunSetupWizard(startupCtx, logger)
		if setupErr != nil {
			logger.Error("setup wizard failed", "component", "main", "operation", "runAppMode", "error", setupErr)
			os.Exit(1)
		}
		logger.Info("setup complete", "component", "main", "operation", "runAppMode", "device", selectedDevice)
	}

	// Load config (now exists after setup)
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		logger.Error("failed to resolve config path", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}

	// Load model
	modelPath, err := config.DefaultModelPath(cfg.ModelSize)
	if err != nil {
		logger.Error("failed to resolve model path", "component", "main", "operation", "runAppMode", "error", err)
		os.Exit(1)
	}

	// Create status bar on the main thread BEFORE [NSApp run].
	// This is safe — the Accessibility dialog it may trigger is a
	// system-level (WindowServer) dialog, not an AppKit dialog.
	InitStatusBar()
	InitPowerObserver()
	UpdateStatusBar(StateLoading)

	events := make(chan HotkeyEvent, 32)
	hotkey := NewHotkeyListener(cfg.TriggerKey, logger)

	activeHotkeyMu.Lock()
	activeHotkey = hotkey
	activeHotkeyMu.Unlock()

	// Launch all heavy init work in a background goroutine.
	initDone := make(chan error, 1)
	var (
		app         *App
		recorder    Recorder
		transcriber Transcriber
		sound       *Sound
		wg          sync.WaitGroup
	)

	go func() {
		// Step 1: Check permissions silently — never trigger system dialogs.
		// WaitForPermissions polls silently. Our UI has "Open" buttons.
		notified := false
		if err := hotkey.WaitForPermissions(startupCtx, func(acc, inp bool) {
			if acc && inp {
				return
			}
			UpdateStatusBar(StateNoPermission)
			if !notified {
				notified = true
				PostNotification("JoiceTyper needs permissions",
					"Click the JoiceTyper menu bar icon → Preferences to grant Accessibility and Input Monitoring.")
			}
		}); err != nil {
			initDone <- err
			if stopErr := hotkey.Stop(); stopErr != nil {
				logger.Error("failed to stop bare run loop after permission error",
					"component", "main", "operation", "runAppMode", "error", stopErr)
			}
			return
		}

		// Step 2: Load model (may download on first run)
		UpdateStatusBar(StateLoading)
		var tErr error
		transcriber, tErr = NewTranscriber(startupCtx, modelPath, cfg.ModelSize, cfg.Language, cfg.SampleRate, cfg.DecodeMode, cfg.PunctuationMode, logger)
		if tErr != nil {
			initDone <- tErr
			if stopErr := hotkey.Stop(); stopErr != nil {
				logger.Error("failed to stop bare run loop after transcriber error",
					"component", "main", "operation", "runAppMode", "error", stopErr)
			}
			return
		}

		// Set vocabulary for whisper prompt biasing
		if cfg.Vocabulary != "" {
			transcriber.SetVocabulary(cfg.Vocabulary)
		}

		// Step 3: Create app components
		recorder = NewRecorder(cfg.SampleRate, cfg.InputDevice, logger)
		settingsRecorder = recorder // expose to settings.go for safe device refresh
		recorder.Warm()             // pre-open audio stream for instant recording start
		paster := NewPaster(logger)
		sound = NewSound(cfg.SoundFeedback, logger)

		app = NewApp(recorder, transcriber, paster, sound, logger)
		app.SetStateCallback(func(state AppState) {
			UpdateStatusBar(state)
		})
		SetPowerEventHandler(makePowerEventHandler(app, func() Recorder { return recorder }, logger))

		wg.Add(1)
		go func() {
			defer wg.Done()
			app.Run(events)
			app.Shutdown()
		}()

		hotkeyDisplay := formatHotkeyDisplay(cfg.TriggerKey)
		SetStatusBarHotkeyText(strings.ReplaceAll(hotkeyDisplay, " + ", "+"))
		UpdateStatusBar(StateReady)
		sound.PlayReady()
		PostNotification("JoiceTyper is ready", "Hold "+hotkeyDisplay+" to dictate.")
		logger.Info("ready", "component", "main", "operation", "runAppMode", "trigger_key", cfg.TriggerKey)

		initDone <- nil
		if stopErr := hotkey.Stop(); stopErr != nil {
			logger.Error("failed to stop bare run loop",
				"component", "main", "operation", "runAppMode", "error", stopErr)
		}
	}()

	// Main thread: run bare [NSApp run] to stay responsive during init.
	// Re-enter if [NSApp run] exits spuriously (e.g. Preferences closed
	// during init calls [NSApp stop:] via signalHotkeyRestart).
	for {
		logger.Info("entering event loop", "component", "main", "operation", "runAppMode", "phase", "init")
		hotkey.RunMainLoopOnly()
		logger.Info("event loop exited", "component", "main", "operation", "runAppMode", "phase", "init")
		select {
		case err := <-initDone:
			if err != nil {
				logger.Error("startup failed", "component", "main", "operation", "runAppMode", "error", err)
				hotkeyMu.Lock()
				hotkeyEvents = nil
				hotkeyMu.Unlock()
				close(events)
				return
			}
			goto initFinished
		default:
			// Init not done yet — [NSApp run] was stopped by something else
			// (e.g. signalHotkeyRestart from Preferences). Re-enter.
			logger.Info("re-entering event loop (init still in progress)",
				"component", "main", "operation", "runAppMode")
		}
	}
initFinished:

	// Hotkey start/restart loop.
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
			cfg, err = config.LoadConfig(cfgPath)
			if err != nil {
				logger.Error("failed to reload config, keeping current hotkey",
					"component", "main", "operation", "runAppMode", "error", err)
				continue // retry with existing hotkey
			}
			hotkey = NewHotkeyListener(cfg.TriggerKey, logger)
			activeHotkeyMu.Lock()
			activeHotkey = hotkey
			activeHotkeyMu.Unlock()

			// Wait for app to become idle before swapping dependencies
			for i := 0; i < 50; i++ { // 5 seconds max
				if app.IsIdle() {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if !app.IsIdle() {
				logger.Error("app not idle after 5s, skipping dependency swap",
					"component", "main", "operation", "runAppMode")
				UpdateStatusBar(StateReady)
				continue // re-enter hotkey loop with existing config
			}

			// Recreate recorder if input device changed
			if oldCfg.InputDevice != cfg.InputDevice {
				if closeErr := recorder.Close(); closeErr != nil {
					logger.Error("failed to close old recorder", "component", "main", "operation", "runAppMode", "error", closeErr)
				}
				recorder = NewRecorder(cfg.SampleRate, cfg.InputDevice, logger)
				settingsRecorder = recorder // update settings.go reference
				app.SetRecorder(recorder)
				logger.Info("recorder updated", "component", "main", "operation", "runAppMode", "device", cfg.InputDevice)
			}

			// Recreate transcriber if language, model, decode mode, or punctuation mode changed.
			// Create new BEFORE closing old — if creation fails, keep the working one.
			if oldCfg.Language != cfg.Language || oldCfg.ModelSize != cfg.ModelSize ||
				oldCfg.DecodeMode != cfg.DecodeMode || oldCfg.PunctuationMode != cfg.PunctuationMode {
				newModelPath, pathErr := config.DefaultModelPath(cfg.ModelSize)
				if pathErr != nil {
					logger.Error("failed to resolve model path for transcriber reload",
						"component", "main", "operation", "runAppMode", "error", pathErr)
					continue
				}
				UpdateStatusBar(StateLoading)
				PostNotification("JoiceTyper", "Loading speech model...")
				reloadCtx, reloadCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				newTranscriber, tErr := NewTranscriber(reloadCtx, newModelPath, cfg.ModelSize, cfg.Language, cfg.SampleRate, cfg.DecodeMode, cfg.PunctuationMode, logger)
				reloadCancel()
				if tErr != nil {
					logger.Error("failed to recreate transcriber, keeping old",
						"component", "main", "operation", "runAppMode", "error", tErr)
				} else {
					oldTranscriber := transcriber
					transcriber = newTranscriber
					newTranscriber.SetVocabulary(cfg.Vocabulary)
					app.SetTranscriber(newTranscriber)
					logger.Info("transcriber updated", "component", "main", "operation", "runAppMode",
						"language", cfg.Language, "decode_mode", cfg.DecodeMode, "punctuation_mode", cfg.PunctuationMode)
					if closeErr := oldTranscriber.Close(); closeErr != nil {
						logger.Error("failed to close old transcriber", "component", "main", "operation", "runAppMode", "error", closeErr)
					}
				}
			}

			// Update vocabulary if changed
			if oldCfg.Vocabulary != cfg.Vocabulary {
				transcriber.SetVocabulary(cfg.Vocabulary)
				logger.Info("vocabulary updated", "component", "main", "operation", "runAppMode")
			}

			hotkeyDisplay := formatHotkeyDisplay(cfg.TriggerKey)
			SetStatusBarHotkeyText(strings.ReplaceAll(hotkeyDisplay, " + ", "+"))
			UpdateStatusBar(StateReady)
			continue
		default:
			if shutdownRequested.Load() {
				break // real shutdown
			}
			// [NSApp stopModal] from preferences bled into [NSApp run].
			// Re-enter — startHotkeyListener cleans up the old tap.
			logger.Info("re-entering hotkey listener after modal exit",
				"component", "main", "operation", "runAppMode")
			continue
		}
		break
	}

	hotkeyMu.Lock()
	hotkeyEvents = nil
	hotkeyMu.Unlock()
	SetPowerEventHandler(nil)
	close(events)
	wg.Wait()

	logger.Info("voicetype stopped", "component", "main", "operation", "runAppMode")
}

// runTerminalMode is the entry point for CLI invocations.
func runTerminalMode(configPath string) {
	// --- Load config ---
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// --- Resolve log directory ---
	logDir, err := config.DefaultConfigDir()
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
		"version", version.Version,
		"config_path", configPath,
		"model_size", cfg.ModelSize,
		"trigger_key", cfg.TriggerKey,
		"language", cfg.Language,
		"sample_rate", cfg.SampleRate,
		"decode_mode", cfg.DecodeMode,
		"punctuation_mode", cfg.PunctuationMode,
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
	modelPath, err := config.DefaultModelPath(cfg.ModelSize)
	if err != nil {
		logger.Error("failed to resolve model path",
			"component", "main", "operation", "runTerminalMode", "error", err)
		os.Exit(1)
	}

	// --- Signal handling (before model init so SIGTERM can cancel download) ---
	startupCtx, startupCancel := context.WithCancel(context.Background())
	events := make(chan HotkeyEvent, 32)
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
	transcriber, err := NewTranscriber(startupCtx, modelPath, cfg.ModelSize, cfg.Language, cfg.SampleRate, cfg.DecodeMode, cfg.PunctuationMode, logger)
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

	// --- Create app ---
	app := NewApp(recorder, transcriber, paster, sound, logger)

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
