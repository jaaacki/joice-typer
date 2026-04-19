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
		`failed to encode bridge payload string literal`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected webview_darwin.m to contain %q", required)
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
		`"accessibility", "Accessibility"`,
		`"input_monitoring", "Input Monitoring"`,
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
