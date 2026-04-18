package launcher

import (
	"flag"
	"fmt"
	"io"

	version "voicetype/internal/core/version"
)

func runUnsupported(args []string, stdout io.Writer, stderr io.Writer, goos string, goarch string) int {
	fs := flag.NewFlagSet("joicetyper", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showVersion := fs.Bool("version", false, "print version and exit")
	listDevices := fs.Bool("list-devices", false, "list available audio input devices and exit")
	_ = fs.String("config", "", "path to config file")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version.FormatVersion(version.Version))
		return 0
	}
	if *listDevices {
		fmt.Fprintf(stderr, "JoiceTyper bootstrap build for %s/%s does not provide audio device listing yet\n", goos, goarch)
		return 1
	}

	fmt.Fprintf(stderr, "JoiceTyper bootstrap build for %s/%s does not provide the desktop runtime yet\n", goos, goarch)
	return 1
}
