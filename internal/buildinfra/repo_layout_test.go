package buildinfra

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoLayout_FutureHomesExist(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"ui",
		"assets",
		"assets/icons",
		"assets/macos",
		"assets/windows",
		"packaging",
		"packaging/macos",
		"packaging/windows",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestRepoLayout_PackagingHomesDocumented(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"packaging/macos/README.md",
		"packaging/windows/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestRepoLayout_FrontendToolchainFilesExist(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"ui/package.json",
		"ui/tsconfig.json",
		"ui/vite.config.ts",
		"ui/index.html",
		"ui/src/main.tsx",
		"ui/src/App.tsx",
		"ui/src/bridge/client.ts",
		"ui/src/bridge/index.ts",
		"ui/src/bridge/generated/protocol.ts",
		"internal/core/bridge/generated/protocol_gen.go",
		"contracts/bridge/v1/catalog.json",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "ui", "src", "bridge.ts")); err == nil {
		t.Fatal("expected legacy ui/src/bridge.ts to be removed")
	}
}

func TestFrontendBuild_ProducesDistIndex(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("make", "frontend-build")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make frontend-build: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "ui", "dist", "index.html")); err != nil {
		t.Fatalf("expected ui/dist/index.html: %v", err)
	}
}

func TestBridgeContractGeneratorOutputsAreCurrent(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./scripts/generate_bridge_contract", "-check")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bridge contract generator check failed: %v\n%s", err, out)
	}
}

func TestGeneratedBridgeProtocol_ContainsCutoverCommandsAndErrors(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"internal/core/bridge/generated/protocol_gen.go",
		"ui/src/bridge/generated/protocol.ts",
	} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source := string(data)
		for _, snippet := range []string{
			"devices.refresh",
			"model.download",
			"model.delete",
			"model.use",
			"hotkey.capture_start",
			"hotkey.capture_cancel",
			"hotkey.capture_confirm",
			"hotkey.capture_changed",
			"devices.refresh_failed",
			"model.download_failed",
			"model.delete_failed",
			"model.use_failed",
		} {
			if !strings.Contains(source, snippet) {
				t.Fatalf("expected %s to contain %q", path, snippet)
			}
		}
	}
}

func TestSettingsScreenSource_UsesAckAwareSaveStates(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ui", "src", "settings", "SettingsScreen.tsx"))
	if err != nil {
		t.Fatalf("read SettingsScreen.tsx: %v", err)
	}
	source := string(data)
	for _, snippet := range []string{
		"Waiting for native confirmation.",
		"await saveConfig(draft)",
		"Saved. JoiceTyper is reloading the runtime.",
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected SettingsScreen.tsx to contain %q", snippet)
		}
	}
}

func TestSettingsScreenSource_PollsPermissionsUntilGranted(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ui", "src", "settings", "SettingsScreen.tsx"))
	if err != nil {
		t.Fatalf("read SettingsScreen.tsx: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`fetchPermissions,`,
		`const refreshLivePermissions = async () => {`,
		`window.setInterval(() => {`,
		`window.addEventListener("focus", onAttention)`,
		`document.addEventListener("visibilitychange", onAttention)`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected SettingsScreen.tsx to contain %q", required)
		}
	}
}

func TestSettingsScreenSource_UsesSharedHotkeyCapabilities(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ui", "src", "settings", "SettingsScreen.tsx"))
	if err != nil {
		t.Fatalf("read SettingsScreen.tsx: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`options.hotkey.modifiers`,
		`options.hotkey.keys`,
		`This platform does not support the current hotkey`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected SettingsScreen.tsx to contain %q", required)
		}
	}
}

func TestSettingsScreenSource_PutsVersionChipInHeader(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ui", "src", "settings", "SettingsScreen.tsx"))
	if err != nil {
		t.Fatalf("read SettingsScreen.tsx: %v", err)
	}
	source := string(data)
	headerIndex := strings.Index(source, `<header className="settings-screen__header">`)
	if headerIndex == -1 {
		t.Fatal("expected settings header")
	}
	gridIndex := strings.Index(source, `<div className="settings-grid">`)
	if gridIndex == -1 {
		t.Fatal("expected settings grid")
	}
	headerSlice := source[headerIndex:gridIndex]
	if !strings.Contains(headerSlice, `className="version-chip"`) {
		t.Fatal("expected version chip to live in the settings header")
	}
	if strings.Contains(source, `<h2>Vocabulary</h2>`) && strings.Contains(source, `<span className="version-chip">{currentAppState.version}</span>`) && !strings.Contains(headerSlice, `<span className="version-chip">{currentAppState.version}</span>`) {
		t.Fatal("expected version chip to stop living in the Vocabulary section")
	}
}

func TestSettingsScreenSource_UsesSharedLogsPaneBridge(t *testing.T) {
	root := repoRoot(t)

	panePath := filepath.Join(root, "ui", "src", "settings", "panes", "LogsPane.tsx")
	data, err := os.ReadFile(panePath)
	if err != nil {
		t.Fatalf("read LogsPane.tsx: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`fetchLogs`,
		`copyFullLog`,
		`subscribeLogsUpdated`,
		`Copy Full Log`,
		`settings-logs`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected LogsPane.tsx to contain %q", required)
		}
	}

	bridgeSource, err := os.ReadFile(filepath.Join(root, "ui", "src", "bridge", "client.ts"))
	if err != nil {
		t.Fatalf("read bridge/client.ts: %v", err)
	}
	bridge := string(bridgeSource)
	for _, required := range []string{
		`METHODS.logsGet`,
		`METHODS.logsCopyAll`,
		`EVENTS.logsUpdated`,
		`fetchLogs`,
		`copyFullLog`,
		`subscribeLogsUpdated`,
	} {
		if !strings.Contains(bridge, required) {
			t.Fatalf("expected bridge/client.ts to contain %q", required)
		}
	}

	settingsSource := readSettingsSourceTree(t)
	for _, forbidden := range []string{
		`webkit?.messageHandlers`,
		`postMessage(`,
		`window.webkit`,
		`joicetyper-bridge-message`,
	} {
		if strings.Contains(settingsSource, forbidden) {
			t.Fatalf("expected settings sources not to contain %q; bridge boundary violation", forbidden)
		}
	}
}

func TestDarwinWebviewTransportSource_LogsNativeFailures(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "darwin", "webview_darwin.m"))
	if err != nil {
		t.Fatalf("read webview_darwin.m: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`webSettingsNativeTransportWarning`,
		`invalid web settings message body`,
		`failed to encode web settings message`,
		`failed to decode web settings message`,
		`failed to duplicate web settings request`,
		`failed to evaluate bridge envelope dispatch`,
		`failed to dispatch bridge envelope in page`,
		`failed to encode bridge payload string literal`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected webview_darwin.m to contain %q", required)
		}
	}
}

func TestWindowsLauncherSource_UsesRealLauncherAndExcludesUnsupportedShim(t *testing.T) {
	root := repoRoot(t)

	unsupportedPath := filepath.Join(root, "internal", "launcher", "launcher_unsupported.go")
	unsupportedData, err := os.ReadFile(unsupportedPath)
	if err != nil {
		t.Fatalf("read launcher_unsupported.go: %v", err)
	}
	unsupported := string(unsupportedData)
	if !strings.Contains(unsupported, "//go:build !darwin && !windows") {
		t.Fatalf("expected launcher_unsupported.go to exclude windows explicitly")
	}

	windowsPath := filepath.Join(root, "internal", "launcher", "launcher_windows.go")
	windowsData, err := os.ReadFile(windowsPath)
	if err != nil {
		t.Fatalf("read launcher_windows.go: %v", err)
	}
	source := string(windowsData)
	if strings.Contains(source, "runUnsupported(") {
		t.Fatalf("expected launcher_windows.go not to route through runUnsupported")
	}
	for _, required := range []string{
		"voicetype/internal/core/runtime",
		"voicetype/internal/core/config",
		"voicetype/internal/core/logging",
		"voicetype/internal/core/version",
		"voicetype/internal/platform",
		"runWindowsDesktopMode",
		"platformpkg.InitStatusBar()",
		"platformpkg.HotkeyRestartCh()",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected launcher_windows.go to contain %q", required)
		}
	}
}

func TestWindowsAudioSource_UsesDedicatedWindowsSurfaceAndExcludesUnsupportedShim(t *testing.T) {
	root := repoRoot(t)

	unsupportedPath := filepath.Join(root, "internal", "core", "audio", "recorder_unsupported.go")
	unsupportedData, err := os.ReadFile(unsupportedPath)
	if err != nil {
		t.Fatalf("read recorder_unsupported.go: %v", err)
	}
	unsupported := string(unsupportedData)
	if !strings.Contains(unsupported, "//go:build !darwin && !windows") {
		t.Fatalf("expected recorder_unsupported.go to exclude windows explicitly")
	}

	windowsPath := filepath.Join(root, "internal", "core", "audio", "recorder_windows.go")
	windowsData, err := os.ReadFile(windowsPath)
	if err != nil {
		t.Fatalf("read recorder_windows.go: %v", err)
	}
	source := string(windowsData)
	for _, required := range []string{
		"//go:build windows",
		"ListInputDeviceSnapshots",
		"ListInputDevices",
		"InitAudio",
		"TerminateAudio",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected recorder_windows.go to contain %q", required)
		}
	}
}

func TestWindowsPackagingSource_StagesNativeWhisperRuntime(t *testing.T) {
	root := repoRoot(t)

	makefileData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(makefileData)
	for _, required := range []string{
		"WINDOWS_RUNTIME_DIR :=",
		"WINDOWS_RUNTIME_DLLS :=",
		"build-windows-amd64:",
		"package-windows: build-windows-runtime-amd64 windows-runtime-stage-check",
	} {
		if !strings.Contains(makefile, required) {
			t.Fatalf("expected Makefile to contain %q", required)
		}
	}

	issData, err := os.ReadFile(filepath.Join(root, "packaging", "windows", "joicetyper.iss"))
	if err != nil {
		t.Fatalf("read packaging/windows/joicetyper.iss: %v", err)
	}
	iss := string(issData)
	for _, required := range []string{
		`Source: "{#MyAppSourceDir}\whisper.dll"; DestDir: "{app}"; Flags: ignoreversion`,
		`Source: "{#MyAppSourceDir}\ggml.dll"; DestDir: "{app}"; Flags: ignoreversion`,
		`Source: "{#MyAppSourceDir}\ggml-base.dll"; DestDir: "{app}"; Flags: ignoreversion`,
		`Source: "{#MyAppSourceDir}\ggml-cpu.dll"; DestDir: "{app}"; Flags: ignoreversion`,
	} {
		if !strings.Contains(iss, required) {
			t.Fatalf("expected Windows installer to stage %q", required)
		}
	}
}

func TestWindowsTranscriptionSource_UsesDedicatedWindowsCGOPath(t *testing.T) {
	root := repoRoot(t)

	unsupportedPath := filepath.Join(root, "internal", "core", "transcription", "transcriber_unsupported.go")
	unsupportedData, err := os.ReadFile(unsupportedPath)
	if err != nil {
		t.Fatalf("read transcriber_unsupported.go: %v", err)
	}
	unsupported := string(unsupportedData)
	if !strings.Contains(unsupported, "//go:build !darwin && (!windows || !cgo)") {
		t.Fatalf("expected transcriber_unsupported.go to exclude windows+cgo explicitly")
	}

	windowsPath := filepath.Join(root, "internal", "core", "transcription", "transcriber_windows.go")
	windowsData, err := os.ReadFile(windowsPath)
	if err != nil {
		t.Fatalf("read transcriber_windows.go: %v", err)
	}
	source := string(windowsData)
	for _, required := range []string{
		"//go:build windows && cgo",
		"#cgo windows CFLAGS:",
		"#cgo windows LDFLAGS:",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected transcriber_windows.go to contain %q", required)
		}
	}

	commonPath := filepath.Join(root, "internal", "core", "transcription", "transcriber_cgo_common.go")
	commonData, err := os.ReadFile(commonPath)
	if err != nil {
		t.Fatalf("read transcriber_cgo_common.go: %v", err)
	}
	common := string(commonData)
	for _, required := range []string{
		"//go:build darwin || (windows && cgo)",
		"#include <whisper.h>",
		"whisper_init_from_file_with_params",
		"whisper_full",
		"whisper_free",
	} {
		if !strings.Contains(common, required) {
			t.Fatalf("expected transcriber_cgo_common.go to contain %q", required)
		}
	}

	makefileData, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(makefileData)
	for _, required := range []string{
		"build-windows-runtime-amd64:",
		"package-windows-runtime:",
		"CGO_ENABLED=1",
		"WINDOWS_CC ?=",
		"WINDOWS_CXX ?=",
		"CC=$(WINDOWS_CC)",
		"CXX=$(WINDOWS_CXX)",
		"WINDOWS_RUNTIME_IMPORT_DIR :=",
		"windows-runtime-prereqs:",
		"windows-runtime-stage-check:",
		"fatal: missing Windows runtime payload",
		"fatal: missing staged Windows runtime artifact",
	} {
		if !strings.Contains(makefile, required) {
			t.Fatalf("expected Makefile to contain %q", required)
		}
	}
}

func TestWindowsRecorderSource_UsesDedicatedWindowsCGOPath(t *testing.T) {
	root := repoRoot(t)

	recorderData, err := os.ReadFile(filepath.Join(root, "internal", "core", "audio", "recorder.go"))
	if err != nil {
		t.Fatalf("read recorder.go: %v", err)
	}
	if !strings.Contains(string(recorderData), "//go:build darwin || (windows && cgo)") {
		t.Fatalf("expected recorder.go to include windows+cgo build tag")
	}

	stubData, err := os.ReadFile(filepath.Join(root, "internal", "core", "audio", "recorder_windows.go"))
	if err != nil {
		t.Fatalf("read recorder_windows.go: %v", err)
	}
	if !strings.Contains(string(stubData), "//go:build windows && !cgo") {
		t.Fatalf("expected recorder_windows.go to be the windows !cgo stub")
	}

	devicesPath := filepath.Join(root, "internal", "core", "audio", "audio_devices_windows.go")
	devicesData, err := os.ReadFile(devicesPath)
	if err != nil {
		t.Fatalf("read audio_devices_windows.go: %v", err)
	}
	devicesSource := string(devicesData)
	for _, required := range []string{
		"//go:build windows",
		"func ListInputDeviceSnapshots() ([]bridgepkg.DeviceSnapshot, error) {",
		"wca.CLSID_MMDeviceEnumerator",
		"wca.PKEY_Device_FriendlyName",
	} {
		if !strings.Contains(devicesSource, required) {
			t.Fatalf("expected audio_devices_windows.go to contain %q", required)
		}
	}
}

func TestDarwinSettingsSource_UsesSharedAudioDeviceSnapshots(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "darwin", "settings.go"))
	if err != nil {
		t.Fatalf("read darwin/settings.go: %v", err)
	}
	source := string(data)
	if !strings.Contains(source, `listAudioDevices = audiopkg.ListInputDeviceSnapshots`) {
		t.Fatalf("expected darwin/settings.go to use shared core audio device snapshots")
	}

	recorderData, err := os.ReadFile(filepath.Join(root, "internal", "core", "audio", "recorder.go"))
	if err != nil {
		t.Fatalf("read core/audio/recorder.go: %v", err)
	}
	if !strings.Contains(string(recorderData), `func ListInputDeviceSnapshots() ([]bridgepkg.DeviceSnapshot, error) {`) {
		t.Fatalf("expected core/audio/recorder.go to define ListInputDeviceSnapshots")
	}
}

func TestWindowsWebviewTransportSource_LogsNativeFailures(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "webview_host_windows.go"))
	if err != nil {
		t.Fatalf("read webview_host_windows.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`webSettingsNativeTransportWarning("showWindowsWebView2Host", err.Error())`,
		`webSettingsNativeTransportWarning("focusWindowsWebView2Host", err.Error())`,
		`webSettingsNativeTransportWarning("dispatchWindowsWebView2Envelope", err.Error())`,
		`webSettingsNativeTransportWarning("windowsWebView2Host", err.Error())`,
		`detectInstalledWebView2Version()`,
		`Install Microsoft Edge WebView2 Runtime`,
		`webView2RuntimeRegistryGUID`,
		`Software\Microsoft\EdgeUpdate\Clients\`,
		`window.dispatchEvent(new CustomEvent("`,
		`function dispatchBridgePayload(payload)`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected webview_host_windows.go to contain %q", required)
		}
	}
	if strings.Contains(source, `window.__JOICETYPER_NATIVE_BRIDGE_DISPATCH__`) {
		t.Fatal("expected Windows WebView2 transport to stop depending on a page-owned bridge dispatch function")
	}
}

func TestSharedSettingsSource_ObservesLogWritesForLiveLogsPane(t *testing.T) {
	root := repoRoot(t)
	targets := []string{
		filepath.Join(root, "internal", "platform", "darwin", "settings.go"),
		filepath.Join(root, "internal", "platform", "windows", "settings.go"),
	}
	for _, target := range targets {
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(target), err)
		}
		source := string(data)
		for _, required := range []string{
			`registerLogWriteObserver`,
			`preferencesOpenLoad()`,
			`scheduleWebSettingsLogsUpdated()`,
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("expected %s to contain %q", filepath.Base(target), required)
			}
		}
	}
}

func TestWindowsSettingsBridgeSource_ProvidesExplicitAdapterHooks(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "settings.go"))
	if err != nil {
		t.Fatalf("read windows/settings.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`return bridgepkg.PermissionsSnapshot{Accessibility: true, InputMonitoring: true}`,
		`ListDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {`,
		`RefreshDevices: func(context.Context) ([]bridgepkg.DeviceSnapshot, error) {`,
		`prefsCtx := currentPreferencesContext()`,
		`preferences context unavailable after opening setup`,
		`func openPreferences() error {`,
		`decorateWebView2UnavailableMessage(err)`,
		`showWindowsMessageBox("JoiceTyper Preferences unavailable", message)`,
		`return fmt.Errorf("failed to start the Windows preferences host: %w", err)`,
		`DownloadModel: func(ctx context.Context, size string) error {`,
		`DeleteModel: func(ctx context.Context, size string) error {`,
		`UseModel: func(ctx context.Context, size string) error {`,
		`StartHotkeyCapture: func(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {`,
		`ConfirmHotkeyCapture: func(context.Context) (bridgepkg.HotkeyCaptureSnapshot, error) {`,
		`transcriptionpkg.DownloadModelWithProgress`,
		`publishModelDownloadProgress(size, progress, downloaded, total)`,
		`"Cannot delete the active model"`,
		`publishModelChanged(snapshot)`,
		`return webSettingsStartHotkeyCapture()`,
		`return webSettingsConfirmHotkeyCapture()`,
		`audiopkg.ListInputDeviceSnapshots`,
		`publishDevicesChanged(devices)`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/settings.go to contain %q", required)
		}
	}
}

func TestWindowsHotkeyCaptureSource_UsesDedicatedLowLevelHook(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "hotkey_capture.go"))
	if err != nil {
		t.Fatalf("read windows/hotkey_capture.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`windowsHotkeyCaptureCallback = windows.NewCallback(windowsLowLevelHotkeyCaptureProc)`,
		`procSetWindowsHookExW.Call(`,
		`publishHotkeyCaptureChanged(snapshot)`,
		`bridgepkg.ErrorCodeHotkeyCaptureStartFailed`,
		`bridgepkg.ErrorCodeHotkeyCaptureConfirmFailed`,
		`bridgepkg.ErrorCodeHotkeyCaptureCancelFailed`,
		`stopWindowsHotkeyCaptureListener()`,
		`windowsModifierPressed(pressed, vkMenu, vkLMenu, vkRMenu)`,
		`windowsModifierPressed(pressed, vkLWin, vkRWin)`,
		`return "option"`,
		`return "cmd"`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/hotkey_capture.go to contain %q", required)
		}
	}
}

func TestWindowsAudioDevicesSource_UsesCoreAudioEnumeration(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "audio_devices.go"))
	if err != nil {
		t.Fatalf("read windows/audio_devices.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`github.com/go-ole/go-ole`,
		`github.com/moutend/go-wca/pkg/wca`,
		`ole.CoInitializeEx`,
		`wca.CLSID_MMDeviceEnumerator`,
		`enumerator.EnumAudioEndpoints(wca.ECapture, wca.DEVICE_STATE_ACTIVE`,
		`wca.PKEY_Device_FriendlyName`,
		`bridgepkg.DeviceSnapshot`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/audio_devices.go to contain %q", required)
		}
	}
}

func TestWindowsTraySource_UsesShellNotifyIconAndMenuActions(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "tray.go"))
	if err != nil {
		t.Fatalf("read windows/tray.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`shell32.NewProc("Shell_NotifyIconW")`,
		`procCreatePopupMenu`,
		`procTrackPopupMenu`,
		`go OpenPreferences()`,
		`go RequestQuit()`,
		`publishRuntimeStateChanged(state)`,
		`wmPowerBroadcast`,
		`dispatchPowerEvent(PowerEventWake)`,
		`SetStatusBarHotkeyText`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/tray.go to contain %q", required)
		}
	}
}

func TestWindowsPowerSource_RefreshesDevicesAfterWake(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "power.go"))
	if err != nil {
		t.Fatalf("read windows/power.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`rec.MarkStale("system_sleep")`,
		`rec.RefreshDevices()`,
		`publishDevicesChanged(devices)`,
		`UpdateStatusBar(StateReady)`,
		`sharedWindowsTrayHost.ensureStarted()`,
		`failed to initialize windows power observer`,
		`dispatchPowerEvent(event PowerEvent)`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/power.go to contain %q", required)
		}
	}
	if strings.Contains(source, `handler := powerHandler`) && strings.Contains(source, `handler(event)`) && !strings.Contains(source, `dispatchPowerEvent`) {
		t.Fatalf("unexpected direct recursive power handler pattern in windows/power.go")
	}
}

func TestWindowsPasterSource_UsesClipboardAndTypingFallback(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "paster.go"))
	if err != nil {
		t.Fatalf("read windows/paster.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`procOpenClipboard`,
		`procSetClipboardData`,
		`procEnumClipboardFormats`,
		`procGlobalSize`,
		`procSendInput`,
		`clipboard paste failed, falling back to unicode typing`,
		`paste shortcut failed, falling back to unicode typing`,
		`keyeventfUnicode`,
		`type windowsClipboardSnapshot struct`,
		`type windowsClipboardEntry struct`,
		`readWindowsClipboardSnapshot()`,
		`restoreWindowsClipboardSnapshot(snapshot windowsClipboardSnapshot)`,
		`isWindowsCloneableClipboardFormat`,
		`procEnumClipboardFormats.Call(0)`,
		`procEnumClipboardFormats.Call(format)`,
		`restore clipboard failed after paste`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/paster.go to contain %q", required)
		}
	}
}

func TestWindowsInstallerSource_HandlesWebView2RuntimePrerequisite(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "packaging", "windows", "joicetyper.iss"))
	if err != nil {
		t.Fatalf("read packaging/windows/joicetyper.iss: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`MicrosoftEdgeWebview2Setup.exe`,
		`{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
		`/silent /install`,
		`function HasWebView2Runtime(): Boolean;`,
		`function HasWebView2Bootstrapper(): Boolean;`,
		`function InstallWebView2Runtime(): Boolean;`,
		`Exec(ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe')`,
		`WebView2 Runtime installation did not complete successfully.`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected packaging/windows/joicetyper.iss to contain %q", required)
		}
	}
}

func TestWindowsRuntimeStateSource_ClearsPendingHotkeyEvents(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "runtime_state.go"))
	if err != nil {
		t.Fatalf("read windows/runtime_state.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{`listener.clearPendingEvents()`} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/runtime_state.go to contain %q", required)
		}
	}

	hotkeyData, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "hotkey.go"))
	if err != nil {
		t.Fatalf("read windows/hotkey.go: %v", err)
	}
	hotkeySource := string(hotkeyData)
	for _, required := range []string{
		`func (h *hotkeyListener) clearPendingEvents() {`,
		`case <-events:`,
		`default:`,
	} {
		if !strings.Contains(hotkeySource, required) {
			t.Fatalf("expected windows/hotkey.go to contain %q", required)
		}
	}
}

func TestWindowsNotificationSource_UsesToastSpawner(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "notification.go"))
	if err != nil {
		t.Fatalf("read windows/notification.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`showWindowsTrayNotification`,
		`failed to show windows notification`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/notification.go to contain %q", required)
		}
	}
	for _, forbidden := range []string{
		`powershell`,
		`ToastNotificationManager`,
		`CreateToastNotifier('JoiceTyper')`,
		`CombinedOutput()`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("expected windows/notification.go not to contain %q", forbidden)
		}
	}

	trayData, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "tray.go"))
	if err != nil {
		t.Fatalf("read windows/tray.go: %v", err)
	}
	traySource := string(trayData)
	for _, required := range []string{
		`showNotification(title, body string)`,
		`nifInfo`,
		`niifInfo`,
		`SzInfo`,
		`SzInfoTitle`,
	} {
		if !strings.Contains(traySource, required) {
			t.Fatalf("expected windows/tray.go to contain %q", required)
		}
	}
}

func TestWindowsHotkeySource_UsesLowLevelKeyboardHook(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "windows", "hotkey.go"))
	if err != nil {
		t.Fatalf("read windows/hotkey.go: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`SetWindowsHookExW`,
		`UnhookWindowsHookEx`,
		`CallNextHookEx`,
		`PostThreadMessageW`,
		`windowsLowLevelKeyboardProc`,
		`TriggerPressed`,
		`TriggerReleased`,
		`modifier != "option"`,
		`modifier != "cmd"`,
		`windowsModifierPressed(h.pressed, vkMenu, vkLMenu, vkRMenu)`,
		`windowsModifierPressed(h.pressed, vkLWin, vkRWin)`,
		`display = append(display, "Alt")`,
		`display = append(display, "Win")`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected windows/hotkey.go to contain %q", required)
		}
	}
}

func TestDarwinWebviewTransportSource_LogsWindowLifecycle(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "darwin", "webview_darwin.m"))
	if err != nil {
		t.Fatalf("read webview_darwin.m: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		`WKNavigationDelegate`,
		`joicetyperConsole`,
		`WKUserScriptInjectionTimeAtDocumentStart`,
		`window.addEventListener('error'`,
		`window.addEventListener('unhandledrejection'`,
		`console.error =`,
		`window.dispatchEvent(new CustomEvent('`,
		`dispatch_error:`,
		`didFinishNavigation`,
		`didFailNavigation`,
		`didFailProvisionalNavigation`,
		`show web settings window requested`,
		`created web settings window`,
		`loading embedded web settings html`,
		`web settings navigation finished`,
		`web settings DOM snapshot`,
		`failed provisional web settings navigation`,
		`failed web settings navigation`,
		`web settings window visible`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected webview_darwin.m to contain %q", required)
		}
	}
	if strings.Contains(source, `window.__JOICETYPER_NATIVE_BRIDGE_DISPATCH__`) {
		t.Fatal("expected native transport to stop depending on a page-owned bridge dispatch function")
	}
}

func TestDarwinWebviewTransportSource_CopiesHTMLBeforeAsyncWindowShow(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "darwin", "webview_darwin.m"))
	if err != nil {
		t.Fatalf("read webview_darwin.m: %v", err)
	}
	source := string(data)
	dispatchIndex := strings.Index(source, "dispatch_async(dispatch_get_main_queue(), ^{")
	if dispatchIndex == -1 {
		t.Fatal("expected dispatch_async web settings window block")
	}
	beforeDispatch := source[:dispatchIndex]
	insideDispatch := source[dispatchIndex:]
	if !strings.Contains(beforeDispatch, `NSString *html = nil;`) ||
		!strings.Contains(beforeDispatch, `html = [NSString stringWithUTF8String:htmlContent];`) {
		t.Fatal("expected htmlContent to be copied into NSString before dispatch_async for window show")
	}
	if strings.Contains(insideDispatch, `NSString *html = [NSString stringWithUTF8String:htmlContent];`) {
		t.Fatal("expected async window block not to decode htmlContent directly after Go frees the C string")
	}
}

func TestDarwinWebviewTransportSource_ClearsSingletonsOnWindowClose(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "darwin", "webview_darwin.m"))
	if err != nil {
		t.Fatalf("read webview_darwin.m: %v", err)
	}
	source := string(data)
	closeIndex := strings.Index(source, "- (void)windowWillClose:")
	if closeIndex == -1 {
		t.Fatal("expected windowWillClose handler in webview_darwin.m")
	}
	closeSlice := source[closeIndex:]
	for _, required := range []string{
		`sWebSettingsView = nil;`,
		`sWebSettingsNavigationDelegate = nil;`,
		`sWebSettingsWindowDelegate = nil;`,
		`sWebSettingsWindow = nil;`,
	} {
		if !strings.Contains(closeSlice, required) {
			t.Fatalf("expected window close path to contain %q", required)
		}
	}
}

func TestDarwinWebviewTransportSource_UsesExplicitCloseFlagFromBridge(t *testing.T) {
	root := repoRoot(t)
	header, err := os.ReadFile(filepath.Join(root, "internal", "platform", "darwin", "webview_darwin.h"))
	if err != nil {
		t.Fatalf("read webview_darwin.h: %v", err)
	}
	if !strings.Contains(string(header), "char *handleWebSettingsMessage(char *messageJSON, int *closeWindow);") {
		t.Fatal("expected handleWebSettingsMessage to expose an explicit closeWindow out-param")
	}

	data, err := os.ReadFile(filepath.Join(root, "internal", "platform", "darwin", "webview_darwin.m"))
	if err != nil {
		t.Fatalf("read webview_darwin.m: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		"int closeWindow = 0;",
		"char *response = handleWebSettingsMessage(request, &closeWindow);",
		"dispatchBridgeEnvelopeJSON(responseJSON, closeWindow == 1);",
		"if (closeWindow && sWebSettingsWindow != nil) {",
		"[sWebSettingsWindow close];",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected webview_darwin.m to contain %q", required)
		}
	}
	if strings.Contains(source, `[(NSNumber *)result boolValue]`) {
		t.Fatal("expected save-window close to stop depending on JavaScript return values")
	}
}

func TestSettingsScreenSource_DoesNotOverclaimPermissionOpenSuccess(t *testing.T) {
	source := readSettingsSourceTree(t)
	for _, forbidden := range []string{
		`Opened ${label} settings.`,
		`Opened Accessibility settings.`,
		`Opened Input Monitoring settings.`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("expected settings sources not to overclaim permission-open success with %q", forbidden)
		}
	}
	for _, required := range []string{
		`Requested ${label} settings.`,
		`Opening ${label} settings...`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected settings sources to contain %q", required)
		}
	}
}

func TestSettingsScreenSource_SeparatesConfigTargetFromActiveModelState(t *testing.T) {
	source := readSettingsSourceTree(t)
	for _, required := range []string{
		`Config target:`,
		`Active session model:`,
		`Cached for active model:`,
		`Active model path`,
		`Save to keep it.`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected settings sources to contain %q", required)
		}
	}
	for _, forbidden := range []string{
		`Action target:`,
		`Cached snapshot:`,
		`Selected ${size} model. Save to apply it.`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("expected settings sources not to contain stale model semantics %q", forbidden)
		}
	}
}

func TestBridgeSource_UsesRequestScopedNativeSaveAck(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ui", "src", "bridge", "client.ts"))
	if err != nil {
		t.Fatalf("read bridge/client.ts: %v", err)
	}
	source := string(data)
	for _, snippet := range []string{
		`kind: KINDS.request`,
		`METHODS.configSave`,
		`BRIDGE_EVENT_NAME`,
		`kind === KINDS.response`,
		`BridgeRequestError`,
		`ERROR_CODES.internalError`,
		`window.setTimeout`,
		`window.clearTimeout`,
		`Native settings bridge timed out`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected bridge.ts to contain %q", snippet)
		}
	}
	for _, forbidden := range []string{
		`type: "saveConfig"`,
		`joicetyper-native-save`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("expected bridge.ts not to contain legacy snippet %q", forbidden)
		}
	}
}

func TestBridgeSource_ExposesRuntimeStateEventSubscription(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ui", "src", "bridge", "client.ts"))
	if err != nil {
		t.Fatalf("read bridge/client.ts: %v", err)
	}
	source := string(data)
	for _, snippet := range []string{
		`EVENTS.runtimeStateChanged`,
		`EVENTS.permissionsChanged`,
		`EVENTS.modelChanged`,
		`EVENTS.modelDownloadProgress`,
		`EVENTS.configSaved`,
		`EVENTS.devicesChanged`,
		`METHODS.permissionsOpenSettings`,
		`METHODS.devicesRefresh`,
		`METHODS.modelDownload`,
		`METHODS.modelDelete`,
		`METHODS.modelUse`,
		`METHODS.hotkeyCaptureStart`,
		`METHODS.hotkeyCaptureCancel`,
		`METHODS.hotkeyCaptureConfirm`,
		`METHODS.optionsGet`,
		`subscribeRuntimeStateChanged`,
		`subscribePermissionsChanged`,
		`subscribeModelChanged`,
		`subscribeModelDownloadProgress`,
		`subscribeConfigSaved`,
		`subscribeDevicesChanged`,
		`subscribeHotkeyCaptureChanged`,
		`openPermissionSettings`,
		`refreshDevices`,
		`downloadModel`,
		`deleteModel`,
		`useModel`,
		`startHotkeyCapture`,
		`cancelHotkeyCapture`,
		`confirmHotkeyCapture`,
		`fetchOptions`,
		`BRIDGE_EVENT_NAME`,
		`function query<`,
		`METHODS.configGet`,
		`METHODS.permissionsGet`,
		`METHODS.devicesList`,
		`METHODS.modelGet`,
		`METHODS.runtimeGet`,
		`EVENTS.hotkeyCaptureChanged`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected bridge/client.ts to contain %q", snippet)
		}
	}
}

func TestSettingsScreenSource_UsesRuntimeStateSubscription(t *testing.T) {
	source := readSettingsSourceTree(t)
	for _, snippet := range []string{
		`subscribeRuntimeStateChanged`,
		`subscribePermissionsChanged`,
		`subscribeModelChanged`,
		`subscribeModelDownloadProgress`,
		`subscribeConfigSaved`,
		`subscribeDevicesChanged`,
		`subscribeHotkeyCaptureChanged`,
		`permissionsPaneVisible`,
		`options.permissions`,
		`item.key`,
		`item.label`,
		`handleRefreshDevices`,
		`handleDownloadModel`,
		`handleDeleteModel`,
		`handleUseModel`,
		`handleStartHotkeyCapture`,
		`handleCancelHotkeyCapture`,
		`handleConfirmHotkeyCapture`,
		`modelActionSize`,
		`confirmDeleteModelSize`,
		`hotkeyCapture`,
		`downloadProgress.bytesDownloaded`,
		`downloadProgress.bytesTotal`,
		`model-download-progress`,
		`Downloading ${downloadProgress.size} model`,
		`useEffect`,
		`setCurrentAppState`,
		`refreshDevices()`,
		`setPermissions`,
		`setDevices`,
		`setModel`,
		`setOptions`,
		`setStatus`,
		`value={draft.modelSize}`,
		`value={draft.language}`,
		`value={draft.decodeMode}`,
		`value={draft.punctuationMode}`,
		`BridgeRequestError`,
		`handleOpenPermissionSettings`,
		`Refresh Devices`,
		`Download Model`,
		`Delete Model`,
		`Confirm Delete`,
		`Use Model`,
		`Change Hotkey`,
		`Cancel`,
		`Confirm Hotkey`,
		"Downloaded ${size} model.",
		`onDeleteModel(modelActionSize)`,
		`onUseModel(modelActionSize)`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected settings sources to contain %q", snippet)
		}
	}
	for _, forbidden := range []string{
		`<Field label="Model size">
              <input`,
		`<Field label="Language">
              <input`,
		`<Field label="Decode mode">
              <input`,
		`<Field label="Punctuation">
              <input`,
		`handleDeleteModel(draft.modelSize)`,
		`handleUseModel(draft.modelSize)`,
		"Started ${size} model download.",
		`value={draft.triggerKey.join(", ")}`,
		`.split(",")`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("expected settings sources not to contain legacy free-text field %q", forbidden)
		}
	}
}

func readSettingsSourceTree(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	settingsRoot := filepath.Join(root, "ui", "src", "settings")
	var builder strings.Builder
	err := filepath.WalkDir(settingsRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".ts" && filepath.Ext(path) != ".tsx" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		builder.Write(data)
		builder.WriteString("\n")
		return nil
	})
	if err != nil {
		t.Fatalf("read settings sources: %v", err)
	}
	return builder.String()
}

func TestFrontendSource_RespectsBridgeBoundary(t *testing.T) {
	root := repoRoot(t)
	uiRoot := filepath.Join(root, "ui", "src")
	forbidden := []string{
		"webkit?.messageHandlers",
		"joicetyper-bridge-message",
		"postMessage(",
	}

	err := filepath.WalkDir(uiRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path == filepath.Join(uiRoot, "bridge") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".ts" && filepath.Ext(path) != ".tsx" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		for _, snippet := range forbidden {
			if strings.Contains(source, snippet) {
				t.Fatalf("expected %s not to contain %q; bridge boundary violation", path, snippet)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ui source: %v", err)
	}
}
