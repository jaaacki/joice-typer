//go:build darwin

package audio

import (
	"strings"
	"testing"
)

func TestListDevicesConfigHint_UsesCurrentConfigPath(t *testing.T) {
	hint := listDevicesConfigHint()
	if !strings.Contains(hint, "Library/Application Support/JoiceTyper/config.yaml") {
		t.Fatalf("expected current config path in hint, got %q", hint)
	}
	if strings.Contains(hint, "~/.config/voicetype") {
		t.Fatalf("expected old config path to be gone, got %q", hint)
	}
}
