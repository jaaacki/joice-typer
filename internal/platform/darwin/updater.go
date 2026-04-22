//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework Cocoa
#include "updater_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

var (
	sparkleFeedURL          string
	sparklePublicEDKey      string
	sparkleAutomaticChecks  bool
	resolveUpdaterInfoPlistPath = defaultUpdaterInfoPlistPath
)

type updaterConfig struct {
	FeedURL         string
	PublicEDKey     string
	AutomaticChecks bool
	Enabled         bool
}

func currentUpdaterConfig() updaterConfig {
	cfg := updaterConfig{
		FeedURL:         strings.TrimSpace(sparkleFeedURL),
		PublicEDKey:     strings.TrimSpace(sparklePublicEDKey),
		AutomaticChecks: sparkleAutomaticChecks,
	}
	if cfg.FeedURL == "" || cfg.PublicEDKey == "" {
		if plistCfg, err := loadUpdaterConfigFromInfoPlist(); err == nil {
			if cfg.FeedURL == "" {
				cfg.FeedURL = plistCfg.FeedURL
			}
			if cfg.PublicEDKey == "" {
				cfg.PublicEDKey = plistCfg.PublicEDKey
			}
			if !cfg.AutomaticChecks {
				cfg.AutomaticChecks = plistCfg.AutomaticChecks
			}
		}
	}
	cfg.Enabled = cfg.FeedURL != "" && cfg.PublicEDKey != ""
	return cfg
}

func updaterEnabled() bool {
	return currentUpdaterConfig().Enabled
}

func StartUpdater() {
	cfg := currentUpdaterConfig()
	if !cfg.Enabled {
		return
	}
	if err := startSparkleUpdaterNative(); err != nil {
		currentSettingsLogger().Warn("failed to start sparkle updater", "operation", "StartUpdater", "error", err)
		return
	}
	currentSettingsLogger().Info("sparkle updater ready", "operation", "StartUpdater", "feed_url", cfg.FeedURL)
}

func startSparkleUpdaterNative() error {
	errText := C.startSparkleUpdater()
	if errText == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(errText))
	return fmt.Errorf("%s", C.GoString(errText))
}

func defaultUpdaterInfoPlistPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(filepath.Dir(exePath), "..", "Info.plist")), nil
}

func loadUpdaterConfigFromInfoPlist() (updaterConfig, error) {
	plistPath, err := resolveUpdaterInfoPlistPath()
	if err != nil {
		return updaterConfig{}, err
	}
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return updaterConfig{}, err
	}
	return parseUpdaterConfigFromInfoPlist(data)
}

func parseUpdaterConfigFromInfoPlist(data []byte) (updaterConfig, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	values := map[string]string{}
	currentKey := ""

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return updaterConfig{}, err
		}

		switch tok := token.(type) {
		case xml.StartElement:
			switch tok.Name.Local {
			case "key":
				var key string
				if err := decoder.DecodeElement(&key, &tok); err != nil {
					return updaterConfig{}, err
				}
				currentKey = strings.TrimSpace(key)
			case "string":
				if currentKey == "" {
					var discard string
					if err := decoder.DecodeElement(&discard, &tok); err != nil {
						return updaterConfig{}, err
					}
					continue
				}
				var value string
				if err := decoder.DecodeElement(&value, &tok); err != nil {
					return updaterConfig{}, err
				}
				values[currentKey] = strings.TrimSpace(value)
				currentKey = ""
			case "true":
				if currentKey != "" {
					values[currentKey] = "true"
					currentKey = ""
				}
			case "false":
				if currentKey != "" {
					values[currentKey] = "false"
					currentKey = ""
				}
			}
		}
	}

	cfg := updaterConfig{
		FeedURL:         strings.TrimSpace(values["SUFeedURL"]),
		PublicEDKey:     strings.TrimSpace(values["SUPublicEDKey"]),
		AutomaticChecks: values["SUEnableAutomaticChecks"] == "true",
	}
	cfg.Enabled = cfg.FeedURL != "" && cfg.PublicEDKey != ""
	return cfg, nil
}
