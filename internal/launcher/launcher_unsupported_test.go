package launcher

import (
	"bytes"
	"strings"
	"testing"

	version "voicetype/internal/version"
)

func TestRunUnsupportedVersionFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runUnsupported([]string{"--version"}, &stdout, &stderr, "windows", "amd64")

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != version.FormatVersion(version.Version) {
		t.Fatalf("expected version output %q, got %q", version.FormatVersion(version.Version), got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", stderr.String())
	}
}

func TestRunUnsupportedListDevicesFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runUnsupported([]string{"--list-devices"}, &stdout, &stderr, "windows", "amd64")

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "not implemented") {
		t.Fatalf("expected unsupported error, got %q", stderr.String())
	}
}
