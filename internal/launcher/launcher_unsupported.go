//go:build !darwin

package launcher

import (
	"fmt"
	"os"
	"runtime"
)

func Main() {
	fmt.Fprintf(os.Stderr, "JoiceTyper desktop runtime is not implemented for %s/%s yet\n", runtime.GOOS, runtime.GOARCH)
	os.Exit(1)
}
