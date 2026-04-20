//go:build darwin

package audio

import (
	"fmt"

	bridgepkg "voicetype/internal/core/bridge"

	"github.com/gordonklaus/portaudio"
)

// ListInputDevices prints available input devices to stdout.
// This is intentional CLI output for --list-devices, not application logging.
func ListInputDevices() error {
	devices, err := ListInputDeviceSnapshots()
	if err != nil {
		return fmt.Errorf("recorder.ListInputDevices: %w", err)
	}
	fmt.Println("Available input devices:")
	for _, d := range devices {
		defaultSuffix := ""
		if d.IsDefault {
			defaultSuffix = " (default)"
		}
		fmt.Printf("  %s%s\n", d.Name, defaultSuffix)
	}
	fmt.Printf("\nSet input_device in %s to use a specific device.\n", listDevicesConfigHint())
	return nil
}

func ListInputDeviceSnapshots() ([]bridgepkg.DeviceSnapshot, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, err
	}
	defaultInput, defaultErr := portaudio.DefaultInputDevice()
	snapshots := make([]bridgepkg.DeviceSnapshot, 0, len(devices))
	for _, device := range devices {
		if device.MaxInputChannels <= 0 {
			continue
		}
		isDefault := defaultErr == nil && defaultInput != nil && defaultInput.Name == device.Name
		snapshots = append(snapshots, bridgepkg.DeviceSnapshot{
			Name:      device.Name,
			IsDefault: isDefault,
		})
	}
	return snapshots, nil
}

