package bridge

import (
	"context"

	configpkg "voicetype/internal/core/config"
	apppkg "voicetype/internal/core/runtime"
	versionpkg "voicetype/internal/core/version"
)

type Dependencies struct {
	LoadConfig      func(context.Context) (configpkg.Config, error)
	SaveConfig      func(context.Context, configpkg.Config) error
	LoadPermissions func(context.Context) (PermissionsSnapshot, error)
	ListDevices     func(context.Context) ([]DeviceSnapshot, error)
	LoadModel       func(context.Context) (ModelSnapshot, error)
	LoadAppState    func(context.Context) (apppkg.AppState, error)
}

type Service struct {
	deps Dependencies
}

func NewService(deps *Dependencies) *Service {
	if deps == nil {
		return &Service{}
	}
	return &Service{deps: *deps}
}

func (s *Service) Config(ctx context.Context) (ConfigSnapshot, error) {
	if s.deps.LoadConfig == nil {
		return ConfigSnapshot{}, nil
	}
	cfg, err := s.deps.LoadConfig(ctx)
	if err != nil {
		return ConfigSnapshot{}, err
	}
	return configSnapshotFromConfig(cfg), nil
}

func (s *Service) SaveConfig(ctx context.Context, snapshot ConfigSnapshot) error {
	if s.deps.SaveConfig == nil {
		return nil
	}
	return s.deps.SaveConfig(ctx, configFromSnapshot(snapshot))
}

func (s *Service) Permissions(ctx context.Context) (PermissionsSnapshot, error) {
	if s.deps.LoadPermissions == nil {
		return PermissionsSnapshot{}, nil
	}
	return s.deps.LoadPermissions(ctx)
}

func (s *Service) Devices(ctx context.Context) ([]DeviceSnapshot, error) {
	if s.deps.ListDevices == nil {
		return nil, nil
	}
	return s.deps.ListDevices(ctx)
}

func (s *Service) Model(ctx context.Context) (ModelSnapshot, error) {
	if s.deps.LoadModel == nil {
		return ModelSnapshot{}, nil
	}
	return s.deps.LoadModel(ctx)
}

func (s *Service) AppState(ctx context.Context) (AppStateSnapshot, error) {
	snapshot := AppStateSnapshot{
		State:   "unknown",
		Version: versionpkg.Version,
	}
	if s.deps.LoadAppState == nil {
		return snapshot, nil
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
	return BootstrapPayload{
		Config:   configSnapshot,
		AppState: appStateSnapshot,
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
