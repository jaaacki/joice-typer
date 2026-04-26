//go:build windows

package windows

import (
	"testing"

	bridgepkg "voicetype/internal/core/bridge"
	configpkg "voicetype/internal/core/config"
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

func TestMigrateWindowsInputDeviceConfig_PreservesSavedDeviceNameWhenIDChanges(t *testing.T) {
	originalList := webSettingsListInputDevices
	defer func() { webSettingsListInputDevices = originalList }()

	webSettingsListInputDevices = func() ([]bridgepkg.DeviceSnapshot, error) {
		return []bridgepkg.DeviceSnapshot{
			{ID: "default-id", Name: "Microphone Array (AMD Audio Device)", IsDefault: true},
			{ID: "new-headset-id", Name: "Microphone (2- Logi USB Headset H340)"},
		}, nil
	}

	cfg := MigrateWindowsInputDeviceConfig(configpkg.Config{
		InputDevice:     "old-headset-id",
		InputDeviceName: "Microphone (2- Logi USB Headset H340)",
	})
	if cfg.InputDevice != "new-headset-id" {
		t.Fatalf("expected migrated headset ID, got %q", cfg.InputDevice)
	}
	if cfg.InputDeviceName != "Microphone (2- Logi USB Headset H340)" {
		t.Fatalf("expected headset name to be preserved, got %q", cfg.InputDeviceName)
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
