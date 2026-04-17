package transcription

import (
	"testing"
	"time"
)

func TestDurationLimits_UseNinetySeconds(t *testing.T) {
	if transcribeTimeout != 90*time.Second {
		t.Fatalf("expected transcribeTimeout 90s, got %s", transcribeTimeout)
	}
	if maxTranscribeSeconds != 90 {
		t.Fatalf("expected maxTranscribeSeconds 90, got %d", maxTranscribeSeconds)
	}
}
