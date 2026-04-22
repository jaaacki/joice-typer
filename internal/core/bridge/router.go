package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	configpkg "voicetype/internal/core/config"
)

type Router struct {
	service *Service
}

func NewRouter(service *Service) *Router {
	if service == nil {
		service = NewService(nil)
	}
	return &Router{service: service}
}

type saveConfigParams struct {
	Config *saveConfigPayload `json:"config"`
}

type openPermissionSettingsParams struct {
	Target string `json:"target"`
}

type modelCommandParams struct {
	Size string `json:"size"`
}

type audioInputMonitorParams struct {
	InputDevice string `json:"inputDevice"`
}

type saveConfigPayload struct {
	TriggerKey      *[]string `json:"triggerKey"`
	ModelSize       *string   `json:"modelSize"`
	Language        *string   `json:"language"`
	SampleRate      *int      `json:"sampleRate"`
	SoundFeedback   *bool     `json:"soundFeedback"`
	InputDevice     *string   `json:"inputDevice"`
	InputDeviceName *string   `json:"inputDeviceName"`
	DecodeMode      *string   `json:"decodeMode"`
	PunctuationMode *string   `json:"punctuationMode"`
	Vocabulary      *string   `json:"vocabulary"`
}

func (r *Router) HandleRequest(ctx context.Context, request RequestEnvelope) ResponseEnvelope {
	if request.V != ProtocolVersion || request.Kind != KindRequest || request.ID == "" || request.Method == "" {
		return NewErrorResponse(request.ID, ErrorCodeBadRequest, "invalid bridge request envelope", false, nil)
	}

	switch request.Method {
	case BootstrapMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		bootstrap, err := r.service.Bootstrap(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeInternal, "failed to load bootstrap state", false, nil)
		}
		return NewSuccessResponse(request.ID, bootstrap)
	case ConfigGetMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		config, err := r.service.Config(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeConfigLoadFailure, "failed to load config", false, nil)
		}
		return NewSuccessResponse(request.ID, config)
	case SaveConfigMethod:
		var params saveConfigParams
		if response := decodeRequestParams(request, &params); response != nil {
			return *response
		}
		snapshot, response := params.snapshot(request.ID)
		if response != nil {
			return *response
		}
		if err := r.service.SaveConfig(ctx, snapshot); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeSaveFailure, "failed to save config", false, nil)
		}
		return NewSuccessResponse(request.ID, map[string]any{"saved": true})
	case PermissionsGetMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		permissions, err := r.service.Permissions(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodePermissionsUnavailable, "failed to load permissions", true, nil)
		}
		return NewSuccessResponse(request.ID, permissions)
	case PermissionsOpenSettingsMethod:
		var params openPermissionSettingsParams
		if response := decodeRequestParams(request, &params); response != nil {
			return *response
		}
		if err := r.service.OpenPermissionSettings(ctx, params.Target); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodePermissionOpenFailed, "failed to open system permission settings", true, nil)
		}
		return NewSuccessResponse(request.ID, map[string]any{"opened": true})
	case DevicesListMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		devices, err := r.service.Devices(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeDevicesEnumerationFailed, "failed to list input devices", true, nil)
		}
		return NewSuccessResponse(request.ID, devices)
	case DevicesRefreshMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		devices, err := r.service.RefreshDevices(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeDevicesRefreshFailed, "failed to refresh input devices", true, nil)
		}
		return NewSuccessResponse(request.ID, DevicesRefreshResult{Devices: devices})
	case AudioInputMonitorSetMethod:
		var params audioInputMonitorParams
		if response := decodeRequestParams(request, &params); response != nil {
			return *response
		}
		if err := r.service.SetAudioInputMonitor(ctx, params.InputDevice); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeDevicesRefreshFailed, "failed to update monitored audio input", true, nil)
		}
		return NewSuccessResponse(request.ID, map[string]any{"selected": true})
	case ModelGetMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		model, err := r.service.Model(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeModelUnavailable, "failed to load model state", false, nil)
		}
		return NewSuccessResponse(request.ID, model)
	case ModelDownloadMethod:
		params, response := parseModelCommandParams(request)
		if response != nil {
			return *response
		}
		if err := r.service.DownloadModel(ctx, params.Size); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeModelDownloadFailed, "failed to download model", true, map[string]any{"size": params.Size})
		}
		return NewSuccessResponse(request.ID, ModelCommandResult{Size: params.Size})
	case ModelDeleteMethod:
		params, response := parseModelCommandParams(request)
		if response != nil {
			return *response
		}
		if err := r.service.DeleteModel(ctx, params.Size); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeModelDeleteFailed, "failed to delete model", false, map[string]any{"size": params.Size})
		}
		return NewSuccessResponse(request.ID, ModelCommandResult{Size: params.Size})
	case ModelUseMethod:
		params, response := parseModelCommandParams(request)
		if response != nil {
			return *response
		}
		if err := r.service.UseModel(ctx, params.Size); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeModelUseFailed, "failed to use model", false, map[string]any{"size": params.Size})
		}
		return NewSuccessResponse(request.ID, ModelCommandResult{Size: params.Size})
	case HotkeyCaptureStartMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		snapshot, err := r.service.StartHotkeyCapture(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeHotkeyCaptureStartFailed, "failed to start hotkey capture", true, nil)
		}
		return NewSuccessResponse(request.ID, snapshot)
	case HotkeyCaptureCancelMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		if err := r.service.CancelHotkeyCapture(ctx); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeHotkeyCaptureCancelFailed, "failed to cancel hotkey capture", false, nil)
		}
		return NewSuccessResponse(request.ID, map[string]any{"cancelled": true})
	case HotkeyCaptureConfirmMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		snapshot, err := r.service.ConfirmHotkeyCapture(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeHotkeyCaptureConfirmFailed, "failed to confirm hotkey capture", false, nil)
		}
		return NewSuccessResponse(request.ID, snapshot)
	case RuntimeGetMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		state, err := r.service.AppState(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeRuntimeUnavailable, "failed to load runtime state", true, nil)
		}
		return NewSuccessResponse(request.ID, state)
	case LogsGetMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		tail, err := r.service.LogsGet(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeLogsUnavailable, "failed to load log tail", false, nil)
		}
		return NewSuccessResponse(request.ID, tail)
	case LogsCopyTailMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		text, err := r.service.LogsCopyTail(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeLogsUnavailable, "failed to copy visible log tail", false, nil)
		}
		return NewSuccessResponse(request.ID, text)
	case LogsCopyAllMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		text, err := r.service.LogsCopyAll(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeLogsUnavailable, "failed to load full logs", false, nil)
		}
		return NewSuccessResponse(request.ID, text)
	case OptionsGetMethod:
		if response := ensureEmptyParams(request); response != nil {
			return *response
		}
		return NewSuccessResponse(request.ID, settingsOptionsSnapshot())
	default:
		return NewErrorResponse(request.ID, ErrorCodeBadMethod, fmt.Sprintf("unsupported bridge method %q", request.Method), false, map[string]any{
			"method": request.Method,
		})
	}
}

func parseModelCommandParams(request RequestEnvelope) (modelCommandParams, *ResponseEnvelope) {
	var params modelCommandParams
	if response := decodeRequestParams(request, &params); response != nil {
		return modelCommandParams{}, response
	}
	if params.Size == "" {
		response := NewErrorResponse(request.ID, ErrorCodeBadRequest, "missing model size", false, map[string]any{"field": "size"})
		return modelCommandParams{}, &response
	}
	return params, nil
}

func decodeRequestParams[T any](request RequestEnvelope, target *T) *ResponseEnvelope {
	if err := decodeStrictJSON(request.Params, target); err != nil {
		response := NewErrorResponse(request.ID, ErrorCodeBadRequest, err.Error(), false, nil)
		return &response
	}
	return nil
}

func ensureEmptyParams(request RequestEnvelope) *ResponseEnvelope {
	var params map[string]json.RawMessage
	if response := decodeRequestParams(request, &params); response != nil {
		return response
	}
	if len(params) == 0 {
		return nil
	}
	response := NewErrorResponse(request.ID, ErrorCodeBadRequest, "request does not accept params", false, map[string]any{
		"method": request.Method,
	})
	return &response
}

func decodeStrictJSON(raw json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return fmt.Errorf("missing params object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("unexpected trailing JSON content")
	}
	return nil
}

func (p saveConfigParams) snapshot(requestID string) (ConfigSnapshot, *ResponseEnvelope) {
	if p.Config == nil {
		response := NewErrorResponse(requestID, ErrorCodeBadRequest, "missing config object", false, map[string]any{"field": "config"})
		return ConfigSnapshot{}, &response
	}
	return p.Config.snapshot(requestID)
}

func (p saveConfigPayload) snapshot(requestID string) (ConfigSnapshot, *ResponseEnvelope) {
	type requiredField struct {
		name    string
		present bool
	}
	required := []requiredField{
		{name: "triggerKey", present: p.TriggerKey != nil},
		{name: "modelSize", present: p.ModelSize != nil},
		{name: "language", present: p.Language != nil},
		{name: "sampleRate", present: p.SampleRate != nil},
		{name: "soundFeedback", present: p.SoundFeedback != nil},
		{name: "inputDevice", present: p.InputDevice != nil},
		{name: "inputDeviceName", present: p.InputDeviceName != nil},
		{name: "decodeMode", present: p.DecodeMode != nil},
		{name: "punctuationMode", present: p.PunctuationMode != nil},
		{name: "vocabulary", present: p.Vocabulary != nil},
	}
	for _, field := range required {
		if !field.present {
			response := NewErrorResponse(requestID, ErrorCodeBadRequest, fmt.Sprintf("missing config field %q", field.name), false, map[string]any{
				"field": field.name,
			})
			return ConfigSnapshot{}, &response
		}
	}
	return ConfigSnapshot{
		TriggerKey:      append([]string(nil), (*p.TriggerKey)...),
		ModelSize:       *p.ModelSize,
		Language:        *p.Language,
		SampleRate:      *p.SampleRate,
		SoundFeedback:   *p.SoundFeedback,
		InputDevice:     *p.InputDevice,
		InputDeviceName: *p.InputDeviceName,
		DecodeMode:      *p.DecodeMode,
		PunctuationMode: *p.PunctuationMode,
		Vocabulary:      *p.Vocabulary,
	}, nil
}

func settingsOptionsSnapshot() SettingsOptionsSnapshot {
	models := make([]OptionSnapshot, 0, len(configpkg.ModelOptions))
	for _, option := range configpkg.ModelOptions {
		modelPath, _ := configpkg.DefaultModelPath(option.Size)
		_, statErr := os.Stat(modelPath)
		models = append(models, OptionSnapshot{
			Code:      option.Size,
			Name:      option.Description,
			Bytes:     option.Bytes,
			Installed: statErr == nil,
		})
	}

	languages := make([]OptionSnapshot, 0, len(configpkg.WhisperLanguages))
	for _, option := range configpkg.WhisperLanguages {
		languages = append(languages, OptionSnapshot{
			Code: option.Code,
			Name: option.Name,
		})
	}

	decodeModes := make([]OptionSnapshot, 0, len(configpkg.DecodeModeOptions))
	for _, option := range configpkg.DecodeModeOptions {
		decodeModes = append(decodeModes, OptionSnapshot{
			Code: option.Code,
			Name: option.Name,
		})
	}

	punctuationModes := make([]OptionSnapshot, 0, len(configpkg.PunctuationModeOptions))
	for _, option := range configpkg.PunctuationModeOptions {
		punctuationModes = append(punctuationModes, OptionSnapshot{
			Code: option.Code,
			Name: option.Name,
		})
	}

	return SettingsOptionsSnapshot{
		Models:           models,
		Languages:        languages,
		DecodeModes:      decodeModes,
		PunctuationModes: punctuationModes,
		Permissions:      permissionOptionsSnapshot(),
		Hotkey: HotkeyOptions{
			Modifiers: configpkg.SupportedHotkeyModifiers(),
			Keys:      configpkg.SupportedHotkeyKeys(),
		},
	}
}

func permissionOptionsSnapshot() PermissionOptions {
	if runtime.GOOS == "windows" {
		return PermissionOptions{
			Accessibility:   PermissionRequirement{Required: false, Actionable: false},
			InputMonitoring: PermissionRequirement{Required: false, Actionable: false},
		}
	}
	return PermissionOptions{
		Accessibility:   PermissionRequirement{Required: true, Actionable: true},
		InputMonitoring: PermissionRequirement{Required: true, Actionable: true},
	}
}
