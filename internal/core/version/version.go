package version

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var Version = "dev"

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func LoadVersionFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("version.LoadVersionFile: %w", err)
	}

	version := strings.TrimSpace(string(data))
	if !semverPattern.MatchString(version) {
		return "", fmt.Errorf("version.LoadVersionFile: invalid version %q", version)
	}
	return version, nil
}

func ValidateReleaseTag(version string, tag string) error {
	if !semverPattern.MatchString(version) {
		return fmt.Errorf("version.ValidateReleaseTag: invalid version %q", version)
	}

	expected := "v" + version
	if tag != expected {
		return fmt.Errorf("version.ValidateReleaseTag: tag %q does not match version %q", tag, version)
	}
	return nil
}

func RenderInfoPlist(template string, version string) (string, error) {
	if !semverPattern.MatchString(version) {
		return "", fmt.Errorf("version.RenderInfoPlist: invalid version %q", version)
	}
	return strings.ReplaceAll(template, "{{VERSION}}", version), nil
}

func FormatVersion(version string) string {
	return "JoiceTyper " + version
}
