package buildinfra

import (
	"os"
	"os/exec"
	"path/filepath"
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
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = filepath.Join(root, "ui")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("npm run build: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "ui", "dist", "index.html")); err != nil {
		t.Fatalf("expected ui/dist/index.html: %v", err)
	}
}
