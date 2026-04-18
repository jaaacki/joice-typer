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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	uiembed "voicetype/ui"
)

var (
	webSettingsEnabled = func() bool {
		return os.Getenv("JOICETYPER_USE_WEB_SETTINGS") == "1"
	}
	webSettingsDefaultConfigPath = configpkg.DefaultConfigPath
	webSettingsSaveConfig        = configpkg.SaveConfig
	webSettingsSignalRestart     = signalHotkeyRestart
	webSettingsPostError         = reportSettingsSaveError
)

func shouldUseWebSettings() bool {
	return webSettingsEnabled()
}

func ShowWebSettingsWindow() error {
	return ShowWebSettingsWindowWithBridge(context.Background(), nil)
}

func ShowWebSettingsWindowWithBridge(ctx context.Context, service *bridgepkg.Service) error {
	indexPath, err := materializeEmbeddedWebUI(ctx, service)
	if err != nil {
		return fmt.Errorf("darwin.ShowWebSettingsWindowWithBridge: %w", err)
	}

	cIndexPath := C.CString(indexPath)
	defer C.free(unsafe.Pointer(cIndexPath))

	C.showWebSettingsWindow(cIndexPath)
	return nil
}

func buildSettingsBridgeService(cfg configpkg.Config) *bridgepkg.Service {
	return bridgepkg.NewService(&bridgepkg.Dependencies{
		LoadConfig: func(context.Context) (configpkg.Config, error) {
			return cfg, nil
		},
		SaveConfig: func(_ context.Context, updated configpkg.Config) error {
			cfgPath, err := webSettingsDefaultConfigPath()
			if err != nil {
				return err
			}
			return webSettingsSaveConfig(cfgPath, updated)
		},
		LoadAppState: func(context.Context) (AppState, error) {
			return currentAppState(), nil
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

	if err := fs.WalkDir(uiembed.EmbeddedAssets, "dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel("dist", path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(root, relPath)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := uiembed.EmbeddedAssets.ReadFile(path)
		if err != nil {
			return err
		}
		if relPath == "index.html" {
			data, err = injectBootstrapScript(data, bootstrap)
			if err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	}); err != nil {
		return "", fmt.Errorf("materialize embedded UI: %w", err)
	}

	return filepath.Join(root, "index.html"), nil
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
	payload, err := json.Marshal(bootstrap)
	if err != nil {
		return nil, fmt.Errorf("marshal bootstrap payload: %w", err)
	}

	script := `<script>window.__JOICETYPER_BOOTSTRAP__ = ` + string(payload) + `;</script>`
	html := string(indexHTML)
	if strings.Contains(html, "</head>") {
		return []byte(strings.Replace(html, "</head>", script+"\n</head>", 1)), nil
	}
	return append(indexHTML, []byte(script)...), nil
}

type webSettingsMessage struct {
	RequestID string                   `json:"requestId"`
	Type      string                   `json:"type"`
	Config    bridgepkg.ConfigSnapshot `json:"config"`
}

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
	webSettingsSignalRestart()
	return nil
}

//export webSettingsWindowClosed
func webSettingsWindowClosed() {
	preferencesOpenStore(0)
}

func webSettingsResponseScript(requestID string, err error) string {
	ok := err == nil
	errorText := ""
	if err != nil {
		errorText = err.Error()
	}

	response := struct {
		RequestID string `json:"requestId"`
		OK        bool   `json:"ok"`
		Error     string `json:"error,omitempty"`
	}{
		RequestID: requestID,
		OK:        ok,
		Error:     errorText,
	}

	payload, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		fallback := fmt.Sprintf(`{"requestId":%q,"ok":false,"error":%q}`, requestID, marshalErr.Error())
		return "window.dispatchEvent(new CustomEvent('joicetyper-native-save', { detail: " + fallback + " }));"
	}
	return "window.dispatchEvent(new CustomEvent('joicetyper-native-save', { detail: " + string(payload) + " }));"
}

//export handleWebSettingsMessage
func handleWebSettingsMessage(messageJSON *C.char) *C.char {
	if messageJSON == nil {
		return C.CString("missing web settings message")
	}

	var message webSettingsMessage
	if err := json.Unmarshal([]byte(C.GoString(messageJSON)), &message); err != nil {
		webSettingsPostError(err.Error())
		return C.CString(err.Error())
	}

	switch message.Type {
	case "saveConfig":
		if err := applyWebSettingsConfig(message.Config); err != nil {
			webSettingsPostError(err.Error())
			return C.CString(err.Error())
		}
		return nil
	default:
		err := fmt.Errorf("unsupported web settings message %q", message.Type)
		webSettingsPostError(err.Error())
		return C.CString(err.Error())
	}
}
