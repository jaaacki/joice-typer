package bridge

type ConfigSnapshot struct {
	TriggerKey      []string
	ModelSize       string
	Language        string
	SampleRate      int
	SoundFeedback   bool
	InputDevice     string
	DecodeMode      string
	PunctuationMode string
	Vocabulary      string
}

type PermissionsSnapshot struct {
	Accessibility   bool
	InputMonitoring bool
}

type DeviceSnapshot struct {
	Name      string
	IsDefault bool
}

type ModelSnapshot struct {
	Size  string
	Path  string
	Ready bool
}

type AppStateSnapshot struct {
	State   string
	Version string
}
