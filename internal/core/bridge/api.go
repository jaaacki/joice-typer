package bridge

import (
	"context"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
	versionpkg "voicetype/internal/core/version"
)

type Dependencies struct {
	LoadConfig             func(context.Context) (configpkg.Config, error)
	SaveConfig             func(context.Context, configpkg.Config) error
	LoadPermissions        func(context.Context) (PermissionsSnapshot, error)
	OpenPermissionSettings func(context.Context, string) error
	ListDevices            func(context.Context) ([]DeviceSnapshot, error)
	RefreshDevices         func(context.Context) ([]DeviceSnapshot, error)
	LoadMachineInfo        func(context.Context) (MachineInfoSnapshot, error)
	SetAudioInputMonitor   func(context.Context, string) error
	StopAudioInputMonitor  func(context.Context) error
	LoadModel              func(context.Context) (ModelSnapshot, error)
	DownloadModel          func(context.Context, string) error
	DeleteModel            func(context.Context, string) error
	UseModel               func(context.Context, string) error
	StartHotkeyCapture     func(context.Context) (HotkeyCaptureSnapshot, error)
	CancelHotkeyCapture    func(context.Context) error
	ConfirmHotkeyCapture   func(context.Context) (HotkeyCaptureSnapshot, error)
	LoadAppState           func(context.Context) (apppkg.AppState, error)
	LoadLogsTail           func(context.Context) (LogTailSnapshot, error)
	LoadLogsFull           func(context.Context) (string, error)
	WriteClipboardText     func(context.Context, string) error
	LoadUpdater            func(context.Context) (UpdaterSnapshot, error)
	CheckForUpdates        func(context.Context) error
	GetLoginItem           func(context.Context) (LoginItemSnapshot, error)
	SetLoginItem           func(context.Context, bool) (LoginItemSnapshot, error)
	GetInputVolume         func(context.Context, string) (InputVolumeSnapshot, error)
	SetInputVolume         func(context.Context, string, float64) (InputVolumeSnapshot, error)
}

type Service struct {
	deps Dependencies
}

func missingDependencyError(operation string, dependency string) error {
	return NewContractError(
		ErrorCodeInternal,
		"bridge service misconfigured: missing dependency",
		false,
		map[string]any{
			"operation":  operation,
			"dependency": dependency,
		},
	)
}

func NewService(deps *Dependencies) *Service {
	if deps == nil {
		return &Service{}
	}
	return &Service{deps: *deps}
}

func (s *Service) Config(ctx context.Context) (ConfigSnapshot, error) {
	if s.deps.LoadConfig == nil {
		return ConfigSnapshot{}, missingDependencyError(ConfigGetMethod, "LoadConfig")
	}
	cfg, err := s.deps.LoadConfig(ctx)
	if err != nil {
		return ConfigSnapshot{}, err
	}
	return configSnapshotFromConfig(cfg), nil
}

func (s *Service) SaveConfig(ctx context.Context, snapshot ConfigSnapshot) error {
	if s.deps.SaveConfig == nil {
		return missingDependencyError(SaveConfigMethod, "SaveConfig")
	}
	return s.deps.SaveConfig(ctx, configFromSnapshot(snapshot))
}

func (s *Service) Permissions(ctx context.Context) (PermissionsSnapshot, error) {
	if s.deps.LoadPermissions == nil {
		return PermissionsSnapshot{}, missingDependencyError(PermissionsGetMethod, "LoadPermissions")
	}
	return s.deps.LoadPermissions(ctx)
}

func (s *Service) OpenPermissionSettings(ctx context.Context, target string) error {
	if s.deps.OpenPermissionSettings == nil {
		return missingDependencyError(PermissionsOpenSettingsMethod, "OpenPermissionSettings")
	}
	return s.deps.OpenPermissionSettings(ctx, target)
}

func (s *Service) Devices(ctx context.Context) ([]DeviceSnapshot, error) {
	if s.deps.ListDevices == nil {
		return nil, missingDependencyError(DevicesListMethod, "ListDevices")
	}
	return s.deps.ListDevices(ctx)
}

func (s *Service) RefreshDevices(ctx context.Context) ([]DeviceSnapshot, error) {
	if s.deps.RefreshDevices == nil {
		return nil, missingDependencyError(DevicesRefreshMethod, "RefreshDevices")
	}
	return s.deps.RefreshDevices(ctx)
}

func (s *Service) MachineInfo(ctx context.Context) (MachineInfoSnapshot, error) {
	if s.deps.LoadMachineInfo == nil {
		return MachineInfoSnapshot{}, nil
	}
	return s.deps.LoadMachineInfo(ctx)
}

func (s *Service) SetAudioInputMonitor(ctx context.Context, inputDevice string) error {
	if s.deps.SetAudioInputMonitor == nil {
		return missingDependencyError(AudioInputMonitorSetMethod, "SetAudioInputMonitor")
	}
	return s.deps.SetAudioInputMonitor(ctx, inputDevice)
}

func (s *Service) StopAudioInputMonitor(ctx context.Context) error {
	if s.deps.StopAudioInputMonitor == nil {
		return missingDependencyError(AudioInputMonitorStopMethod, "StopAudioInputMonitor")
	}
	return s.deps.StopAudioInputMonitor(ctx)
}

func (s *Service) Model(ctx context.Context) (ModelSnapshot, error) {
	if s.deps.LoadModel == nil {
		return ModelSnapshot{}, missingDependencyError(ModelGetMethod, "LoadModel")
	}
	return s.deps.LoadModel(ctx)
}

func (s *Service) DownloadModel(ctx context.Context, size string) error {
	if s.deps.DownloadModel == nil {
		return missingDependencyError(ModelDownloadMethod, "DownloadModel")
	}
	return s.deps.DownloadModel(ctx, size)
}

func (s *Service) DeleteModel(ctx context.Context, size string) error {
	if s.deps.DeleteModel == nil {
		return missingDependencyError(ModelDeleteMethod, "DeleteModel")
	}
	return s.deps.DeleteModel(ctx, size)
}

func (s *Service) UseModel(ctx context.Context, size string) error {
	if s.deps.UseModel == nil {
		return missingDependencyError(ModelUseMethod, "UseModel")
	}
	return s.deps.UseModel(ctx, size)
}

func (s *Service) StartHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error) {
	if s.deps.StartHotkeyCapture == nil {
		return HotkeyCaptureSnapshot{}, missingDependencyError(HotkeyCaptureStartMethod, "StartHotkeyCapture")
	}
	return s.deps.StartHotkeyCapture(ctx)
}

func (s *Service) CancelHotkeyCapture(ctx context.Context) error {
	if s.deps.CancelHotkeyCapture == nil {
		return missingDependencyError(HotkeyCaptureCancelMethod, "CancelHotkeyCapture")
	}
	return s.deps.CancelHotkeyCapture(ctx)
}

func (s *Service) ConfirmHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error) {
	if s.deps.ConfirmHotkeyCapture == nil {
		return HotkeyCaptureSnapshot{}, missingDependencyError(HotkeyCaptureConfirmMethod, "ConfirmHotkeyCapture")
	}
	return s.deps.ConfirmHotkeyCapture(ctx)
}

func (s *Service) AppState(ctx context.Context) (AppStateSnapshot, error) {
	snapshot := AppStateSnapshot{
		State:   "unknown",
		Version: versionpkg.Version,
	}
	if s.deps.LoadAppState == nil {
		return AppStateSnapshot{}, missingDependencyError(RuntimeGetMethod, "LoadAppState")
	}
	state, err := s.deps.LoadAppState(ctx)
	if err != nil {
		return AppStateSnapshot{}, err
	}
	snapshot.State = state.String()
	return snapshot, nil
}

func (s *Service) LogsGet(ctx context.Context) (LogTailSnapshot, error) {
	if s.deps.LoadLogsTail == nil {
		return LogTailSnapshot{}, missingDependencyError(LogsGetMethod, "LoadLogsTail")
	}
	return s.deps.LoadLogsTail(ctx)
}

func (s *Service) LogsCopyAll(ctx context.Context) (string, error) {
	if s.deps.LoadLogsFull == nil {
		return "", missingDependencyError(LogsCopyAllMethod, "LoadLogsFull")
	}
	text, err := s.deps.LoadLogsFull(ctx)
	if err != nil {
		return "", err
	}
	if s.deps.WriteClipboardText != nil {
		if err := s.deps.WriteClipboardText(ctx, text); err != nil {
			return "", err
		}
	}
	return text, nil
}

func (s *Service) LogsCopyTail(ctx context.Context) (string, error) {
	if s.deps.LoadLogsTail == nil {
		return "", missingDependencyError(LogsCopyTailMethod, "LoadLogsTail")
	}
	snapshot, err := s.deps.LoadLogsTail(ctx)
	if err != nil {
		return "", err
	}
	if s.deps.WriteClipboardText != nil {
		if err := s.deps.WriteClipboardText(ctx, snapshot.Text); err != nil {
			return "", err
		}
	}
	return snapshot.Text, nil
}

func (s *Service) Updater(ctx context.Context) (UpdaterSnapshot, error) {
	if s.deps.LoadUpdater == nil {
		return UpdaterSnapshot{}, missingDependencyError(UpdaterGetMethod, "LoadUpdater")
	}
	return s.deps.LoadUpdater(ctx)
}

func (s *Service) CheckForUpdates(ctx context.Context) error {
	if s.deps.CheckForUpdates == nil {
		return missingDependencyError(UpdaterCheckMethod, "CheckForUpdates")
	}
	return s.deps.CheckForUpdates(ctx)
}

func (s *Service) GetLoginItem(ctx context.Context) (LoginItemSnapshot, error) {
	if s.deps.GetLoginItem == nil {
		return LoginItemSnapshot{}, missingDependencyError(LoginItemGetMethod, "GetLoginItem")
	}
	return s.deps.GetLoginItem(ctx)
}

func (s *Service) SetLoginItem(ctx context.Context, enabled bool) (LoginItemSnapshot, error) {
	if s.deps.SetLoginItem == nil {
		return LoginItemSnapshot{}, missingDependencyError(LoginItemSetMethod, "SetLoginItem")
	}
	return s.deps.SetLoginItem(ctx, enabled)
}

func (s *Service) GetInputVolume(ctx context.Context, deviceName string) (InputVolumeSnapshot, error) {
	if s.deps.GetInputVolume == nil {
		return InputVolumeSnapshot{}, missingDependencyError(InputVolumeGetMethod, "GetInputVolume")
	}
	return s.deps.GetInputVolume(ctx, deviceName)
}

func (s *Service) SetInputVolume(ctx context.Context, deviceName string, volume float64) (InputVolumeSnapshot, error) {
	if s.deps.SetInputVolume == nil {
		return InputVolumeSnapshot{}, missingDependencyError(InputVolumeSetMethod, "SetInputVolume")
	}
	return s.deps.SetInputVolume(ctx, deviceName, volume)
}

func (s *Service) Bootstrap(ctx context.Context) (BootstrapPayload, error) {
	configSnapshot, err := s.Config(ctx)
	if err != nil {
		return BootstrapPayload{}, err
	}
	appStateSnapshot, err := s.AppState(ctx)
	if err != nil {
		return BootstrapPayload{}, err
	}
	permissionsSnapshot, err := s.Permissions(ctx)
	if err != nil {
		return BootstrapPayload{}, err
	}
	modelSnapshot, err := s.Model(ctx)
	if err != nil {
		return BootstrapPayload{}, err
	}
	machineInfoSnapshot, err := s.MachineInfo(ctx)
	if err != nil {
		return BootstrapPayload{}, err
	}
	// Do not call GetLoginItem here: it would load ServiceManagement at every
	// preferences-open, which adds perceptible latency. The UI fetches it
	// lazily via fetchLoginItem() after mount.
	return BootstrapPayload{
		Config:      configSnapshot,
		AppState:    appStateSnapshot,
		Permissions: permissionsSnapshot,
		Model:       modelSnapshot,
		MachineInfo: machineInfoSnapshot,
		Options:     settingsOptionsSnapshot(),
	}, nil
}

func configSnapshotFromConfig(cfg configpkg.Config) ConfigSnapshot {
	return ConfigSnapshot{
		TriggerKey:      append([]string(nil), cfg.TriggerKey...),
		ModelSize:       cfg.ModelSize,
		Language:        cfg.Language,
		OutputMode:      cfg.OutputMode,
		SampleRate:      cfg.SampleRate,
		SoundFeedback:   cfg.SoundFeedback,
		InputDevice:     cfg.InputDevice,
		InputDeviceName: cfg.InputDeviceName,
		DecodeMode:      cfg.DecodeMode,
		PunctuationMode: cfg.PunctuationMode,
		Vocabulary:      cfg.Vocabulary,
	}
}

func configFromSnapshot(snapshot ConfigSnapshot) configpkg.Config {
	return configpkg.Config{
		TriggerKey:      append([]string(nil), snapshot.TriggerKey...),
		ModelSize:       snapshot.ModelSize,
		Language:        snapshot.Language,
		OutputMode:      snapshot.OutputMode,
		SampleRate:      snapshot.SampleRate,
		SoundFeedback:   snapshot.SoundFeedback,
		InputDevice:     snapshot.InputDevice,
		InputDeviceName: snapshot.InputDeviceName,
		DecodeMode:      snapshot.DecodeMode,
		PunctuationMode: snapshot.PunctuationMode,
		Vocabulary:      snapshot.Vocabulary,
	}
}
