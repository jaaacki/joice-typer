package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestContractsDoNotOverstateBehavior(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "contracts.go"))
	if err != nil {
		t.Fatalf("read contracts.go: %v", err)
	}
	text := string(data)

	forbidden := []string{
		"CONTRACTS — These interfaces are ABSOLUTE.",
		"prompting the user via macOS dialogs",
	}
	for _, s := range forbidden {
		if strings.Contains(text, s) {
			t.Fatalf("contracts.go still contains overstated wording %q", s)
		}
	}
}
