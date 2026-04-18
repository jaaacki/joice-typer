package buildinfra

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoLayout_FutureHomesExist(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"ui",
		"assets",
		"assets/icons",
		"assets/macos",
		"assets/windows",
		"packaging",
		"packaging/macos",
		"packaging/windows",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestRepoLayout_PackagingHomesDocumented(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"packaging/macos/README.md",
		"packaging/windows/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestRepoLayout_FrontendToolchainFilesExist(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"ui/package.json",
		"ui/tsconfig.json",
		"ui/vite.config.ts",
		"ui/index.html",
		"ui/src/main.tsx",
		"ui/src/App.tsx",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestFrontendBuild_ProducesDistIndex(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("make", "frontend-build")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make frontend-build: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "ui", "dist", "index.html")); err != nil {
		t.Fatalf("expected ui/dist/index.html: %v", err)
	}
}

func TestSettingsScreenSource_DoesNotClaimSaveBeforeAck(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ui", "src", "settings", "SettingsScreen.tsx"))
	if err != nil {
		t.Fatalf("read SettingsScreen.tsx: %v", err)
	}
	source := string(data)
	if strings.Contains(source, "Saved. JoiceTyper is reloading the runtime.") {
		t.Fatal("expected settings screen to avoid claiming save success before native acknowledgement")
	}
}
