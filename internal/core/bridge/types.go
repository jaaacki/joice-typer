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

type OptionSnapshot struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type SettingsOptionsSnapshot struct {
	Models           []OptionSnapshot `json:"models"`
	Languages        []OptionSnapshot `json:"languages"`
	DecodeModes      []OptionSnapshot `json:"decodeModes"`
	PunctuationModes []OptionSnapshot `json:"punctuationModes"`
}

type DevicesRefreshResult struct {
	Devices []DeviceSnapshot `json:"devices"`
}

type ModelCommandResult struct {
	Size string `json:"size"`
}

type HotkeyCaptureSnapshot struct {
	TriggerKey []string `json:"triggerKey"`
	Display    string   `json:"display"`
	Recording  bool     `json:"recording"`
	CanConfirm bool     `json:"canConfirm"`
}
