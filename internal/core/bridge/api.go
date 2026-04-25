package bridge

import (
	"context"

	configpkg "voicetype/internal/core/config"
	versionpkg "voicetype/internal/core/version"
)

// Service exposes the bridge methods consumed by the router and the
// embedded preferences UI. It delegates every call to a Platform —
// the shape of the platform contract is enforced at compile time, so
// adding a new method to Platform breaks every adapter that hasn't
// implemented it.
type Service struct {
	p Platform
}

// NewService wires a Platform into a bridge Service. The caller MUST
// pass a real Platform — there is no nil shortcut. Tests that need a
// blank stub should construct an empty funcPlatform{}.
func NewService(p Platform) *Service {
	return &Service{p: p}
}

func (s *Service) Config(ctx context.Context) (ConfigSnapshot, error) {
	cfg, err := s.p.LoadConfig(ctx)
	if err != nil {
		return ConfigSnapshot{}, err
	}
	return configSnapshotFromConfig(cfg), nil
}

func (s *Service) SaveConfig(ctx context.Context, snapshot ConfigSnapshot) error {
	return s.p.SaveConfig(ctx, configFromSnapshot(snapshot))
}

func (s *Service) Permissions(ctx context.Context) (PermissionsSnapshot, error) {
	return s.p.LoadPermissions(ctx)
}

func (s *Service) OpenPermissionSettings(ctx context.Context, target string) error {
	return s.p.OpenPermissionSettings(ctx, target)
}

func (s *Service) Devices(ctx context.Context) ([]DeviceSnapshot, error) {
	return s.p.ListDevices(ctx)
}

func (s *Service) RefreshDevices(ctx context.Context) ([]DeviceSnapshot, error) {
	return s.p.RefreshDevices(ctx)
}

func (s *Service) MachineInfo(ctx context.Context) (MachineInfoSnapshot, error) {
	return s.p.LoadMachineInfo(ctx)
}

func (s *Service) SetAudioInputMonitor(ctx context.Context, inputDevice string) error {
	return s.p.SetAudioInputMonitor(ctx, inputDevice)
}

func (s *Service) StopAudioInputMonitor(ctx context.Context) error {
	return s.p.StopAudioInputMonitor(ctx)
}

func (s *Service) Model(ctx context.Context) (ModelSnapshot, error) {
	return s.p.LoadModel(ctx)
}

func (s *Service) DownloadModel(ctx context.Context, size string) error {
	return s.p.DownloadModel(ctx, size)
}

func (s *Service) DeleteModel(ctx context.Context, size string) error {
	return s.p.DeleteModel(ctx, size)
}

func (s *Service) UseModel(ctx context.Context, size string) error {
	return s.p.UseModel(ctx, size)
}

func (s *Service) StartHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error) {
	return s.p.StartHotkeyCapture(ctx)
}

func (s *Service) CancelHotkeyCapture(ctx context.Context) error {
	return s.p.CancelHotkeyCapture(ctx)
}

func (s *Service) ConfirmHotkeyCapture(ctx context.Context) (HotkeyCaptureSnapshot, error) {
	return s.p.ConfirmHotkeyCapture(ctx)
}

func (s *Service) AppState(ctx context.Context) (AppStateSnapshot, error) {
	state, err := s.p.LoadAppState(ctx)
	if err != nil {
		return AppStateSnapshot{}, err
	}
	return AppStateSnapshot{
		State:   state.String(),
		Version: versionpkg.Version,
	}, nil
}

func (s *Service) LogsGet(ctx context.Context) (LogTailSnapshot, error) {
	return s.p.LoadLogsTail(ctx)
}

func (s *Service) LogsCopyAll(ctx context.Context) (string, error) {
	text, err := s.p.LoadLogsFull(ctx)
	if err != nil {
		return "", err
	}
	if err := s.p.WriteClipboardText(ctx, text); err != nil {
		return "", err
	}
	return text, nil
}

func (s *Service) LogsCopyTail(ctx context.Context) (string, error) {
	snapshot, err := s.p.LoadLogsTail(ctx)
	if err != nil {
		return "", err
	}
	if err := s.p.WriteClipboardText(ctx, snapshot.Text); err != nil {
		return "", err
	}
	return snapshot.Text, nil
}

func (s *Service) Updater(ctx context.Context) (UpdaterSnapshot, error) {
	return s.p.LoadUpdater(ctx)
}

func (s *Service) CheckForUpdates(ctx context.Context) error {
	return s.p.CheckForUpdates(ctx)
}

func (s *Service) GetLoginItem(ctx context.Context) (LoginItemSnapshot, error) {
	return s.p.GetLoginItem(ctx)
}

func (s *Service) SetLoginItem(ctx context.Context, enabled bool) (LoginItemSnapshot, error) {
	return s.p.SetLoginItem(ctx, enabled)
}

func (s *Service) GetInputVolume(ctx context.Context, deviceName string) (InputVolumeSnapshot, error) {
	return s.p.GetInputVolume(ctx, deviceName)
}

func (s *Service) SetInputVolume(ctx context.Context, deviceName string, volume float64) (InputVolumeSnapshot, error) {
	return s.p.SetInputVolume(ctx, deviceName, volume)
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
