//go:build windows

package windows

import (
	"testing"

	bridgepkg "voicetype/internal/core/bridge"
)

// TestWindowsPlatformSatisfiesBridgeContract is a redundant check on top of
// the var _ bridgepkg.Platform = windowsPlatform{} assertion in settings.go,
// kept here so the Windows test binary itself fails fast (and visibly) if a
// future change to bridgepkg.Platform leaves windowsPlatform out of sync.
func TestWindowsPlatformSatisfiesBridgeContract(t *testing.T) {
	var _ bridgepkg.Platform = windowsPlatform{}
	svc := bridgepkg.NewService(windowsPlatform{})
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
