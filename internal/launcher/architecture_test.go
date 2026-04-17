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
