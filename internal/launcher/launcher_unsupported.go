//go:build !darwin

package launcher

import (
	"os"
	"runtime"
)

func Main() {
	os.Exit(runUnsupported(os.Args[1:], os.Stdout, os.Stderr, runtime.GOOS, runtime.GOARCH))
}
