//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include "webview_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unsafe"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
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
	webSettingsCleanupDir        = os.RemoveAll
	webSettingsLoadPermissions   = loadWebSettingsPermissionsSnapshot
	webSettingsListInputDevices  = listWebSettingsInputDevices
	webSettingsRefreshDevices    = refreshWebSettingsDevices
	webSettingsDownloadModel     = downloadWebSettingsModel
	webSettingsDeleteModel       = deleteWebSettingsModel
	webSettingsUseModel          = useWebSettingsModel
	webSettingsDispatchScript    = func(script string) {
		cScript := C.CString(script)
		defer C.free(unsafe.Pointer(cScript))
		C.dispatchWebSettingsScript(cScript)
	}

	webSettingsAssetsMu   sync.Mutex
	webSettingsAssetsRoot string

	webHotkeyCaptureMu    sync.Mutex
	webHotkeyCaptureState bridgepkg.HotkeyCaptureSnapshot

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

func ShowWebSettingsWindowWithBridge(ctx context.Context, service *bridgepkg.Service) error {
	if service == nil {
		service = buildSettingsBridgeService(configpkg.Config{})
	}
	setActiveWebSettingsBridgeService(service)
	indexPath, err := materializeEmbeddedWebUI(ctx, service)
	if err != nil {
		clearActiveWebSettingsBridgeService()
		return fmt.Errorf("darwin.ShowWebSettingsWindowWithBridge: %w", err)
	}

	cIndexPath := C.CString(indexPath)
	defer C.free(unsafe.Pointer(cIndexPath))

	C.showWebSettingsWindow(cIndexPath)
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

func activeWebSettingsBridgeService() *bridgepkg.Service {
	webSettingsServiceMu.Lock()
	defer webSettingsServiceMu.Unlock()
	if webSettingsService == nil {
		return defaultWebSettingsBridgeService()
	}
	return webSettingsService
}

func clearActiveWebSettingsBridgeService() {
	webSettingsServiceMu.Lock()
	webSettingsService = nil
	webSettingsServiceMu.Unlock()
}

func buildSettingsBridgeService(_ configpkg.Config) *bridgepkg.Service {
	return bridgepkg.NewService(&bridgepkg.Dependencies{
		LoadConfig: func(context.Context) (configpkg.Config, error) {
			cfgPath, err := webSettingsDefaultConfigPath()
			if err != nil {
				return configpkg.Config{}, bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigLoadFailure,
					"Failed to resolve config path",
					false,
					nil,
					err,
				)
			}
			loaded, err := webSettingsLoadConfig(cfgPath)
			if err != nil {
				return configpkg.Config{}, bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigLoadFailure,
					"Failed to load config",
					false,
					nil,
					err,
				)
			}
			return loaded, nil
		},
		SaveConfig: func(_ context.Context, updated configpkg.Config) error {
			if err := updated.Validate(); err != nil {
				return bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigInvalid,
					"Config validation failed",
					false,
					nil,
					err,
				)
			}
			if err := applyWebSettingsConfig(bridgepkg.ConfigSnapshot{
				TriggerKey:      append([]string(nil), updated.TriggerKey...),
				ModelSize:       updated.ModelSize,
				Language:        updated.Language,
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
					false,
					nil,
					err,
				)
			}
			return nil
		},
		LoadAppState: func(context.Context) (AppState, error) {
			return currentAppState(), nil
		},
		LoadPermissions: func(context.Context) (bridgepkg.PermissionsSnapshot, error) {
			return webSettingsLoadPermissions(), nil
		},
		OpenPermissionSettings: func(ctx context.Context, target string) error {
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
		},
		ListDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
			return webSettingsListInputDevices()
		},
		RefreshDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {
			return webSettingsRefreshDevices()
		},
		LoadModel: func(context.Context) (bridgepkg.ModelSnapshot, error) {
			cfgPath, err := webSettingsDefaultConfigPath()
			if err != nil {
				return bridgepkg.ModelSnapshot{}, bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigLoadFailure,
					"Failed to resolve config path",
					false,
					nil,
					err,
				)
			}
			currentCfg, err := webSettingsLoadConfig(cfgPath)
			if err != nil {
				return bridgepkg.ModelSnapshot{}, bridgepkg.WrapContractError(
					bridgepkg.ErrorCodeConfigLoadFailure,
					"Failed to load config",
					false,
					nil,
					err,
				)
			}
			return loadWebSettingsModelSnapshot(currentCfg.ModelSize)
		},
		DownloadModel: func(ctx context.Context, size string) error {
			return webSettingsDownloadModel(size)
		},
		DeleteModel: func(ctx context.Context, size string) error {
			return webSettingsDeleteModel(size)
		},
		UseModel: func(ctx context.Context, size string) error {
			return webSettingsUseModel(size)
		},
		StartHotkeyCapture: func(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {
			return startWebSettingsHotkeyCapture(), nil
		},
		CancelHotkeyCapture: func(context.Context) error {
			cancelWebSettingsHotkeyCapture()
			return nil
		},
		ConfirmHotkeyCapture: func(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {
			return confirmWebSettingsHotkeyCapture()
		},
	})
}

func materializeEmbeddedWebUI(ctx context.Context, service *bridgepkg.Service) (string, error) {
	root, err := os.MkdirTemp("", "joicetyper-web-ui-*")
	if err != nil {
		return "", fmt.Errorf("create temp UI dir: %w", err)
	}

	bootstrap, err := buildBootstrapPayload(ctx, service)
	if err != nil {
		return "", err
	}

	indexHTML, err := uiembed.EmbeddedAssets.ReadFile("dist/index.html")
	if err != nil {
		return "", fmt.Errorf("read embedded index.html: %w", err)
	}
	indexHTML, err = inlineEmbeddedAssetReferences(indexHTML, uiembed.EmbeddedAssets.ReadFile)
	if err != nil {
		return "", fmt.Errorf("inline embedded UI assets: %w", err)
	}
	indexHTML, err = injectBootstrapScript(indexHTML, bootstrap)
	if err != nil {
		return "", fmt.Errorf("inject bootstrap payload: %w", err)
	}

	targetPath := filepath.Join(root, "index.html")
	if err := os.WriteFile(targetPath, indexHTML, 0644); err != nil {
		return "", fmt.Errorf("write embedded index.html: %w", err)
	}

	trackWebSettingsAssetsRoot(root)
	return filepath.Join(root, "index.html"), nil
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
		service = bridgepkg.NewService(nil)
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
	snapshot := hotkeyCaptureSnapshot(nil, true)
	setWebHotkeyCaptureState(snapshot)
	publishHotkeyCaptureChanged(snapshot)
	C.startWebHotkeyCapture()
	return snapshot
}

func cancelWebSettingsHotkeyCapture() {
	C.cancelWebHotkeyCapture()
	snapshot := bridgepkg.HotkeyCaptureSnapshot{}
	setWebHotkeyCaptureState(snapshot)
	publishHotkeyCaptureChanged(snapshot)
}

func confirmWebSettingsHotkeyCapture() (bridgepkg.HotkeyCaptureSnapshot, error) {
	var flags C.uint64_t
	var keycode C.int
	if C.confirmWebHotkeyCapture(&flags, &keycode) == 0 {
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
	publishHotkeyCaptureChanged(snapshot)
	return snapshot, nil
}

//export webSettingsWindowClosed
func webSettingsWindowClosed() {
	setWebHotkeyCaptureState(bridgepkg.HotkeyCaptureSnapshot{})
	clearActiveWebSettingsBridgeService()
	preferencesOpenStore(0)
	cleanupTrackedWebSettingsAssets()
}

func webSettingsResponseScript(response webSettingsResponse, closeWindow bool) string {
	payload, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		fallback := bridgepkg.NewErrorResponse(response.ID, bridgepkg.ErrorCodeBadRequest, marshalErr.Error(), false, nil)
		fallbackPayload, _ := json.Marshal(fallback)
		return "(() => { window.dispatchEvent(new CustomEvent('" + bridgepkg.BridgeEventName + "', { detail: " + string(fallbackPayload) + " })); return false; })();"
	}
	return "(() => { window.dispatchEvent(new CustomEvent('" + bridgepkg.BridgeEventName + "', { detail: " + string(payload) + " })); return " + strings.ToLower(fmt.Sprintf("%t", closeWindow)) + "; })();"
}

func webSettingsEventScript(event bridgepkg.EventEnvelope) string {
	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		currentSettingsLogger().Warn("failed to marshal bridge event", "event", event.Event, "error", marshalErr)
		return ""
	}
	return "(() => { window.dispatchEvent(new CustomEvent('" + bridgepkg.BridgeEventName + "', { detail: " + string(payload) + " })); })();"
}

func dispatchWebSettingsEvent(event bridgepkg.EventEnvelope) {
	script := webSettingsEventScript(event)
	if script == "" {
		return
	}
	webSettingsDispatchScript(script)
}

func publishRuntimeStateChanged(state AppState) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.RuntimeStateChangedEvent, bridgepkg.AppStateSnapshot{
		State:   state.String(),
		Version: versionpkg.Version,
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

func publishConfigSaved(snapshot bridgepkg.ConfigSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.ConfigSavedEvent, snapshot))
}

func publishDevicesChanged(devices []bridgepkg.DeviceSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.DevicesChangedEvent, devices))
}

func publishHotkeyCaptureChanged(snapshot bridgepkg.HotkeyCaptureSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.HotkeyCaptureChangedEvent, snapshot))
}

type webSettingsProcessResult struct {
	response    webSettingsResponse
	closeWindow bool
}

func processWebSettingsMessage(messageJSON string) webSettingsProcessResult {
	var request bridgepkg.RequestEnvelope
	if err := json.Unmarshal([]byte(messageJSON), &request); err != nil {
		webSettingsPostError(err.Error())
		return webSettingsProcessResult{response: bridgepkg.NewErrorResponse("", bridgepkg.ErrorCodeBadRequest, err.Error(), false, nil)}
	}
	router := bridgepkg.NewRouter(activeWebSettingsBridgeService())
	response := router.HandleRequest(context.Background(), request)
	if !response.OK && response.Error != nil {
		webSettingsPostError(response.Error.Message)
	}
	return webSettingsProcessResult{
		response:    response,
		closeWindow: response.OK && request.Method == bridgepkg.SaveConfigMethod,
	}
}

//export handleWebSettingsMessage
func handleWebSettingsMessage(messageJSON *C.char, closeWindow *C.int) *C.char {
	if closeWindow != nil {
		*closeWindow = 0
	}
	if messageJSON == nil {
		response := bridgepkg.NewErrorResponse("", bridgepkg.ErrorCodeBadRequest, "missing web settings message", false, nil)
		return C.CString(webSettingsResponseScript(response, false))
	}

	result := processWebSettingsMessage(C.GoString(messageJSON))
	if closeWindow != nil && result.closeWindow {
		*closeWindow = 1
	}
	return C.CString(webSettingsResponseScript(result.response, result.closeWindow))
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
	currentSettingsLogger().Warn("web settings native transport warning", "operation", op, "message", msg)
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

func trackWebSettingsAssetsRoot(root string) {
	webSettingsAssetsMu.Lock()
	defer webSettingsAssetsMu.Unlock()
	if webSettingsAssetsRoot != "" && webSettingsAssetsRoot != root {
		if err := webSettingsCleanupDir(webSettingsAssetsRoot); err != nil {
			currentSettingsLogger().Warn("failed to clean up web settings assets", "path", webSettingsAssetsRoot, "error", err)
		}
	}
	webSettingsAssetsRoot = root
}

func cleanupTrackedWebSettingsAssets() {
	webSettingsAssetsMu.Lock()
	root := webSettingsAssetsRoot
	webSettingsAssetsRoot = ""
	webSettingsAssetsMu.Unlock()
	if root == "" {
		return
	}
	if err := webSettingsCleanupDir(root); err != nil {
		currentSettingsLogger().Warn("failed to clean up web settings assets", "path", root, "error", err)
	}
}
