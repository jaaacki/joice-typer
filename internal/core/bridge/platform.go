package bridge

import (
	"context"
	"errors"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
)

// Platform is the contract every platform adapter (darwin, windows, ...)
// must satisfy in full. The bridge Service consumes a Platform — there are
// no optional or nullable fields. Adding a method here breaks compilation
// on every platform that hasn't implemented it. That is the entire point:
// cross-platform drift becomes a build error, not a runtime "missing
// dependency" surprise.
//
// Genuinely platform-exclusive features should still appear here. The
// unsupported platform's adapter implements the method by returning a
// ContractError (typically with code ErrorCodeInternal and a clear
// message). Do NOT add nullable function fields or "if nil" branches —
// that path is what previous designs got wrong.
type Platform interface {
	// Configuration
	LoadConfig(ctx context.Context) (configpkg.Config, error)
	SaveConfig(ctx context.Context, cfg configpkg.Config) error

	// Permissions
	LoadPermissions(ctx context.Context) (PermissionsSnapshot, error)
	OpenPermissionSettings(ctx context.Context, target string) error

	// Audio devices
	ListDevices(ctx context.Context) ([]DeviceSnapshot, error)
	RefreshDevices(ctx context.Context) ([]DeviceSnapshot, error)
	SetAudioInputMonitor(ctx context.Context, inputDevice string) error
	StopAudioInputMonitor(ctx context.Context) error
	GetInputVolume(ctx context.Context, deviceName string) (InputVolumeSnapshot, error)
	SetInputVolume(ctx context.Context, deviceName string, volume float64) (InputVolumeSnapshot, error)

	// Machine info
	LoadMachineInfo(ctx context.Context) (MachineInfoSnapshot, error)

	// Models
	LoadModel(ctx context.Context) (ModelSnapshot, error)
	DownloadModel(ctx context.Context, size string) error
	DeleteModel(ctx context.Context, size string) error
	UseModel(ctx context.Context, size string) error

	// Hotkey capture
	StartHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error)
	CancelHotkeyCapture(ctx context.Context) error
	ConfirmHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error)

	// Runtime state
	LoadAppState(ctx context.Context) (apppkg.AppState, error)

	// Logs
	LoadLogsTail(ctx context.Context) (LogTailSnapshot, error)
	LoadLogsFull(ctx context.Context) (string, error)
	WriteClipboardText(ctx context.Context, text string) error

	// Updater
	LoadUpdater(ctx context.Context) (UpdaterSnapshot, error)
	CheckForUpdates(ctx context.Context) error

	// Login item (launch at login)
	GetLoginItem(ctx context.Context) (LoginItemSnapshot, error)
	SetLoginItem(ctx context.Context, enabled bool) (LoginItemSnapshot, error)
}

// errPlatformMethodNotStubbed is returned by FuncPlatform when a method is
// invoked but its function field was left nil. Production adapters never
// hit this — only tests should construct partial FuncPlatform values.
var errPlatformMethodNotStubbed = errors.New("bridge: platform method not stubbed in test")

// FuncPlatform is a test-only adapter: it implements Platform via per-method
// function fields, returning errPlatformMethodNotStubbed when a field is
// left nil. Production code MUST NOT use FuncPlatform — it should
// construct a real struct that implements Platform directly so the
// compiler enforces full coverage.
//
// Each test sets only the fields it exercises and leaves the rest nil.
type FuncPlatform struct {
	LoadConfigFn             func(ctx context.Context) (configpkg.Config, error)
	SaveConfigFn             func(ctx context.Context, cfg configpkg.Config) error
	LoadPermissionsFn        func(ctx context.Context) (PermissionsSnapshot, error)
	OpenPermissionSettingsFn func(ctx context.Context, target string) error
	ListDevicesFn            func(ctx context.Context) ([]DeviceSnapshot, error)
	RefreshDevicesFn         func(ctx context.Context) ([]DeviceSnapshot, error)
	LoadMachineInfoFn        func(ctx context.Context) (MachineInfoSnapshot, error)
	SetAudioInputMonitorFn   func(ctx context.Context, inputDevice string) error
	StopAudioInputMonitorFn  func(ctx context.Context) error
	GetInputVolumeFn         func(ctx context.Context, deviceName string) (InputVolumeSnapshot, error)
	SetInputVolumeFn         func(ctx context.Context, deviceName string, volume float64) (InputVolumeSnapshot, error)
	LoadModelFn              func(ctx context.Context) (ModelSnapshot, error)
	DownloadModelFn          func(ctx context.Context, size string) error
	DeleteModelFn            func(ctx context.Context, size string) error
	UseModelFn               func(ctx context.Context, size string) error
	StartHotkeyCaptureFn     func(ctx context.Context) (HotkeyCaptureSnapshot, error)
	CancelHotkeyCaptureFn    func(ctx context.Context) error
	ConfirmHotkeyCaptureFn   func(ctx context.Context) (HotkeyCaptureSnapshot, error)
	LoadAppStateFn           func(ctx context.Context) (apppkg.AppState, error)
	LoadLogsTailFn           func(ctx context.Context) (LogTailSnapshot, error)
	LoadLogsFullFn           func(ctx context.Context) (string, error)
	WriteClipboardTextFn     func(ctx context.Context, text string) error
	LoadUpdaterFn            func(ctx context.Context) (UpdaterSnapshot, error)
	CheckForUpdatesFn        func(ctx context.Context) error
	GetLoginItemFn           func(ctx context.Context) (LoginItemSnapshot, error)
	SetLoginItemFn           func(ctx context.Context, enabled bool) (LoginItemSnapshot, error)
}

// Compile-time assertion that FuncPlatform satisfies Platform. If you add a
// method to Platform, this line is what tells you to add the corresponding
// field + delegator to FuncPlatform.
var _ Platform = FuncPlatform{}

func (p FuncPlatform) LoadConfig(ctx context.Context) (configpkg.Config, error) {
	if p.LoadConfigFn == nil {
		return configpkg.Config{}, errPlatformMethodNotStubbed
	}
	return p.LoadConfigFn(ctx)
}

func (p FuncPlatform) SaveConfig(ctx context.Context, cfg configpkg.Config) error {
	if p.SaveConfigFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.SaveConfigFn(ctx, cfg)
}

func (p FuncPlatform) LoadPermissions(ctx context.Context) (PermissionsSnapshot, error) {
	if p.LoadPermissionsFn == nil {
		return PermissionsSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.LoadPermissionsFn(ctx)
}

func (p FuncPlatform) OpenPermissionSettings(ctx context.Context, target string) error {
	if p.OpenPermissionSettingsFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.OpenPermissionSettingsFn(ctx, target)
}

func (p FuncPlatform) ListDevices(ctx context.Context) ([]DeviceSnapshot, error) {
	if p.ListDevicesFn == nil {
		return nil, errPlatformMethodNotStubbed
	}
	return p.ListDevicesFn(ctx)
}

func (p FuncPlatform) RefreshDevices(ctx context.Context) ([]DeviceSnapshot, error) {
	if p.RefreshDevicesFn == nil {
		return nil, errPlatformMethodNotStubbed
	}
	return p.RefreshDevicesFn(ctx)
}

func (p FuncPlatform) LoadMachineInfo(ctx context.Context) (MachineInfoSnapshot, error) {
	if p.LoadMachineInfoFn == nil {
		return MachineInfoSnapshot{}, nil // tolerated nil-as-empty for back-compat
	}
	return p.LoadMachineInfoFn(ctx)
}

func (p FuncPlatform) SetAudioInputMonitor(ctx context.Context, inputDevice string) error {
	if p.SetAudioInputMonitorFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.SetAudioInputMonitorFn(ctx, inputDevice)
}

func (p FuncPlatform) StopAudioInputMonitor(ctx context.Context) error {
	if p.StopAudioInputMonitorFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.StopAudioInputMonitorFn(ctx)
}

func (p FuncPlatform) GetInputVolume(ctx context.Context, deviceName string) (InputVolumeSnapshot, error) {
	if p.GetInputVolumeFn == nil {
		return InputVolumeSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.GetInputVolumeFn(ctx, deviceName)
}

func (p FuncPlatform) SetInputVolume(ctx context.Context, deviceName string, volume float64) (InputVolumeSnapshot, error) {
	if p.SetInputVolumeFn == nil {
		return InputVolumeSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.SetInputVolumeFn(ctx, deviceName, volume)
}

func (p FuncPlatform) LoadModel(ctx context.Context) (ModelSnapshot, error) {
	if p.LoadModelFn == nil {
		return ModelSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.LoadModelFn(ctx)
}

func (p FuncPlatform) DownloadModel(ctx context.Context, size string) error {
	if p.DownloadModelFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.DownloadModelFn(ctx, size)
}

func (p FuncPlatform) DeleteModel(ctx context.Context, size string) error {
	if p.DeleteModelFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.DeleteModelFn(ctx, size)
}

func (p FuncPlatform) UseModel(ctx context.Context, size string) error {
	if p.UseModelFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.UseModelFn(ctx, size)
}

func (p FuncPlatform) StartHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error) {
	if p.StartHotkeyCaptureFn == nil {
		return HotkeyCaptureSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.StartHotkeyCaptureFn(ctx)
}

func (p FuncPlatform) CancelHotkeyCapture(ctx context.Context) error {
	if p.CancelHotkeyCaptureFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.CancelHotkeyCaptureFn(ctx)
}

func (p FuncPlatform) ConfirmHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error) {
	if p.ConfirmHotkeyCaptureFn == nil {
		return HotkeyCaptureSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.ConfirmHotkeyCaptureFn(ctx)
}

func (p FuncPlatform) LoadAppState(ctx context.Context) (apppkg.AppState, error) {
	if p.LoadAppStateFn == nil {
		return 0, errPlatformMethodNotStubbed
	}
	return p.LoadAppStateFn(ctx)
}

func (p FuncPlatform) LoadLogsTail(ctx context.Context) (LogTailSnapshot, error) {
	if p.LoadLogsTailFn == nil {
		return LogTailSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.LoadLogsTailFn(ctx)
}

func (p FuncPlatform) LoadLogsFull(ctx context.Context) (string, error) {
	if p.LoadLogsFullFn == nil {
		return "", errPlatformMethodNotStubbed
	}
	return p.LoadLogsFullFn(ctx)
}

func (p FuncPlatform) WriteClipboardText(ctx context.Context, text string) error {
	if p.WriteClipboardTextFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.WriteClipboardTextFn(ctx, text)
}

func (p FuncPlatform) LoadUpdater(ctx context.Context) (UpdaterSnapshot, error) {
	if p.LoadUpdaterFn == nil {
		return UpdaterSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.LoadUpdaterFn(ctx)
}

func (p FuncPlatform) CheckForUpdates(ctx context.Context) error {
	if p.CheckForUpdatesFn == nil {
		return errPlatformMethodNotStubbed
	}
	return p.CheckForUpdatesFn(ctx)
}

func (p FuncPlatform) GetLoginItem(ctx context.Context) (LoginItemSnapshot, error) {
	if p.GetLoginItemFn == nil {
		return LoginItemSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.GetLoginItemFn(ctx)
}

func (p FuncPlatform) SetLoginItem(ctx context.Context, enabled bool) (LoginItemSnapshot, error) {
	if p.SetLoginItemFn == nil {
		return LoginItemSnapshot{}, errPlatformMethodNotStubbed
	}
	return p.SetLoginItemFn(ctx, enabled)
}
