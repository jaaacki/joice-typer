package buildinfra

import (
	"os"
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
