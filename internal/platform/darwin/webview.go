//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include "webview_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unsafe"

	audiopkg "voicetype/internal/core/audio"
	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	loggingpkg "voicetype/internal/core/logging"
	transcriptionpkg "voicetype/internal/core/transcription"
	versionpkg "voicetype/internal/core/version"
	uiembed "voicetype/ui"
)

var (
	webSettingsEnabled = func() bool {
		if os.Getenv("JOICETYPER_USE_NATIVE_PREFERENCES") == "1" {
			return false
		}
		if value := os.Getenv("JOICETYPER_USE_WEB_SETTINGS"); value != "" {
			return value == "1"
		}
		return true
	}
	webSettingsDefaultConfigPath = configpkg.DefaultConfigPath
	webSettingsLoadConfig        = configpkg.LoadConfig
	webSettingsSaveConfig        = configpkg.SaveConfig
	webSettingsSignalRestart     = signalHotkeyRestart
	webSettingsPostError         = reportSettingsSaveError
	webSettingsLoadPermissions   = loadWebSettingsPermissionsSnapshot
	webSettingsListInputDevices  = listWebSettingsInputDevices
	webSettingsRefreshDevices    = refreshWebSettingsDevices
	webSettingsDownloadModel     = downloadWebSettingsModel
	webSettingsDeleteModel       = deleteWebSettingsModel
	webSettingsUseModel          = useWebSettingsModel
	webSettingsDispatchEnvelope  = func(payload string, closeWindow bool) {
		cPayload := C.CString(payload)
		defer C.free(unsafe.Pointer(cPayload))
		closeWindowFlag := 0
		if closeWindow {
			closeWindowFlag = 1
		}
		C.dispatchWebSettingsEnvelope(cPayload, C.int(closeWindowFlag))
	}

	embeddedWebUIHTMLMu       sync.Mutex
	embeddedWebUIHTMLTemplate []byte
	embeddedWebUIHTMLInitErr  error

	webHotkeyCaptureMu    sync.Mutex
	webHotkeyCaptureState bridgepkg.HotkeyCaptureSnapshot

	// lastDispatchedEventMu guards lastDispatchedEvent; it is read from the
	// C completion handler thread (via webSettingsNativeTransportWarning) and
	// written from the Go dispatch path. Holding this reliably associates a
	// JS eval failure with the event that triggered it.
	lastDispatchedEventMu sync.Mutex
	lastDispatchedEvent   string

	// webSettingsWarningThrottle rate-limits noisy warnings from the native
	// transport (e.g. repeated bridge dispatch failures when the webview is
	// loading or an event payload is malformed). Keyed by (operation,event)
	// so unrelated failures stay visible.
	webSettingsWarningThrottle = loggingpkg.NewThrottler(10 * time.Second)

	webSettingsServiceMu sync.Mutex
	webSettingsService   *bridgepkg.Service
)

var (
	embeddedWebUIScriptPattern = regexp.MustCompile(`<script[^>]*src="\./assets/([^"]+\.js)"[^>]*></script>`)
	embeddedWebUIStylePattern  = regexp.MustCompile(`<link[^>]*rel="stylesheet"[^>]*href="\./assets/([^"]+\.css)"[^>]*>`)
	embeddedWebUIHeadPattern   = regexp.MustCompile(`(?i)<head[^>]*>`)
)

func shouldUseWebSettings() bool {
	return webSettingsEnabled()
}

func ShowWebSettingsWindow() error {
	return ShowWebSettingsWindowWithBridge(context.Background(), nil)
}

func FocusWebSettingsWindow() {
	C.focusWebSettingsWindow()
}

func ShowWebSettingsWindowWithBridge(ctx context.Context, service *bridgepkg.Service) error {
	if service == nil {
		service = buildSettingsBridgeService(configpkg.Config{})
	}
	setActiveWebSettingsBridgeService(service)
	html, err := renderEmbeddedWebUI(ctx, service)
	if err != nil {
		clearActiveWebSettingsBridgeService()
		return fmt.Errorf("darwin.ShowWebSettingsWindowWithBridge: %w", err)
	}

	cHTML := C.CString(html)
	defer C.free(unsafe.Pointer(cHTML))

	C.showWebSettingsWindow(cHTML)
	return nil
}

func setActiveWebSettingsBridgeService(service *bridgepkg.Service) {
	webSettingsServiceMu.Lock()
	webSettingsService = service
	webSettingsServiceMu.Unlock()
}

func defaultWebSettingsBridgeService() *bridgepkg.Service {
	return buildSettingsBridgeService(configpkg.Config{})
}

func activeWebSettingsBridgeService() (*bridgepkg.Service, bool) {
	webSettingsServiceMu.Lock()
	defer webSettingsServiceMu.Unlock()
	if webSettingsService == nil {
		return nil, false
	}
	return webSettingsService, true
}

func clearActiveWebSettingsBridgeService() {
	webSettingsServiceMu.Lock()
	webSettingsService = nil
	webSettingsServiceMu.Unlock()
}

func loadWebSettingsMachineInfo() bridgepkg.MachineInfoSnapshot {
	snapshot := bridgepkg.MachineInfoSnapshot{
		Platform:          "darwin",
		WhisperSystemInfo: transcriptionpkg.WhisperSystemInfo(),
	}
	if out, err := exec.Command("/usr/sbin/sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		snapshot.CPUModel = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.model").Output(); err == nil {
		snapshot.MachineModel = strings.TrimSpace(string(out))
	}
	if snapshot.Chip == "" {
		snapshot.Chip = snapshot.CPUModel
	}
	return snapshot
}

// darwinPlatform implements bridgepkg.Platform. The compile-time assertion
// below means adding a new method to bridgepkg.Platform breaks this build
// until the corresponding method is implemented here. That is the whole
// point of converting the old nullable Dependencies struct into an
// interface — drift becomes impossible.
type darwinPlatform struct{}

var _ bridgepkg.Platform = darwinPlatform{}

func buildSettingsBridgeService(_ configpkg.Config) *bridgepkg.Service {
	return bridgepkg.NewService(darwinPlatform{})
}

func (darwinPlatform) LoadConfig(context.Context) (configpkg.Config, error) {
	cfgPath, err := webSettingsDefaultConfigPath()
	if err != nil {
		return configpkg.Config{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeConfigLoadFailure,
			"Failed to resolve config path",
			false, nil, err,
		)
	}
	loaded, err := webSettingsLoadConfig(cfgPath)
	if err != nil {
		return configpkg.Config{}, bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeConfigLoadFailure,
			"Failed to load config",
			false, nil, err,
		)
	}
	return loaded, nil
}

func (darwinPlatform) SaveConfig(_ context.Context, updated configpkg.Config) error {
	if err := updated.Validate(); err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeConfigInvalid,
			"Config validation failed",
			false, nil, err,
		)
	}
	if err := applyWebSettingsConfig(bridgepkg.ConfigSnapshot{
		TriggerKey:      append([]string(nil), updated.TriggerKey...),
		ModelSize:       updated.ModelSize,
		Language:        updated.Language,
		OutputMode:      updated.OutputMode,
		SampleRate:      updated.SampleRate,
		SoundFeedback:   updated.SoundFeedback,
		InputDevice:     updated.InputDevice,
		DecodeMode:      updated.DecodeMode,
		PunctuationMode: updated.PunctuationMode,
		Vocabulary:      updated.Vocabulary,
	}); err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeSaveFailure,
			"Failed to save config",
			false, nil, err,
		)
	}
	return nil
}

func (darwinPlatform) LoadAppState(context.Context) (AppState, error) {
	return currentAppState(), nil
}

func (darwinPlatform) LoadPermissions(context.Context) (bridgepkg.PermissionsSnapshot, error) {
	return webSettingsLoadPermissions(), nil
}

func (darwinPlatform) LoadMachineInfo(context.Context) (bridgepkg.MachineInfoSnapshot, error) {
	return loadWebSettingsMachineInfo(), nil
}

func (darwinPlatform) OpenPermissionSettings(_ context.Context, target string) error {
	if err := webSettingsOpenPermissionSettings(target); err != nil {
		if _, ok := bridgepkg.AsContractError(err); ok {
			return err
		}
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodePermissionOpenFailed,
			"Failed to open system permission settings",
			true,
			map[string]any{"target": target},
			err,
		)
	}
	return nil
}

func (darwinPlatform) ListDevices(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
	return webSettingsListInputDevices()
}

func (darwinPlatform) RefreshDevices(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
	return webSettingsRefreshDevices()
}

func (darwinPlatform) LoadModel(context.Context) (bridgepkg.ModelSnapshot, error) {
	return loadActiveWebSettingsModelSnapshot()
}

func (darwinPlatform) DownloadModel(ctx context.Context, size string) error {
	return webSettingsDownloadModel(ctx, size)
}

func (darwinPlatform) DeleteModel(_ context.Context, size string) error {
	return webSettingsDeleteModel(size)
}

func (darwinPlatform) UseModel(_ context.Context, size string) error {
	return webSettingsUseModel(size)
}

func (darwinPlatform) LoadLogsTail(context.Context) (bridgepkg.LogTailSnapshot, error) {
	return loadWebSettingsLogTailSnapshot()
}

func (darwinPlatform) LoadLogsFull(context.Context) (string, error) {
	return loadWebSettingsLogFullText()
}

func (darwinPlatform) WriteClipboardText(_ context.Context, text string) error {
	return copyTextToClipboard(text)
}

func (darwinPlatform) LoadUpdater(context.Context) (bridgepkg.UpdaterSnapshot, error) {
	return currentUpdaterSnapshot(), nil
}

func (darwinPlatform) CheckForUpdates(context.Context) error {
	return CheckForUpdates()
}

func (darwinPlatform) GetLoginItem(context.Context) (bridgepkg.LoginItemSnapshot, error) {
	return webSettingsGetLoginItem(), nil
}

func (darwinPlatform) SetLoginItem(_ context.Context, enabled bool) (bridgepkg.LoginItemSnapshot, error) {
	return webSettingsSetLoginItem(enabled)
}

func (darwinPlatform) GetInputVolume(_ context.Context, deviceName string) (bridgepkg.InputVolumeSnapshot, error) {
	return webSettingsGetInputVolume(deviceName), nil
}

func (darwinPlatform) SetInputVolume(_ context.Context, deviceName string, volume float64) (bridgepkg.InputVolumeSnapshot, error) {
	return webSettingsSetInputVolume(deviceName, volume)
}

func (darwinPlatform) StartHotkeyCapture(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {
	return startWebSettingsHotkeyCapture(), nil
}

func (darwinPlatform) CancelHotkeyCapture(context.Context) error {
	cancelWebSettingsHotkeyCapture()
	return nil
}

func (darwinPlatform) ConfirmHotkeyCapture(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {
	return confirmWebSettingsHotkeyCapture()
}

func (darwinPlatform) SetAudioInputMonitor(_ context.Context, inputDevice string) error {
	cfgPath, err := webSettingsDefaultConfigPath()
	if err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeSaveFailure,
			"Failed to resolve config path",
			false, nil, err,
		)
	}
	cfg, err := webSettingsLoadConfig(cfgPath)
	if err != nil {
		return bridgepkg.WrapContractError(
			bridgepkg.ErrorCodeConfigLoadFailure,
			"Failed to load config",
			false, nil, err,
		)
	}
	deviceName := inputDevice
	if deviceName == "" {
		deviceName = cfg.InputDevice
	}
	// Capture and clear the old monitor before spawning. Pa_OpenStream
	// and Pa_StartStream interact with Core Audio, which needs the main
	// run loop on macOS. Running them synchronously inside a
	// WKScriptMessageHandler callback (main thread) blocks the run loop
	// and kills the window. Do all PortAudio work off the main thread.
	oldMonitor := currentSettingsInputMonitor()
	if oldMonitor != nil {
		setSettingsInputMonitor(nil)
	}
	sampleRate := cfg.SampleRate
	ensureWebSettingsInputLevelPublisher()
	go func() {
		if oldMonitor != nil {
			if err := oldMonitor.Close(); err != nil {
				currentSettingsLogger().Warn("failed to close previous input monitor",
					"operation", "SetAudioInputMonitor", "error", err)
			}
		}
		currentSettingsLogger().Info("starting input monitor",
			"operation", "SetAudioInputMonitor",
			"device", deviceName,
			"sample_rate", sampleRate)
		monitor, err := audiopkg.NewInputLevelMonitor(sampleRate, deviceName, currentSettingsLogger())
		if err != nil {
			currentSettingsLogger().Warn("failed to start monitored input device",
				"operation", "SetAudioInputMonitor", "device", deviceName, "error", err)
			publishInputLevelChanged(bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"})
			return
		}
		setSettingsInputMonitor(monitor)
		publishInputLevelChanged(monitor.Snapshot())
		// The microphone permission dialog (first run) causes the
		// preferences window to lose activation and go behind other
		// windows. Bring it back to front now that the stream is live.
		C.focusWebSettingsWindow()
	}()
	return nil
}

func (darwinPlatform) StopAudioInputMonitor(context.Context) error {
	monitor := currentSettingsInputMonitor()
	if monitor == nil {
		currentSettingsLogger().Debug("stop requested but no active input monitor",
			"operation", "StopAudioInputMonitor")
		publishInputLevelChanged(bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"})
		return nil
	}
	currentSettingsLogger().Info("stopping input monitor",
		"operation", "StopAudioInputMonitor")
	// Clear the reference and return immediately — Close blocks on
	// Pa_StopStream which needs the main run loop on macOS.
	setSettingsInputMonitor(nil)
	publishInputLevelChanged(bridgepkg.InputLevelSnapshot{Level: 0, Quality: "poor"})
	go func() {
		if err := monitor.Close(); err != nil {
			currentSettingsLogger().Warn("failed to close input monitor",
				"operation", "StopAudioInputMonitor", "error", err)
		}
	}()
	return nil
}

func renderEmbeddedWebUI(ctx context.Context, service *bridgepkg.Service) (string, error) {
	bootstrap, err := buildBootstrapPayload(ctx, service)
	if err != nil {
		return "", err
	}

	indexHTML, err := embeddedWebUIBaseHTML()
	if err != nil {
		return "", err
	}
	indexHTML, err = injectBootstrapScript(indexHTML, bootstrap)
	if err != nil {
		return "", fmt.Errorf("inject bootstrap payload: %w", err)
	}
	return string(indexHTML), nil
}

func embeddedWebUIBaseHTML() ([]byte, error) {
	embeddedWebUIHTMLMu.Lock()
	defer embeddedWebUIHTMLMu.Unlock()
	if embeddedWebUIHTMLTemplate != nil || embeddedWebUIHTMLInitErr != nil {
		return embeddedWebUIHTMLTemplate, embeddedWebUIHTMLInitErr
	}
	indexHTML, err := uiembed.EmbeddedAssets.ReadFile("dist/index.html")
	if err != nil {
		embeddedWebUIHTMLInitErr = fmt.Errorf("read embedded index.html: %w", err)
		return nil, embeddedWebUIHTMLInitErr
	}
	indexHTML, err = inlineEmbeddedAssetReferences(indexHTML, uiembed.EmbeddedAssets.ReadFile)
	if err != nil {
		embeddedWebUIHTMLInitErr = fmt.Errorf("inline embedded UI assets: %w", err)
		return nil, embeddedWebUIHTMLInitErr
	}
	embeddedWebUIHTMLTemplate = indexHTML
	return embeddedWebUIHTMLTemplate, nil
}

func inlineEmbeddedAssetReferences(indexHTML []byte, readFile func(string) ([]byte, error)) ([]byte, error) {
	html := string(indexHTML)

	inlinePattern := func(pattern *regexp.Regexp, replacer func(string) string) (string, error) {
		matches := pattern.FindAllStringSubmatchIndex(html, -1)
		if len(matches) == 0 {
			return html, nil
		}

		var out strings.Builder
		last := 0
		for _, match := range matches {
			if len(match) < 4 {
				return "", fmt.Errorf("unexpected asset match shape")
			}
			out.WriteString(html[last:match[0]])
			assetName := html[match[2]:match[3]]
			data, err := readFile(filepath.ToSlash(filepath.Join("dist", "assets", assetName)))
			if err != nil {
				return "", fmt.Errorf("read embedded asset %q: %w", assetName, err)
			}
			out.WriteString(replacer(string(data)))
			last = match[1]
		}
		out.WriteString(html[last:])
		return out.String(), nil
	}

	var err error
	html, err = inlinePattern(embeddedWebUIStylePattern, func(css string) string {
		return "<style>" + css + "</style>"
	})
	if err != nil {
		return nil, err
	}
	html, err = inlinePattern(embeddedWebUIScriptPattern, func(js string) string {
		return `<script type="module">` + strings.ReplaceAll(js, "</script>", "<\\/script>") + `</script>`
	})
	if err != nil {
		return nil, err
	}
	return []byte(html), nil
}

func buildBootstrapPayload(ctx context.Context, service *bridgepkg.Service) (bridgepkg.BootstrapPayload, error) {
	if service == nil {
		// Defensive: callers should pass a real service. We construct one
		// from the real darwinPlatform rather than NewService(nil) — passing
		// nil now nil-derefs (the old code returned ErrorCodeInternal because
		// dependencies were nullable, which is no longer true).
		service = bridgepkg.NewService(darwinPlatform{})
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		return bridgepkg.BootstrapPayload{}, fmt.Errorf("build bootstrap payload: %w", err)
	}
	return bootstrap, nil
}

func injectBootstrapScript(indexHTML []byte, bootstrap bridgepkg.BootstrapPayload) ([]byte, error) {
	payload, err := json.Marshal(bridgepkg.NewSuccessResponse(bridgepkg.BootstrapMethod, bootstrap))
	if err != nil {
		return nil, fmt.Errorf("marshal bootstrap payload: %w", err)
	}

	script := `<script>window.__JOICETYPER_BOOTSTRAP__ = ` + string(payload) + `;</script>`
	html := string(indexHTML)
	if loc := embeddedWebUIHeadPattern.FindStringIndex(html); loc != nil {
		return []byte(html[:loc[1]] + script + "\n" + html[loc[1]:]), nil
	}
	if strings.Contains(html, "</head>") {
		return []byte(strings.Replace(html, "</head>", script+"\n</head>", 1)), nil
	}
	return append(indexHTML, []byte(script)...), nil
}

type webSettingsParams struct {
	Config bridgepkg.ConfigSnapshot `json:"config"`
}

type webSettingsResponse = bridgepkg.ResponseEnvelope

func applyWebSettingsConfig(snapshot bridgepkg.ConfigSnapshot) error {
	cfgPath, err := webSettingsDefaultConfigPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg := configpkg.Config{
		TriggerKey:      append([]string(nil), snapshot.TriggerKey...),
		ModelSize:       snapshot.ModelSize,
		Language:        snapshot.Language,
		OutputMode:      snapshot.OutputMode,
		SampleRate:      snapshot.SampleRate,
		SoundFeedback:   snapshot.SoundFeedback,
		InputDevice:     snapshot.InputDevice,
		DecodeMode:      snapshot.DecodeMode,
		PunctuationMode: snapshot.PunctuationMode,
		Vocabulary:      snapshot.Vocabulary,
	}
	if err := webSettingsSaveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	publishConfigSaved(snapshot)
	webSettingsSignalRestart()
	return nil
}

//export webSettingsHotkeyCaptureChanged
func webSettingsHotkeyCaptureChanged(flags C.uint64_t, keycode C.int, recording C.int) {
	keys := hotkeyToKeys(uint64(flags), int(keycode))
	snapshot := hotkeyCaptureSnapshot(keys, recording == 1)
	setWebHotkeyCaptureState(snapshot)
	// Debug-level: this fires on every modifier press/release during capture.
	// Info would be too noisy; Debug still gives us a timeline when debugging.
	currentSettingsLogger().Debug("hotkey capture state changed",
		"operation", "webSettingsHotkeyCaptureChanged",
		"flags", fmt.Sprintf("0x%x", uint64(flags)),
		"keycode", int(keycode),
		"keys", keys,
		"recording", recording == 1)
	publishHotkeyCaptureChanged(snapshot)
}

func hotkeyCaptureSnapshot(keys []string, recording bool) bridgepkg.HotkeyCaptureSnapshot {
	display := "Press keys..."
	if len(keys) > 0 {
		display = FormatHotkeyDisplay(keys)
	} else if !recording {
		display = ""
	}
	return bridgepkg.HotkeyCaptureSnapshot{
		TriggerKey: append([]string(nil), keys...),
		Display:    display,
		Recording:  recording,
		CanConfirm: len(keys) > 0,
	}
}

func setWebHotkeyCaptureState(snapshot bridgepkg.HotkeyCaptureSnapshot) {
	webHotkeyCaptureMu.Lock()
	webHotkeyCaptureState = snapshot
	webHotkeyCaptureMu.Unlock()
}

func currentWebHotkeyCaptureState() bridgepkg.HotkeyCaptureSnapshot {
	webHotkeyCaptureMu.Lock()
	defer webHotkeyCaptureMu.Unlock()
	return webHotkeyCaptureState
}

func startWebSettingsHotkeyCapture() bridgepkg.HotkeyCaptureSnapshot {
	currentSettingsLogger().Info("hotkey capture started",
		"operation", "startWebSettingsHotkeyCapture")
	snapshot := hotkeyCaptureSnapshot(nil, true)
	setWebHotkeyCaptureState(snapshot)
	publishHotkeyCaptureChanged(snapshot)
	C.startWebHotkeyCapture()
	return snapshot
}

func cancelWebSettingsHotkeyCapture() {
	currentSettingsLogger().Info("hotkey capture cancelled",
		"operation", "cancelWebSettingsHotkeyCapture")
	C.cancelWebHotkeyCapture()
	snapshot := bridgepkg.HotkeyCaptureSnapshot{}
	setWebHotkeyCaptureState(snapshot)
	publishHotkeyCaptureChanged(snapshot)
}

func confirmWebSettingsHotkeyCapture() (bridgepkg.HotkeyCaptureSnapshot, error) {
	var flags C.uint64_t
	var keycode C.int
	if C.confirmWebHotkeyCapture(&flags, &keycode) == 0 {
		currentSettingsLogger().Warn("hotkey capture confirm failed — no keys captured",
			"operation", "confirmWebSettingsHotkeyCapture")
		return bridgepkg.HotkeyCaptureSnapshot{}, bridgepkg.NewContractError(
			bridgepkg.ErrorCodeHotkeyCaptureConfirmFailed,
			"No hotkey was captured",
			false,
			nil,
		)
	}
	keys := hotkeyToKeys(uint64(flags), int(keycode))
	snapshot := hotkeyCaptureSnapshot(keys, false)
	setWebHotkeyCaptureState(snapshot)
	currentSettingsLogger().Info("hotkey capture confirmed",
		"operation", "confirmWebSettingsHotkeyCapture",
		"flags", fmt.Sprintf("0x%x", uint64(flags)),
		"keycode", int(keycode),
		"keys", keys)
	publishHotkeyCaptureChanged(snapshot)
	return snapshot, nil
}

//export webSettingsWindowClosed
func webSettingsWindowClosed() {
	// Tear down any hotkey capture tap and mic monitor left open by the user
	// closing the window mid-capture (e.g. via the red X). Safe to call even
	// when nothing is active.
	C.cancelWebHotkeyCapture()
	if monitor := currentSettingsInputMonitor(); monitor != nil {
		setSettingsInputMonitor(nil)
		go func() {
			if err := monitor.Close(); err != nil {
				currentSettingsLogger().Warn("failed to close input monitor on window close", "operation", "webSettingsWindowClosed", "error", err)
			}
		}()
	}
	setWebHotkeyCaptureState(bridgepkg.HotkeyCaptureSnapshot{})
	cancelPreferencesContext()
	clearActiveWebSettingsBridgeService()
	preferencesOpenStore(0)
}

func webSettingsResponseJSON(response webSettingsResponse) string {
	payload, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		fallback := bridgepkg.NewErrorResponse(response.ID, bridgepkg.ErrorCodeInternal, marshalErr.Error(), false, nil)
		fallbackPayload, _ := json.Marshal(fallback)
		return string(fallbackPayload)
	}
	return string(payload)
}

func webSettingsEventPayload(event bridgepkg.EventEnvelope) string {
	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		currentSettingsLogger().Warn("failed to marshal bridge event", "event", event.Event, "error", marshalErr)
		return ""
	}
	return string(payload)
}

func dispatchWebSettingsEvent(event bridgepkg.EventEnvelope) {
	payload := webSettingsEventPayload(event)
	if payload == "" {
		return
	}
	lastDispatchedEventMu.Lock()
	lastDispatchedEvent = string(event.Event)
	lastDispatchedEventMu.Unlock()
	currentSettingsLogger().Debug("dispatching bridge event",
		"operation", "dispatchWebSettingsEvent",
		"event", event.Event,
		"payload_bytes", len(payload))
	webSettingsDispatchEnvelope(payload, false)
}

func currentLastDispatchedEvent() string {
	lastDispatchedEventMu.Lock()
	defer lastDispatchedEventMu.Unlock()
	return lastDispatchedEvent
}

func publishRuntimeStateChanged(state AppState) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.RuntimeStateChangedEvent, bridgepkg.AppStateSnapshot{
		State:   state.String(),
		Version: versionpkg.DisplayVersion(),
	}))
}

func publishPermissionsChanged(snapshot bridgepkg.PermissionsSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.PermissionsChangedEvent, snapshot))
}

func publishModelChanged(snapshot bridgepkg.ModelSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.ModelChangedEvent, snapshot))
}

func publishModelDownloadProgress(size string, progress float64, bytesDownloaded, bytesTotal int64) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.ModelDownloadProgressEvent, map[string]any{
		"size":            size,
		"progress":        progress,
		"bytesDownloaded": bytesDownloaded,
		"bytesTotal":      bytesTotal,
	}))
}

func publishModelDownloadCompleted(size string) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.ModelDownloadCompletedEvent, map[string]any{
		"size": size,
	}))
}

func publishModelDownloadFailed(size string, message string, retriable bool) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.ModelDownloadFailedEvent, map[string]any{
		"size":      size,
		"message":   message,
		"retriable": retriable,
	}))
}

func publishConfigSaved(snapshot bridgepkg.ConfigSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.ConfigSavedEvent, snapshot))
}

func publishDevicesChanged(devices []bridgepkg.DeviceSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.DevicesChangedEvent, devices))
}

func publishLogsUpdated(snapshot bridgepkg.LogTailSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.LogsUpdatedEvent, snapshot))
}

func publishHotkeyCaptureChanged(snapshot bridgepkg.HotkeyCaptureSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.HotkeyCaptureChangedEvent, snapshot))
}

func publishInputLevelChanged(snapshot bridgepkg.InputLevelSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.InputLevelChangedEvent, snapshot))
}

type webSettingsProcessResult struct {
	response    webSettingsResponse
	closeWindow bool
}

func processWebSettingsMessage(messageJSON string) webSettingsProcessResult {
	var request bridgepkg.RequestEnvelope
	if err := decodeStrictBridgeEnvelope([]byte(messageJSON), &request); err != nil {
		webSettingsPostError(err.Error())
		return webSettingsProcessResult{response: bridgepkg.NewErrorResponse("", bridgepkg.ErrorCodeBadRequest, err.Error(), false, nil)}
	}
	service, ok := activeWebSettingsBridgeService()
	if !ok {
		response := bridgepkg.NewErrorResponse(request.ID, bridgepkg.ErrorCodeInternal, "preferences bridge session is closed", false, nil)
		return webSettingsProcessResult{response: response}
	}
	ctx := currentPreferencesContext()
	if ctx == nil {
		ctx = context.Background()
	}
	router := bridgepkg.NewRouter(service)
	response := router.HandleRequest(ctx, request)
	if !response.OK && response.Error != nil {
		webSettingsPostError(response.Error.Message)
	}
	return webSettingsProcessResult{
		response:    response,
		closeWindow: response.OK && request.Method == bridgepkg.SaveConfigMethod,
	}
}

func decodeStrictBridgeEnvelope(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("unexpected trailing JSON content")
	}
	return nil
}

//export handleWebSettingsMessage
func handleWebSettingsMessage(messageJSON *C.char, closeWindow *C.int) *C.char {
	if closeWindow != nil {
		*closeWindow = 0
	}
	if messageJSON == nil {
		response := bridgepkg.NewErrorResponse("", bridgepkg.ErrorCodeBadRequest, "missing web settings message", false, nil)
		return C.CString(webSettingsResponseJSON(response))
	}

	result := processWebSettingsMessage(C.GoString(messageJSON))
	if closeWindow != nil && result.closeWindow {
		*closeWindow = 1
	}
	return C.CString(webSettingsResponseJSON(result.response))
}

//export webSettingsNativeTransportWarning
func webSettingsNativeTransportWarning(operation *C.char, message *C.char) {
	op := "<unknown>"
	msg := "<unknown>"
	if operation != nil {
		op = C.GoString(operation)
	}
	if message != nil {
		msg = C.GoString(message)
	}
	event := currentLastDispatchedEvent()
	key := op + "::" + event
	attrs := []any{
		slog.String("operation", op),
		slog.String("message", msg),
	}
	if event != "" {
		attrs = append(attrs, slog.String("event", event))
	}
	webSettingsWarningThrottle.Log(currentSettingsLogger(), slog.LevelWarn, key,
		"web settings native transport warning", attrs...)
}

//export webSettingsNativeTransportInfo
func webSettingsNativeTransportInfo(operation *C.char, message *C.char) {
	op := "<unknown>"
	msg := "<unknown>"
	if operation != nil {
		op = C.GoString(operation)
	}
	if message != nil {
		msg = C.GoString(message)
	}
	currentSettingsLogger().Info("web settings native transport info", "operation", op, "message", msg)
}
