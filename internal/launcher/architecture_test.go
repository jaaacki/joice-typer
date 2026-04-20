package launcher

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func launcherDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func TestLauncherDoesNotKeepDarwinShimFile(t *testing.T) {
	path := filepath.Join(launcherDir(t), "deps.go")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("launcher Darwin shim still present: %s", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat deps.go: %v", err)
	}
}

func TestLauncherDoesNotImportDarwinPackageDirectly(t *testing.T) {
	path := filepath.Join(launcherDir(t), "launcher.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read launcher.go: %v", err)
	}
	if strings.Contains(string(data), "internal/platform/darwin") {
		t.Fatalf("launcher.go still imports internal/platform/darwin directly")
	}
}

func TestArchitecture_UsesCorePackages(t *testing.T) {
	path := filepath.Join(launcherDir(t), "launcher.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read launcher.go: %v", err)
	}
	text := string(data)

	for _, needle := range []string{
		"voicetype/internal/app",
		"voicetype/internal/config",
		"voicetype/internal/logging",
		"voicetype/internal/version",
		"voicetype/internal/transcription",
		"voicetype/internal/audio",
	} {
		if strings.Contains(text, needle) {
			t.Fatalf("found old import %q", needle)
		}
	}

	for _, needle := range []string{
		"voicetype/internal/core/runtime",
		"voicetype/internal/core/config",
		"voicetype/internal/core/logging",
		"voicetype/internal/core/version",
		"voicetype/internal/core/transcription",
		"voicetype/internal/core/audio",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing new import %q", needle)
		}
	}
}

func TestLauncherUnsupportedExcludesWindows(t *testing.T) {
	path := filepath.Join(launcherDir(t), "launcher_unsupported.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read launcher_unsupported.go: %v", err)
	}
	source := string(data)
	if strings.Contains(source, "//go:build !darwin\n") || strings.Contains(source, "//go:build !darwin\r\n") {
		t.Fatalf("launcher_unsupported.go still includes windows in unsupported build tag")
	}
	if !strings.Contains(source, "!windows") {
		t.Fatalf("launcher_unsupported.go must explicitly exclude windows")
	}
}

func TestWindowsLauncherExistsAndAvoidsBootstrapShim(t *testing.T) {
	path := filepath.Join(launcherDir(t), "launcher_windows.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read launcher_windows.go: %v", err)
	}
	source := string(data)
	if strings.Contains(source, "runUnsupported(") {
		t.Fatalf("launcher_windows.go must not route through runUnsupported")
	}
	for _, needle := range []string{
		"voicetype/internal/core/runtime",
		"voicetype/internal/core/config",
		"voicetype/internal/core/logging",
		"voicetype/internal/core/version",
		"voicetype/internal/platform",
	} {
		if !strings.Contains(source, needle) {
			t.Fatalf("launcher_windows.go missing shared import %q", needle)
		}
	}
}
