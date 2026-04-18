package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedRecorderSource_PreservesPerMethodOperationMetadata(t *testing.T) {
	sourcePath := filepath.Join("recorder_unsupported.go")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	source := string(data)

	if strings.Contains(source, `UnsupportedDependencyError("recorder", "unsupported"`) {
		t.Fatalf("generic recorder unsupported operation metadata still present in %s", sourcePath)
	}

	for _, op := range []string{
		"InitAudio",
		"TerminateAudio",
		"ListInputDevices",
		"Start",
		"Stop",
		"RefreshDevices",
		"Close",
	} {
		needle := fmt.Sprintf(`unsupportedAudioError("%s")`, op)
		if !strings.Contains(source, needle) {
			t.Fatalf("expected recorder unsupported source to preserve %s metadata", op)
		}
	}
}
