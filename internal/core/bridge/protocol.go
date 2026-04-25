package bridge

import (
	"encoding/json"

	generated "voicetype/internal/core/bridge/generated"
)

const (
	ProtocolVersion                     = generated.ProtocolVersion
	KindRequest                         = generated.KindRequest
	KindResponse                        = generated.KindResponse
	KindEvent                           = generated.KindEvent
	BridgeEventName                     = generated.BridgeEventName
	BootstrapMethod                     = generated.BootstrapMethod
	ConfigGetMethod                     = generated.ConfigGetMethod
	SaveConfigMethod                    = generated.SaveConfigMethod
	PermissionsGetMethod                = generated.PermissionsGetMethod
	PermissionsOpenSettingsMethod       = generated.PermissionsOpenSettingsMethod
	DevicesListMethod                   = generated.DevicesListMethod
	DevicesRefreshMethod                = generated.DevicesRefreshMethod
	ModelGetMethod                      = generated.ModelGetMethod
	ModelDownloadMethod                 = generated.ModelDownloadMethod
	ModelDeleteMethod                   = generated.ModelDeleteMethod
	ModelUseMethod                      = generated.ModelUseMethod
	HotkeyCaptureStartMethod            = generated.HotkeyCaptureStartMethod
	HotkeyCaptureCancelMethod           = generated.HotkeyCaptureCancelMethod
	HotkeyCaptureConfirmMethod          = generated.HotkeyCaptureConfirmMethod
	AudioInputMonitorSetMethod          = generated.AudioInputMonitorSetMethod
	AudioInputMonitorStopMethod         = generated.AudioInputMonitorStopMethod
	RuntimeGetMethod                    = generated.RuntimeGetMethod
	OptionsGetMethod                    = generated.OptionsGetMethod
	LogsGetMethod                       = generated.LogsGetMethod
	LogsCopyTailMethod                  = generated.LogsCopyTailMethod
	LogsCopyAllMethod                   = generated.LogsCopyAllMethod
	UpdaterGetMethod                    = generated.UpdaterGetMethod
	UpdaterCheckMethod                  = generated.UpdaterCheckMethod
	RuntimeStateChangedEvent            = generated.RuntimeStateChangedEvent
	PermissionsChangedEvent             = generated.PermissionsChangedEvent
	ModelChangedEvent                   = generated.ModelChangedEvent
	ModelDownloadProgressEvent          = generated.ModelDownloadProgressEvent
	ModelDownloadCompletedEvent         = generated.ModelDownloadCompletedEvent
	ModelDownloadFailedEvent            = generated.ModelDownloadFailedEvent
	ConfigSavedEvent                    = generated.ConfigSavedEvent
	LogsUpdatedEvent                    = generated.LogsUpdatedEvent
	DevicesChangedEvent                 = generated.DevicesChangedEvent
	HotkeyCaptureChangedEvent           = generated.HotkeyCaptureChangedEvent
	InputLevelChangedEvent              = generated.InputLevelChangedEvent
	ErrorCodeBadRequest                 = generated.ErrorCodeBadRequest
	ErrorCodeBadMethod                  = generated.ErrorCodeBadMethod
	ErrorCodeInternal                   = generated.ErrorCodeInternal
	ErrorCodeConfigInvalid              = generated.ErrorCodeConfigInvalid
	ErrorCodeConfigLoadFailure          = generated.ErrorCodeConfigLoadFailure
	ErrorCodeSaveFailure                = generated.ErrorCodeSaveFailure
	ErrorCodePermissionsUnavailable     = generated.ErrorCodePermissionsUnavailable
	ErrorCodePermissionInvalidTarget    = generated.ErrorCodePermissionInvalidTarget
	ErrorCodePermissionOpenFailed       = generated.ErrorCodePermissionOpenFailed
	ErrorCodeDevicesEnumerationFailed   = generated.ErrorCodeDevicesEnumerationFailed
	ErrorCodeDevicesRefreshFailed       = generated.ErrorCodeDevicesRefreshFailed
	ErrorCodeModelUnavailable           = generated.ErrorCodeModelUnavailable
	ErrorCodeModelDownloadFailed        = generated.ErrorCodeModelDownloadFailed
	ErrorCodeModelDeleteFailed          = generated.ErrorCodeModelDeleteFailed
	ErrorCodeModelUseFailed             = generated.ErrorCodeModelUseFailed
	ErrorCodeHotkeyCaptureStartFailed   = generated.ErrorCodeHotkeyCaptureStartFailed
	ErrorCodeHotkeyCaptureCancelFailed  = generated.ErrorCodeHotkeyCaptureCancelFailed
	ErrorCodeHotkeyCaptureConfirmFailed = generated.ErrorCodeHotkeyCaptureConfirmFailed
	ErrorCodeLogsUnavailable            = generated.ErrorCodeLogsUnavailable
	ErrorCodeRuntimeUnavailable         = generated.ErrorCodeRuntimeUnavailable
	ErrorCodeUpdaterUnavailable         = generated.ErrorCodeUpdaterUnavailable
	ErrorCodeUpdaterCheckFailed         = generated.ErrorCodeUpdaterCheckFailed
	LoginItemGetMethod                  = generated.LoginItemGetMethod
	LoginItemSetMethod                  = generated.LoginItemSetMethod
	ErrorCodeLoginItemFailed            = generated.ErrorCodeLoginItemFailed
	InputVolumeGetMethod                = generated.InputVolumeGetMethod
	InputVolumeSetMethod                = generated.InputVolumeSetMethod
	MicrophoneModeGetMethod             = generated.MicrophoneModeGetMethod
	MicrophoneModeSetMethod             = generated.MicrophoneModeSetMethod
	ErrorCodeInputVolumeFailed          = generated.ErrorCodeInputVolumeFailed
	ErrorCodeMicrophoneModeFailed       = generated.ErrorCodeMicrophoneModeFailed
)

type ErrorObject struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	Retriable bool           `json:"retriable"`
}

type RequestEnvelope struct {
	V      int             `json:"v"`
	Kind   string          `json:"kind"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type ResponseEnvelope struct {
	V      int          `json:"v"`
	Kind   string       `json:"kind"`
	ID     string       `json:"id"`
	OK     bool         `json:"ok"`
	Result interface{}  `json:"result,omitempty"`
	Error  *ErrorObject `json:"error,omitempty"`
}

type EventEnvelope struct {
	V       int         `json:"v"`
	Kind    string      `json:"kind"`
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

func NewErrorResponse(id, code, message string, retriable bool, details map[string]any) ResponseEnvelope {
	if details == nil {
		details = map[string]any{}
	}
	return ResponseEnvelope{
		V:    ProtocolVersion,
		Kind: KindResponse,
		ID:   id,
		OK:   false,
		Error: &ErrorObject{
			Code:      code,
			Message:   message,
			Details:   details,
			Retriable: retriable,
		},
	}
}

func NewSuccessResponse(id string, result interface{}) ResponseEnvelope {
	return ResponseEnvelope{
		V:      ProtocolVersion,
		Kind:   KindResponse,
		ID:     id,
		OK:     true,
		Result: result,
	}
}

func NewEvent(event string, payload interface{}) EventEnvelope {
	return EventEnvelope{
		V:       ProtocolVersion,
		Kind:    KindEvent,
		Event:   event,
		Payload: payload,
	}
}
