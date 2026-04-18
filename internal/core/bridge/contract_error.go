package bridge

import "errors"

type ContractError struct {
	Code      string
	Message   string
	Details   map[string]any
	Retriable bool
	Cause     error
}

func (e *ContractError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}

func (e *ContractError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewContractError(code, message string, retriable bool, details map[string]any) *ContractError {
	return &ContractError{
		Code:      code,
		Message:   message,
		Details:   cloneDetails(details),
		Retriable: retriable,
	}
}

func WrapContractError(code, message string, retriable bool, details map[string]any, cause error) *ContractError {
	err := NewContractError(code, message, retriable, details)
	err.Cause = cause
	return err
}

func AsContractError(err error) (*ContractError, bool) {
	var contractErr *ContractError
	if errors.As(err, &contractErr) {
		return contractErr, true
	}
	return nil, false
}

func NewErrorResponseFromError(id string, err error, fallbackCode, fallbackMessage string, retriable bool, details map[string]any) ResponseEnvelope {
	if contractErr, ok := AsContractError(err); ok {
		message := contractErr.Message
		if message == "" {
			message = fallbackMessage
		}
		return NewErrorResponse(id, contractErr.Code, message, contractErr.Retriable, contractErr.Details)
	}
	return NewErrorResponse(id, fallbackCode, fallbackMessage, retriable, details)
}

func cloneDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}
	clone := make(map[string]any, len(details))
	for key, value := range details {
		clone[key] = value
	}
	return clone
}
