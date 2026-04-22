//go:build darwin

package darwin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdaterIsDisabledForDevBuilds(t *testing.T) {
	origFeed := sparkleFeedURL
	origKey := sparklePublicEDKey
	origChecks := sparkleAutomaticChecks
	origResolve := resolveUpdaterInfoPlistPath
	defer func() {
		sparkleFeedURL = origFeed
		sparklePublicEDKey = origKey
		sparkleAutomaticChecks = origChecks
		resolveUpdaterInfoPlistPath = origResolve
	}()

	sparkleFeedURL = ""
	sparklePublicEDKey = ""
	sparkleAutomaticChecks = false
	resolveUpdaterInfoPlistPath = defaultUpdaterInfoPlistPath

	cfg := currentUpdaterConfig()
	if cfg.Enabled {
		t.Fatal("expected updater to be disabled by default in dev builds")
	}
	if updaterEnabled() {
		t.Fatal("expected updaterEnabled to be false for dev builds")
	}
}

func TestUpdaterConfig_EnablesWhenSparkleMetadataPresent(t *testing.T) {
	origFeed := sparkleFeedURL
	origKey := sparklePublicEDKey
	origChecks := sparkleAutomaticChecks
	origResolve := resolveUpdaterInfoPlistPath
	defer func() {
		sparkleFeedURL = origFeed
		sparklePublicEDKey = origKey
		sparkleAutomaticChecks = origChecks
		resolveUpdaterInfoPlistPath = origResolve
	}()

	sparkleFeedURL = "https://example.com/appcast.xml"
	sparklePublicEDKey = "PUBLIC_KEY"
	sparkleAutomaticChecks = true

	cfg := currentUpdaterConfig()
	if !cfg.Enabled {
		t.Fatal("expected updater to be enabled when Sparkle metadata is present")
	}
	if !cfg.AutomaticChecks {
		t.Fatal("expected updater automatic checks to track release metadata")
	}
}

func TestUpdaterConfig_FallsBackToInfoPlist(t *testing.T) {
	origFeed := sparkleFeedURL
	origKey := sparklePublicEDKey
	origChecks := sparkleAutomaticChecks
	origResolve := resolveUpdaterInfoPlistPath
	defer func() {
		sparkleFeedURL = origFeed
		sparklePublicEDKey = origKey
		sparkleAutomaticChecks = origChecks
		resolveUpdaterInfoPlistPath = origResolve
	}()

	sparkleFeedURL = ""
	sparklePublicEDKey = ""
	sparkleAutomaticChecks = false

	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "Info.plist")
	if err := os.WriteFile(plistPath, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>SUFeedURL</key>
  <string>https://example.com/appcast.xml</string>
  <key>SUPublicEDKey</key>
  <string>PUBLIC_KEY</string>
  <key>SUEnableAutomaticChecks</key>
  <true/>
</dict>
</plist>`), 0644); err != nil {
		t.Fatalf("write temp plist: %v", err)
	}
	resolveUpdaterInfoPlistPath = func() (string, error) { return plistPath, nil }

	cfg := currentUpdaterConfig()
	if !cfg.Enabled {
		t.Fatal("expected updater to be enabled from Info.plist fallback")
	}
	if cfg.FeedURL != "https://example.com/appcast.xml" {
		t.Fatalf("FeedURL = %q", cfg.FeedURL)
	}
	if cfg.PublicEDKey != "PUBLIC_KEY" {
		t.Fatalf("PublicEDKey = %q", cfg.PublicEDKey)
	}
	if !cfg.AutomaticChecks {
		t.Fatal("expected automatic checks to be true from Info.plist fallback")
	}
}
