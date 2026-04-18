//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include "webview_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	uiembed "voicetype/ui"
)

var (
	webSettingsEnabled = func() bool {
		return os.Getenv("JOICETYPER_USE_WEB_SETTINGS") == "1"
	}
	embeddedWebUIRoot   string
	embeddedWebUIRootMu sync.Mutex
)

func shouldUseWebSettings() bool {
	return webSettingsEnabled()
}

func ShowWebSettingsWindow() error {
	indexPath, err := ensureEmbeddedWebUI()
	if err != nil {
		return fmt.Errorf("darwin.ShowWebSettingsWindow: %w", err)
	}

	cIndexPath := C.CString(indexPath)
	defer C.free(unsafe.Pointer(cIndexPath))

	C.showWebSettingsWindow(cIndexPath)
	return nil
}

func ensureEmbeddedWebUI() (string, error) {
	embeddedWebUIRootMu.Lock()
	defer embeddedWebUIRootMu.Unlock()

	if embeddedWebUIRoot != "" {
		return filepath.Join(embeddedWebUIRoot, "index.html"), nil
	}

	root, err := os.MkdirTemp("", "joicetyper-web-ui-*")
	if err != nil {
		return "", fmt.Errorf("create temp UI dir: %w", err)
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
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	}); err != nil {
		return "", fmt.Errorf("materialize embedded UI: %w", err)
	}

	embeddedWebUIRoot = root
	return filepath.Join(root, "index.html"), nil
}
