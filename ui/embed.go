package uiembed

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// EmbeddedAssets contains the built frontend shell assets that will be loaded
// by the desktop runtime.
//
//go:embed dist dist/* dist/assets/*
var EmbeddedAssets embed.FS

// requireBuiltIndexHTML forces a clear, fail-fast compile error if
// `ui/dist/index.html` does not exist when the Go binary is built. Without
// this, the embed pattern above fails with a vague "no matching files found"
// when the frontend has not been built. Build with `make build` / `make app`,
// not `go build` directly.
//
//go:embed dist/index.html
var requireBuiltIndexHTML []byte

// ValidateBuiltAssets returns an error if the embedded frontend assets look
// like a stub or unbuilt placeholder rather than a real Vite production build.
// Call this at startup so a binary built without `make frontend-build` fails
// loudly instead of silently falling back to native preferences.
func ValidateBuiltAssets() error {
	indexBytes, err := fs.ReadFile(EmbeddedAssets, "dist/index.html")
	if err != nil {
		return fmt.Errorf("uiembed.ValidateBuiltAssets: read embedded dist/index.html: %w", err)
	}
	if len(bytes.TrimSpace(indexBytes)) == 0 {
		return fmt.Errorf("uiembed.ValidateBuiltAssets: embedded dist/index.html is empty; run `make build` or `make app` to build the frontend first")
	}
	indexHTML := string(indexBytes)
	if !strings.Contains(indexHTML, `src="./assets/`) || !strings.Contains(indexHTML, `.js"`) {
		return fmt.Errorf("uiembed.ValidateBuiltAssets: embedded dist/index.html does not reference a built ./assets/*.js bundle; run `make build` or `make app` to produce a real Vite build instead of a stub")
	}
	entries, err := fs.ReadDir(EmbeddedAssets, "dist/assets")
	if err != nil {
		return fmt.Errorf("uiembed.ValidateBuiltAssets: read embedded dist/assets: %w", err)
	}
	hasJS := false
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") {
			hasJS = true
			break
		}
	}
	if !hasJS {
		return fmt.Errorf("uiembed.ValidateBuiltAssets: embedded dist/assets contains no JavaScript bundle; run `make build` or `make app` to produce a real Vite build")
	}
	return nil
}
