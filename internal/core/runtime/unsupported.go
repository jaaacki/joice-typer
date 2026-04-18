package app

import "fmt"

func UnsupportedDependencyError(component string, operation string, feature string, goos string, goarch string) error {
	return &ErrDependencyUnavailable{
		Component: component,
		Operation: operation,
		Wrapped:   fmt.Errorf("JoiceTyper bootstrap build for %s/%s does not provide %s", goos, goarch, feature),
	}
}
