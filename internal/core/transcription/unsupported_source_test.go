package transcription

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedTranscriberSource_PreservesPerMethodOperationMetadata(t *testing.T) {
	sourcePath := filepath.Join("transcriber_unsupported.go")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	source := string(data)

	if strings.Contains(source, `UnsupportedDependencyError("transcriber", "unsupported"`) {
		t.Fatalf("generic transcriber unsupported operation metadata still present in %s", sourcePath)
	}

	for _, op := range []string{
		"NewTranscriber",
		"Transcribe",
		"Close",
	} {
		needle := fmt.Sprintf(`unsupportedTranscriptionError("%s")`, op)
		if !strings.Contains(source, needle) {
			t.Fatalf("expected transcriber unsupported source to preserve %s metadata", op)
		}
	}
}
