package version

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var Version = "dev"

func DisplayVersion() string {
	return Version
}

var semverPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

type Semver struct {
	Major int
	Minor int
	Patch int
}

func ParseSemver(version string) (Semver, error) {
	matches := semverPattern.FindStringSubmatch(strings.TrimSpace(version))
	if matches == nil {
		return Semver{}, fmt.Errorf("version.ParseSemver: invalid version %q", version)
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return Semver{}, fmt.Errorf("version.ParseSemver: parse major: %w", err)
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return Semver{}, fmt.Errorf("version.ParseSemver: parse minor: %w", err)
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return Semver{}, fmt.Errorf("version.ParseSemver: parse patch: %w", err)
	}
	return Semver{Major: major, Minor: minor, Patch: patch}, nil
}

func (s Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

func BumpPatch(version string) (string, error) {
	semver, err := ParseSemver(version)
	if err != nil {
		return "", fmt.Errorf("version.BumpPatch: %w", err)
	}
	semver.Patch++
	return semver.String(), nil
}

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
