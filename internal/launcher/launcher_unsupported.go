//go:build !darwin && !windows

package launcher

import (
	"os"
	"runtime"
)

func Main() {
	os.Exit(runUnsupported(os.Args[1:], os.Stdout, os.Stderr, runtime.GOOS, runtime.GOARCH))
}
