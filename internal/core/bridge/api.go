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
	LoadModel              func(context.Context) (ModelSnapshot, error)
	DownloadModel          func(context.Context, string) error
	DeleteModel            func(context.Context, string) error
	UseModel               func(context.Context, string) error
	StartHotkeyCapture     func(context.Context) (HotkeyCaptureSnapshot, error)
	CancelHotkeyCapture    func(context.Context) error
	ConfirmHotkeyCapture   func(context.Context) (HotkeyCaptureSnapshot, error)
	LoadAppState           func(context.Context) (apppkg.AppState, error)
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
	return BootstrapPayload{
		Config:      configSnapshot,
		AppState:    appStateSnapshot,
		Permissions: permissionsSnapshot,
		Model:       modelSnapshot,
		Options:     settingsOptionsSnapshot(),
	}, nil
}

func configSnapshotFromConfig(cfg configpkg.Config) ConfigSnapshot {
	return ConfigSnapshot{
		TriggerKey:      append([]string(nil), cfg.TriggerKey...),
		ModelSize:       cfg.ModelSize,
		Language:        cfg.Language,
		SampleRate:      cfg.SampleRate,
		SoundFeedback:   cfg.SoundFeedback,
		InputDevice:     cfg.InputDevice,
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
		SampleRate:      snapshot.SampleRate,
		SoundFeedback:   snapshot.SoundFeedback,
		InputDevice:     snapshot.InputDevice,
		DecodeMode:      snapshot.DecodeMode,
		PunctuationMode: snapshot.PunctuationMode,
		Vocabulary:      snapshot.Vocabulary,
	}
}
