//go:build windows

package windows

import (
	"context"
	"log/slog"
)

func IsFirstRun() bool {
	return false
}

func RunSetupWizard(context.Context, *slog.Logger) (string, error) {
	return "", nil
}
