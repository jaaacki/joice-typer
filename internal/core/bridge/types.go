package bridge

type ConfigSnapshot struct {
	TriggerKey      []string `json:"triggerKey"`
	ModelSize       string   `json:"modelSize"`
	Language        string   `json:"language"`
	OutputMode      string   `json:"outputMode"`
	SampleRate      int      `json:"sampleRate"`
	SoundFeedback   bool     `json:"soundFeedback"`
	InputDevice     string   `json:"inputDevice"`
	InputDeviceName string   `json:"inputDeviceName"`
	DecodeMode      string   `json:"decodeMode"`
	PunctuationMode string   `json:"punctuationMode"`
	Vocabulary      string   `json:"vocabulary"`
}

type InputLevelSnapshot struct {
	Level   float64 `json:"level"`
	Quality string  `json:"quality"`
}

type PermissionsSnapshot struct {
	Accessibility   bool `json:"accessibility"`
	InputMonitoring bool `json:"inputMonitoring"`
}

type DeviceSnapshot struct {
	ID        string `json:"id"`
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

type MachineBackendSnapshot struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type MachineInfoSnapshot struct {
	Platform              string                   `json:"platform"`
	MachineModel          string                   `json:"machineModel"`
	Chip                  string                   `json:"chip"`
	CPUModel              string                   `json:"cpuModel"`
	IntegratedGPU         string                   `json:"integratedGpu"`
	Graphics              []string                 `json:"graphics"`
	WhisperSystemInfo     string                   `json:"whisperSystemInfo"`
	InferenceBackends     []MachineBackendSnapshot `json:"inferenceBackends"`
	WebViewRuntimeVersion string                   `json:"webViewRuntimeVersion"`
}

type UpdaterSnapshot struct {
	Enabled             bool   `json:"enabled"`
	SupportsManualCheck bool   `json:"supportsManualCheck"`
	FeedURL             string `json:"feedURL"`
	Channel             string `json:"channel"`
}

type LoginItemSnapshot struct {
	Enabled bool `json:"enabled"`
}

type InputVolumeSnapshot struct {
	Volume    float64 `json:"volume"`
	Supported bool    `json:"supported"`
}

type MicrophoneModeSnapshot struct {
	Available bool `json:"available"`
	Preferred int  `json:"preferred"`
	Active    int  `json:"active"`
}

type BootstrapPayload struct {
	Config      ConfigSnapshot          `json:"config"`
	AppState    AppStateSnapshot        `json:"appState"`
	Permissions PermissionsSnapshot     `json:"permissions"`
	Model       ModelSnapshot           `json:"model"`
	MachineInfo MachineInfoSnapshot     `json:"machineInfo"`
	Options     SettingsOptionsSnapshot `json:"options"`
	LoginItem   LoginItemSnapshot       `json:"loginItem"`
}

type OptionSnapshot struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Bytes       int64  `json:"bytes,omitempty"`
	Installed   bool   `json:"installed"`
	EnglishOnly bool   `json:"englishOnly,omitempty"`
}

type SettingsOptionsSnapshot struct {
	Models           []OptionSnapshot  `json:"models"`
	Languages        []OptionSnapshot  `json:"languages"`
	DecodeModes      []OptionSnapshot  `json:"decodeModes"`
	PunctuationModes []OptionSnapshot  `json:"punctuationModes"`
	Permissions      PermissionOptions `json:"permissions"`
	Hotkey           HotkeyOptions     `json:"hotkey"`
}

type PermissionOptions struct {
	Accessibility   PermissionRequirement `json:"accessibility"`
	InputMonitoring PermissionRequirement `json:"inputMonitoring"`
}

type PermissionRequirement struct {
	Required   bool `json:"required"`
	Actionable bool `json:"actionable"`
}

type HotkeyOptions struct {
	Modifiers []string `json:"modifiers"`
	Keys      []string `json:"keys"`
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

type LogTailSnapshot struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	ByteSize  int64  `json:"byteSize"`
	UpdatedAt string `json:"updatedAt"`
}
