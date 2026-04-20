//go:build windows

package windows

import (
	"runtime"
	"strings"

	bridgepkg "voicetype/internal/core/bridge"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

func listWindowsCaptureDevices() ([]bridgepkg.DeviceSnapshot, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return nil, err
	}
	defer ole.CoUninitialize()

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator,
		0,
		wca.CLSCTX_INPROC_SERVER,
		wca.IID_IMMDeviceEnumerator,
		&enumerator,
	); err != nil {
		return nil, err
	}
	defer enumerator.Release()

	defaultID := ""
	var defaultDevice *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ECapture, wca.EConsole, &defaultDevice); err == nil && defaultDevice != nil {
		defer defaultDevice.Release()
		_ = defaultDevice.GetId(&defaultID)
	}

	var collection *wca.IMMDeviceCollection
	if err := enumerator.EnumAudioEndpoints(wca.ECapture, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, err
	}
	defer collection.Release()

	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, err
	}

	snapshots := make([]bridgepkg.DeviceSnapshot, 0, count)
	for i := uint32(0); i < count; i++ {
		var device *wca.IMMDevice
		if err := collection.Item(i, &device); err != nil {
			return nil, err
		}
		if device == nil {
			continue
		}

		snapshot, err := windowsCaptureDeviceSnapshot(device, defaultID)
		device.Release()
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}

	return snapshots, nil
}

func windowsCaptureDeviceSnapshot(device *wca.IMMDevice, defaultID string) (bridgepkg.DeviceSnapshot, error) {
	deviceID := ""
	if err := device.GetId(&deviceID); err != nil {
		return bridgepkg.DeviceSnapshot{}, err
	}

	var propertyStore *wca.IPropertyStore
	if err := device.OpenPropertyStore(wca.STGM_READ, &propertyStore); err != nil {
		return bridgepkg.DeviceSnapshot{}, err
	}
	defer propertyStore.Release()

	var friendlyName wca.PROPVARIANT
	if err := propertyStore.GetValue(&wca.PKEY_Device_FriendlyName, &friendlyName); err != nil {
		return bridgepkg.DeviceSnapshot{}, err
	}
	defer ole.VariantClear(&friendlyName.VARIANT)

	name := strings.TrimSpace(friendlyName.String())
	if name == "" {
		name = "Unknown input device"
	}

	return bridgepkg.DeviceSnapshot{
		Name:      name,
		IsDefault: deviceID != "" && deviceID == defaultID,
	}, nil
}
