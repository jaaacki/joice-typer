package bridge

import "testing"

func TestNewErrorResponseFromError_PreservesContractError(t *testing.T) {
	response := NewErrorResponseFromError(
		"req-1",
		NewContractError(
			ErrorCodeDevicesEnumerationFailed,
			"Failed to list input devices",
			true,
			map[string]any{"source": "portaudio"},
		),
		ErrorCodeInternal,
		"fallback",
		false,
		nil,
	)

	if response.OK {
		t.Fatal("expected failure response")
	}
	if response.Error == nil {
		t.Fatal("expected error object")
	}
	if response.Error.Code != ErrorCodeDevicesEnumerationFailed {
		t.Fatalf("Code = %q, want %q", response.Error.Code, ErrorCodeDevicesEnumerationFailed)
	}
	if !response.Error.Retriable {
		t.Fatal("expected retriable contract error to remain retriable")
	}
	if got := response.Error.Details["source"]; got != "portaudio" {
		t.Fatalf("Details[source] = %#v, want portaudio", got)
	}
}
