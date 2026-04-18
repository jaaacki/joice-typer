package uiembed

import "embed"

// EmbeddedAssets contains the built frontend shell assets that will be loaded
// by the desktop runtime.
//
//go:embed dist dist/* dist/assets/*
var EmbeddedAssets embed.FS
