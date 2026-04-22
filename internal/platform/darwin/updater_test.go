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
	origStart := startSparkleUpdaterNativeFn
	origCheck := checkSparkleUpdaterNativeFn
	defer func() {
		sparkleFeedURL = origFeed
		sparklePublicEDKey = origKey
		sparkleAutomaticChecks = origChecks
		resolveUpdaterInfoPlistPath = origResolve
		startSparkleUpdaterNativeFn = origStart
		checkSparkleUpdaterNativeFn = origCheck
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
	snapshot := currentUpdaterSnapshot()
	if snapshot.Enabled {
		t.Fatal("expected updater snapshot to be disabled for dev builds")
	}
	if snapshot.SupportsManualCheck {
		t.Fatal("expected manual check support to be disabled for dev builds")
	}
	if err := CheckForUpdates(); err == nil {
		t.Fatal("expected CheckForUpdates to fail when updater metadata is missing")
	}
}

func TestUpdaterConfig_EnablesWhenSparkleMetadataPresent(t *testing.T) {
	origFeed := sparkleFeedURL
	origKey := sparklePublicEDKey
	origChecks := sparkleAutomaticChecks
	origResolve := resolveUpdaterInfoPlistPath
	origStart := startSparkleUpdaterNativeFn
	origCheck := checkSparkleUpdaterNativeFn
	defer func() {
		sparkleFeedURL = origFeed
		sparklePublicEDKey = origKey
		sparkleAutomaticChecks = origChecks
		resolveUpdaterInfoPlistPath = origResolve
		startSparkleUpdaterNativeFn = origStart
		checkSparkleUpdaterNativeFn = origCheck
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
	snapshot := currentUpdaterSnapshot()
	if !snapshot.Enabled || !snapshot.SupportsManualCheck {
		t.Fatalf("snapshot = %#v, want enabled/manual-check updater", snapshot)
	}
	startCalls := 0
	checkCalls := 0
	startSparkleUpdaterNativeFn = func() error {
		startCalls++
		return nil
	}
	checkSparkleUpdaterNativeFn = func() error {
		checkCalls++
		return nil
	}
	if err := CheckForUpdates(); err != nil {
		t.Fatalf("CheckForUpdates returned error: %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("startSparkleUpdaterNativeFn calls = %d, want 1", startCalls)
	}
	if checkCalls != 1 {
		t.Fatalf("checkSparkleUpdaterNativeFn calls = %d, want 1", checkCalls)
	}
}

func TestUpdaterConfig_FallsBackToInfoPlist(t *testing.T) {
	origFeed := sparkleFeedURL
	origKey := sparklePublicEDKey
	origChecks := sparkleAutomaticChecks
	origResolve := resolveUpdaterInfoPlistPath
	origStart := startSparkleUpdaterNativeFn
	origCheck := checkSparkleUpdaterNativeFn
	defer func() {
		sparkleFeedURL = origFeed
		sparklePublicEDKey = origKey
		sparkleAutomaticChecks = origChecks
		resolveUpdaterInfoPlistPath = origResolve
		startSparkleUpdaterNativeFn = origStart
		checkSparkleUpdaterNativeFn = origCheck
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
	snapshot := currentUpdaterSnapshot()
	if !snapshot.Enabled || snapshot.FeedURL != "https://example.com/appcast.xml" {
		t.Fatalf("snapshot = %#v, want enabled plist-backed updater", snapshot)
	}
}
