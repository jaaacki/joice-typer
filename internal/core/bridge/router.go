package bridge

import (
	"context"
	"encoding/json"
	"fmt"

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
	Config ConfigSnapshot `json:"config"`
}

type openPermissionSettingsParams struct {
	Target string `json:"target"`
}

type modelCommandParams struct {
	Size string `json:"size"`
}

func (r *Router) HandleRequest(ctx context.Context, request RequestEnvelope) ResponseEnvelope {
	if request.V != ProtocolVersion || request.Kind != KindRequest || request.ID == "" || request.Method == "" {
		return NewErrorResponse(request.ID, ErrorCodeBadRequest, "invalid bridge request envelope", false, nil)
	}

	switch request.Method {
	case BootstrapMethod:
		bootstrap, err := r.service.Bootstrap(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeInternal, "failed to load bootstrap state", false, nil)
		}
		return NewSuccessResponse(request.ID, bootstrap)
	case ConfigGetMethod:
		config, err := r.service.Config(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeConfigLoadFailure, "failed to load config", false, nil)
		}
		return NewSuccessResponse(request.ID, config)
	case SaveConfigMethod:
		var params saveConfigParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return NewErrorResponse(request.ID, ErrorCodeBadRequest, err.Error(), false, nil)
		}
		if err := r.service.SaveConfig(ctx, params.Config); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeSaveFailure, "failed to save config", false, nil)
		}
		return NewSuccessResponse(request.ID, map[string]any{"saved": true})
	case PermissionsGetMethod:
		permissions, err := r.service.Permissions(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodePermissionsUnavailable, "failed to load permissions", true, nil)
		}
		return NewSuccessResponse(request.ID, permissions)
	case PermissionsOpenSettingsMethod:
		var params openPermissionSettingsParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return NewErrorResponse(request.ID, ErrorCodeBadRequest, err.Error(), false, nil)
		}
		if err := r.service.OpenPermissionSettings(ctx, params.Target); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodePermissionOpenFailed, "failed to open system permission settings", true, nil)
		}
		return NewSuccessResponse(request.ID, map[string]any{"opened": true})
	case DevicesListMethod:
		devices, err := r.service.Devices(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeDevicesEnumerationFailed, "failed to list input devices", true, nil)
		}
		return NewSuccessResponse(request.ID, devices)
	case DevicesRefreshMethod:
		devices, err := r.service.RefreshDevices(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeDevicesRefreshFailed, "failed to refresh input devices", true, nil)
		}
		return NewSuccessResponse(request.ID, DevicesRefreshResult{Devices: devices})
	case ModelGetMethod:
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
		snapshot, err := r.service.StartHotkeyCapture(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeHotkeyCaptureStartFailed, "failed to start hotkey capture", true, nil)
		}
		return NewSuccessResponse(request.ID, snapshot)
	case HotkeyCaptureCancelMethod:
		if err := r.service.CancelHotkeyCapture(ctx); err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeHotkeyCaptureCancelFailed, "failed to cancel hotkey capture", false, nil)
		}
		return NewSuccessResponse(request.ID, map[string]any{"cancelled": true})
	case HotkeyCaptureConfirmMethod:
		snapshot, err := r.service.ConfirmHotkeyCapture(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeHotkeyCaptureConfirmFailed, "failed to confirm hotkey capture", false, nil)
		}
		return NewSuccessResponse(request.ID, snapshot)
	case RuntimeGetMethod:
		state, err := r.service.AppState(ctx)
		if err != nil {
			return NewErrorResponseFromError(request.ID, err, ErrorCodeRuntimeUnavailable, "failed to load runtime state", true, nil)
		}
		return NewSuccessResponse(request.ID, state)
	case OptionsGetMethod:
		return NewSuccessResponse(request.ID, settingsOptionsSnapshot())
	default:
		return NewErrorResponse(request.ID, ErrorCodeBadMethod, fmt.Sprintf("unsupported bridge method %q", request.Method), false, map[string]any{
			"method": request.Method,
		})
	}
}

func parseModelCommandParams(request RequestEnvelope) (modelCommandParams, *ResponseEnvelope) {
	var params modelCommandParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		response := NewErrorResponse(request.ID, ErrorCodeBadRequest, err.Error(), false, nil)
		return modelCommandParams{}, &response
	}
	if params.Size == "" {
		response := NewErrorResponse(request.ID, ErrorCodeBadRequest, "missing model size", false, map[string]any{"field": "size"})
		return modelCommandParams{}, &response
	}
	return params, nil
}

func settingsOptionsSnapshot() SettingsOptionsSnapshot {
	models := make([]OptionSnapshot, 0, len(configpkg.ModelOptions))
	for _, option := range configpkg.ModelOptions {
		models = append(models, OptionSnapshot{
			Code: option.Size,
			Name: option.Description,
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
	}
}
