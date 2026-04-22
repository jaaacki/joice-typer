package version

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSemver(t *testing.T) {
	got, err := ParseSemver("1.2.3")
	if err != nil {
		t.Fatalf("ParseSemver: %v", err)
	}
	if got.Major != 1 || got.Minor != 2 || got.Patch != 3 {
		t.Fatalf("unexpected semver: %#v", got)
	}
}

func TestBumpPatch(t *testing.T) {
	got, err := BumpPatch("1.2.3")
	if err != nil {
		t.Fatalf("BumpPatch: %v", err)
	}
	if got != "1.2.4" {
		t.Fatalf("expected 1.2.4, got %q", got)
	}
}

func TestBumpPatch_InvalidVersion(t *testing.T) {
	if _, err := BumpPatch("banana"); err == nil {
		t.Fatal("expected invalid version to fail")
	}
}

func TestLoadVersionFile_TrimsWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "VERSION")
	if err := os.WriteFile(path, []byte("1.0.0\n"), 0644); err != nil {
		t.Fatalf("write version file: %v", err)
	}

	got, err := LoadVersionFile(path)
	if err != nil {
		t.Fatalf("LoadVersionFile: %v", err)
	}
	if got != "1.0.0" {
		t.Fatalf("expected trimmed version, got %q", got)
	}
}

func TestLoadVersionFile_RejectsInvalidVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "VERSION")
	if err := os.WriteFile(path, []byte("banana\n"), 0644); err != nil {
		t.Fatalf("write version file: %v", err)
	}

	_, err := LoadVersionFile(path)
	if err == nil {
		t.Fatal("expected invalid version to fail")
	}
}

func TestValidateReleaseTag(t *testing.T) {
	if err := ValidateReleaseTag("1.0.0", "v1.0.0"); err != nil {
		t.Fatalf("expected matching tag to pass, got %v", err)
	}

	err := ValidateReleaseTag("1.0.0", "v1.0.1")
	if err == nil {
		t.Fatal("expected mismatched tag to fail")
	}
	if !strings.Contains(err.Error(), "1.0.0") {
		t.Fatalf("expected mismatch error to mention version, got %v", err)
	}

	err = ValidateReleaseTag("1.0.0", "1.0.0")
	if err == nil {
		t.Fatal("expected malformed tag to fail")
	}
}

func TestRenderInfoPlist(t *testing.T) {
	rendered, err := RenderInfoPlist(
		"<plist><string>{{VERSION}}</string><string>{{VERSION}}</string></plist>",
		"1.0.0",
	)
	if err != nil {
		t.Fatalf("RenderInfoPlist: %v", err)
	}

	if strings.Count(rendered, "1.0.0") != 2 {
		t.Fatalf("expected rendered plist to contain version twice, got %q", rendered)
	}
}

func TestFormatVersion(t *testing.T) {
	if got := FormatVersion("1.0.0"); got != "JoiceTyper 1.0.0" {
		t.Fatalf("unexpected formatted version: %q", got)
	}
}
