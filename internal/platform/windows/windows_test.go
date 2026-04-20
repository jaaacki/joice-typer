//go:build windows

package windows

import (
	"context"
	"testing"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

func TestBridgeService_ConstructsFromWindowsDependencies(t *testing.T) {
	svc := bridgepkg.NewService(&bridgepkg.Dependencies{
		LoadConfig: func(context.Context) (configpkg.Config, error) {
			return configpkg.Config{}, nil
		},
		LoadAppState: func(context.Context) (apppkg.AppState, error) {
			return apppkg.StateReady, nil
		},
		LoadPermissions: func(context.Context) (bridgepkg.PermissionsSnapshot, error) {
			return bridgepkg.PermissionsSnapshot{}, nil
		},
		LoadModel: func(context.Context) (bridgepkg.ModelSnapshot, error) {
			return bridgepkg.ModelSnapshot{}, nil
		},
	})
	if svc == nil {
		t.Fatal("expected bridge service")
	}
}

func TestWindowsPackageBuilds(t *testing.T) {
	t.Parallel()
	_ = NewPaster(nil)
}

func TestIsWindowsCloneableClipboardFormat_OnlyAllowsKnownMemoryFormats(t *testing.T) {
	for _, format := range []uint32{cfText, cfUnicodeText, cfHDrop, cfDib, cfDibV5, cfLocale} {
		if !isWindowsCloneableClipboardFormat(format) {
			t.Fatalf("expected format %d to be cloneable", format)
		}
	}
	for _, format := range []uint32{2, 3, 9, 14, 0xC000} {
		if isWindowsCloneableClipboardFormat(format) {
			t.Fatalf("expected format %d to be rejected as non-cloneable", format)
		}
	}
}
