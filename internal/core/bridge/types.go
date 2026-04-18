package bridge

type ConfigSnapshot struct {
	TriggerKey      []string `json:"triggerKey"`
	ModelSize       string   `json:"modelSize"`
	Language        string   `json:"language"`
	SampleRate      int      `json:"sampleRate"`
	SoundFeedback   bool     `json:"soundFeedback"`
	InputDevice     string   `json:"inputDevice"`
	DecodeMode      string   `json:"decodeMode"`
	PunctuationMode string   `json:"punctuationMode"`
	Vocabulary      string   `json:"vocabulary"`
}

type PermissionsSnapshot struct {
	Accessibility   bool `json:"accessibility"`
	InputMonitoring bool `json:"inputMonitoring"`
}

type DeviceSnapshot struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type ModelSnapshot struct {
	Size  string `json:"size"`
	Path  string `json:"path"`
	Ready bool   `json:"ready"`
}

type AppStateSnapshot struct {
	State   string `json:"state"`
	Version string `json:"version"`
}

type BootstrapPayload struct {
	Config   ConfigSnapshot   `json:"config"`
	AppState AppStateSnapshot `json:"appState"`
}
