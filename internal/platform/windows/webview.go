//go:build windows

package windows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	versionpkg "voicetype/internal/core/version"
	uiembed "voicetype/ui"
)

var (
	webSettingsDispatchEnvelope = dispatchWindowsWebView2Envelope
	webSettingsShowWindow       = showWindowsWebView2Host
	webSettingsFocusWindow      = focusWindowsWebView2Host

	embeddedWebUIHTMLMu       sync.Mutex
	embeddedWebUIHTMLTemplate []byte
	embeddedWebUIHTMLInitErr  error

	webSettingsServiceMu sync.Mutex
	webSettingsService   *bridgepkg.Service
)

var (
	embeddedWebUIScriptPattern = regexp.MustCompile(`<script[^>]*src="\./assets/([^"]+\.js)"[^>]*></script>`)
	embeddedWebUIStylePattern  = regexp.MustCompile(`<link[^>]*rel="stylesheet"[^>]*href="\./assets/([^"]+\.css)"[^>]*>`)
	embeddedWebUIHeadPattern   = regexp.MustCompile(`(?i)<head[^>]*>`)
)

func ShowWebSettingsWindow() error {
	return ShowWebSettingsWindowWithBridge(context.Background(), nil)
}

func FocusWebSettingsWindow() {
	webSettingsFocusWindow()
}

func ShowWebSettingsWindowWithBridge(ctx context.Context, service *bridgepkg.Service) error {
	if service == nil {
		service = buildSettingsBridgeService(configpkg.Config{})
	}
	setActiveWebSettingsBridgeService(service)

	html, err := renderEmbeddedWebUI(ctx, service)
	if err != nil {
		clearActiveWebSettingsBridgeService()
		return fmt.Errorf("windows.ShowWebSettingsWindowWithBridge: %w", err)
	}

	if err := webSettingsShowWindow(html); err != nil {
		clearActiveWebSettingsBridgeService()
		return fmt.Errorf("windows.ShowWebSettingsWindowWithBridge: %w", err)
	}
	return nil
}

func setActiveWebSettingsBridgeService(service *bridgepkg.Service) {
	webSettingsServiceMu.Lock()
	webSettingsService = service
	webSettingsServiceMu.Unlock()
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

type webSettingsProcessResult struct {
	response    bridgepkg.ResponseEnvelope
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

func webSettingsResponseJSON(response bridgepkg.ResponseEnvelope) string {
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
	webSettingsDispatchEnvelope(payload, false)
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

func publishLogsUpdated(snapshot bridgepkg.LogTailSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.LogsUpdatedEvent, snapshot))
}

func publishHotkeyCaptureChanged(snapshot bridgepkg.HotkeyCaptureSnapshot) {
	dispatchWebSettingsEvent(bridgepkg.NewEvent(bridgepkg.HotkeyCaptureChangedEvent, snapshot))
}

func webSettingsWindowClosed() {
	resetWebSettingsHotkeyCapture()
	cancelPreferencesContext()
	clearActiveWebSettingsBridgeService()
	preferencesOpenStore(0)
}
