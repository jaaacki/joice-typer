package buildinfra

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestMakeDownloadModelUsesRuntimeModelDir(t *testing.T) {
	root := repoRoot(t)
	home := t.TempDir()

	cmd := exec.Command("make", "-n", "download-model")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n download-model: %v\n%s", err, out)
	}

	want := filepath.Join(home, "Library", "Application Support", "JoiceTyper", "models", "ggml-small.bin")
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected make output to use runtime model path %q\noutput:\n%s", want, out)
	}
}

func TestMakeAppUsesConfiguredPortaudioPrefix(t *testing.T) {
	root := repoRoot(t)
	const portaudioPrefix = "/usr/local/opt/portaudio"

	cmd := exec.Command("make", "-n", "app", "PORTAUDIO_PREFIX="+portaudioPrefix)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n app: %v\n%s", err, out)
	}

	want := filepath.Join(portaudioPrefix, "lib", "libportaudio.2.dylib")
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected make output to use portaudio path %q\noutput:\n%s", want, out)
	}
}
