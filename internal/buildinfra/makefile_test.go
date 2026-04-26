package buildinfra

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func makeCommand(root string, args ...string) *exec.Cmd {
	makeBin := "make"
	if _, err := exec.LookPath(makeBin); err != nil {
		fallback := `C:\Program Files (x86)\GnuWin32\bin\make.exe`
		if _, statErr := os.Stat(fallback); statErr == nil {
			makeBin = fallback
		}
	}
	cmd := exec.Command(makeBin, args...)
	cmd.Dir = root
	return cmd
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func currentVersion(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func TestMakeBuildTargetsUseExpectedVersionPolicy(t *testing.T) {
	root := repoRoot(t)

	macOut, err := makeCommand(root, "-n", "build").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build: %v\n%s", err, macOut)
	}
	if !strings.Contains(string(macOut), `printf '%s\n' "$next" > "VERSION"`) {
		t.Fatalf("expected macOS build target to bump VERSION before building\noutput:\n%s", macOut)
	}

	winOut, err := makeCommand(root, "-n", "build-windows-runtime-amd64").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build-windows-runtime-amd64: %v\n%s", err, winOut)
	}
	if strings.Contains(string(winOut), `printf '%s\n' "$next" > "VERSION"`) {
		t.Fatalf("expected Windows runtime build target not to bump VERSION during local dev builds\noutput:\n%s", winOut)
	}

	winReleaseOut, err := makeCommand(root, "-n", "build-windows-runtime-amd64-release").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build-windows-runtime-amd64-release: %v\n%s", err, winReleaseOut)
	}
	if !strings.Contains(string(winReleaseOut), `printf '%s\n' "$next" > "VERSION"`) {
		t.Fatalf("expected Windows runtime release target to bump VERSION\noutput:\n%s", winReleaseOut)
	}
}

func TestMakeDownloadModelUsesRuntimeModelDir(t *testing.T) {
	root := repoRoot(t)
	home := t.TempDir()

	cmd := makeCommand(root, "-n", "download-model")
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n download-model: %v\n%s", err, out)
	}

	want := filepath.Join(home, "Library", "Application Support", "JoiceTyper", "models", "ggml-small.bin")
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected make output to use runtime model path %q\noutput:\n%s", want, out)
	}
}

func TestMakeDownloadModelUsesXDGModelDirOnLinux(t *testing.T) {
	root := repoRoot(t)
	home := t.TempDir()
	xdgConfigHome := filepath.Join(home, ".config-alt")

	cmd := makeCommand(root, "-n", "download-model", "HOST_GOOS=linux")
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+xdgConfigHome)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n download-model HOST_GOOS=linux: %v\n%s", err, out)
	}

	want := filepath.Join(xdgConfigHome, "JoiceTyper", "models", "ggml-small.bin")
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected make output to use linux runtime model path %q\noutput:\n%s", want, out)
	}
}

func TestMakeDownloadModelSkipsExistingFile(t *testing.T) {
	root := repoRoot(t)
	home := t.TempDir()
	modelPath := filepath.Join(home, "Library", "Application Support", "JoiceTyper", "models", "ggml-small.bin")

	if err := os.MkdirAll(filepath.Dir(modelPath), 0755); err != nil {
		t.Fatalf("mkdir model dir: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("existing-model"), 0644); err != nil {
		t.Fatalf("write model file: %v", err)
	}

	cmd := makeCommand(root, "download-model", "CURL=false")
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected existing model to skip download, got error: %v\n%s", err, out)
	}
}

func TestMakeAppUsesConfiguredPortaudioPrefix(t *testing.T) {
	root := repoRoot(t)
	const portaudioPrefix = "/usr/local/opt/portaudio"

	cmd := makeCommand(root, "-n", "app", "PORTAUDIO_PREFIX="+portaudioPrefix)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n app: %v\n%s", err, out)
	}

	want := filepath.Join(portaudioPrefix, "lib", "libportaudio.2.dylib")
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected make output to use portaudio path %q\noutput:\n%s", want, out)
	}
}

func TestMakeAppUsesAssetPaths(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "app")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n app: %v\n%s", err, out)
	}

	text := string(out)
	if !strings.Contains(text, "assets/macos/Info.plist.tmpl") {
		t.Fatalf("expected app build to use assets/macos/Info.plist.tmpl\noutput:\n%s", text)
	}
	if !strings.Contains(text, "assets/icons/icon.icns") {
		t.Fatalf("expected app build to use assets/icons/icon.icns\noutput:\n%s", text)
	}
}

func TestMakefileHasMacReleaseTargets(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "mac-release-archive", "mac-appcast")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-release-archive mac-appcast: %v\n%s", err, out)
	}

	text := string(out)
	for _, required := range []string{
		`sh "scripts/release/macos_prepare_release_app.sh"`,
		`sh "scripts/release/macos_release_env.sh" archive`,
		"scripts/release/macos_prepare_release_app.sh",
		"scripts/release/macos_archive.sh",
		`sh "scripts/release/macos_release_env.sh" appcast`,
		"scripts/release/macos_appcast.py",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected mac release flow to contain %q\noutput:\n%s", required, text)
		}
	}
}

func TestMakeBuildRunsFrontendBuild(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "build")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build: %v\n%s", err, out)
	}

	text := string(out)
	if !strings.Contains(text, "cd ui && npm run build") {
		t.Fatalf("expected build output to include frontend build step\noutput:\n%s", text)
	}
}

func TestMacReleaseTargetsFailClosedWithoutCredentials(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "mac-release-archive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-release-archive: %v\n%s", err, out)
	}

	text := string(out)
	if !strings.Contains(text, `sh "scripts/release/macos_release_env.sh" archive`) {
		t.Fatalf("expected mac release archive to validate release credentials explicitly\noutput:\n%s", text)
	}
	if strings.Contains(text, "scripts/release/macos_release_env.sh dev") {
		t.Fatalf("did not expect dev app path to require release credential validation\noutput:\n%s", text)
	}
}

func TestMacReleaseArchiveTargetUsesVersionedArtifactNames(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "mac-release-archive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-release-archive: %v\n%s", err, out)
	}

	text := string(out)
	for _, required := range []string{
		"build/macos-release/JoiceTyper-",
		"-macos.zip",
		"build/macos-release/JoiceTyper.app",
		"macos_prepare_release_app.sh",
		"macos_archive.sh",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected mac release archive flow to contain %q\noutput:\n%s", required, text)
		}
	}
}

func TestMacReleasePathStagesSparkleSeparatelyFromDevBuilds(t *testing.T) {
	root := repoRoot(t)

	releaseOut, err := makeCommand(root, "-n", "mac-stage-sparkle").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-stage-sparkle: %v\n%s", err, releaseOut)
	}
	if !strings.Contains(string(releaseOut), "scripts/release/macos_stage_sparkle.sh") {
		t.Fatalf("expected mac release path to stage Sparkle explicitly\noutput:\n%s", releaseOut)
	}

	devOut, err := makeCommand(root, "-n", "app").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n app: %v\n%s", err, devOut)
	}
	if strings.Contains(string(devOut), "scripts/release/macos_stage_sparkle.sh") {
		t.Fatalf("expected normal dev app path not to stage Sparkle\noutput:\n%s", devOut)
	}
}

func TestMacReleaseTargetsProduceGitHubReleaseFriendlyOutputs(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "mac-release-artifacts")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-release-artifacts: %v\n%s", err, out)
	}

	text := string(out)
	for _, required := range []string{
		"build/macos-release/JoiceTyper.app",
		"build/macos-release/JoiceTyper-",
		"-macos.zip",
		"appcast.xml",
		"build/macos-release/JoiceTyper-",
		".dmg",
		"macos_notarize.sh",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected mac release artifacts flow to contain %q\noutput:\n%s", required, text)
		}
	}
	if strings.Contains(text, "Version bumped:") {
		t.Fatalf("expected mac release artifacts flow not to bump VERSION\noutput:\n%s", text)
	}

	notarizeIndex := strings.Index(text, "macos_notarize.sh")
	appcastIndex := strings.Index(text, "macos_appcast.py")
	if notarizeIndex == -1 || appcastIndex == -1 || notarizeIndex > appcastIndex {
		t.Fatalf("expected notarization to happen before appcast generation\noutput:\n%s", text)
	}
}

func TestMacReleaseDMGDependsOnNotarizedReleaseApp(t *testing.T) {
	root := repoRoot(t)

	data, err := os.ReadFile(filepath.Join(root, "scripts", "make", "macos.mk"))
	if err != nil {
		t.Fatalf("read scripts/make/macos.mk: %v", err)
	}
	source := string(data)
	if !strings.Contains(source, "mac-release-dmg: mac-notarize-release") {
		t.Fatalf("expected mac-release-dmg to depend on mac-notarize-release so it cannot package an unstapled app\nsource:\n%s", source)
	}

	cmd := makeCommand(root, "-n", "mac-release-dmg")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-release-dmg: %v\n%s", err, out)
	}
	text := string(out)
	notarizeIndex := strings.Index(text, "macos_notarize.sh")
	dmgIndex := strings.Index(text, "hdiutil create")
	if notarizeIndex == -1 || dmgIndex == -1 || notarizeIndex > dmgIndex {
		t.Fatalf("expected notarization to happen before DMG creation\noutput:\n%s", text)
	}
	dmgNotarizeIndex := strings.LastIndex(text, "macos_notarize.sh")
	if dmgNotarizeIndex == notarizeIndex || dmgNotarizeIndex < dmgIndex {
		t.Fatalf("expected DMG artifact to be notarized after DMG creation\noutput:\n%s", text)
	}
	if !strings.Contains(text, `codesign --force --sign "${MACOS_CODESIGN_IDENTITY}" --timestamp "build/macos-release/JoiceTyper-`) {
		t.Fatalf("expected DMG artifact to be signed before notarization\noutput:\n%s", text)
	}
}

func TestMacNotarizeScriptSubmitsZipWhenGivenAppBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script execution uses POSIX paths")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	appPath := filepath.Join(tmp, "JoiceTyper.app")
	if err := os.MkdirAll(filepath.Join(appPath, "Contents", "MacOS"), 0755); err != nil {
		t.Fatalf("create app bundle: %v", err)
	}

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}
	logPath := filepath.Join(tmp, "xcrun.log")
	fakeXcrun := filepath.Join(binDir, "xcrun")
	fakeScript := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"" + logPath + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(fakeXcrun, []byte(fakeScript), 0755); err != nil {
		t.Fatalf("write fake xcrun: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "release", "macos_notarize.sh"), appPath, "test-profile")
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("macos_notarize.sh failed: %v\n%s", err, out)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake xcrun log: %v", err)
	}
	logText := string(logData)
	if strings.Contains(logText, "notarytool submit "+appPath+" ") {
		t.Fatalf("expected script not to submit raw .app bundle\nxcrun log:\n%s", logText)
	}
	if !strings.Contains(logText, "notarytool submit ") || !strings.Contains(logText, ".zip --keychain-profile test-profile --wait") {
		t.Fatalf("expected script to submit a temporary zip for app notarization\nxcrun log:\n%s", logText)
	}
	if !strings.Contains(logText, "stapler staple "+appPath) {
		t.Fatalf("expected script to staple the original app bundle\nxcrun log:\n%s", logText)
	}
}

func TestMacReleaseScriptsRequireSparkleDownloadChecksum(t *testing.T) {
	root := repoRoot(t)

	stageScript, err := os.ReadFile(filepath.Join(root, "scripts", "release", "macos_stage_sparkle.sh"))
	if err != nil {
		t.Fatalf("read macos_stage_sparkle.sh: %v", err)
	}
	stageSource := string(stageScript)
	for _, required := range []string{
		"MACOS_SPARKLE_DOWNLOAD_SHA256",
		"shasum -a 256",
		"fatal: set MACOS_SPARKLE_DOWNLOAD_SHA256",
	} {
		if !strings.Contains(stageSource, required) {
			t.Fatalf("expected Sparkle staging script to contain %q\nsource:\n%s", required, stageSource)
		}
	}

	for _, path := range []string{
		filepath.Join(root, "packaging", "macos", "release.env.example"),
		filepath.Join(root, ".github", "workflows", "macos-release.yml"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(data), "MACOS_SPARKLE_DOWNLOAD_SHA256") {
			t.Fatalf("expected %s to configure MACOS_SPARKLE_DOWNLOAD_SHA256", path)
		}
	}
}

func TestMacMetadataRenderersEscapeXMLValues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script execution uses POSIX paths")
	}
	root := repoRoot(t)
	tmp := t.TempDir()

	metadataPath := filepath.Join(tmp, "metadata.env")
	if err := os.WriteFile(metadataPath, []byte(strings.Join([]string{
		"VERSION=1.2.3",
		"ARCHIVE_LENGTH=123",
		"PUBLICATION_DATE=Thu, 23 Apr 2026 00:00:00 +0000",
		"EDDSA_SIGNATURE=sig+with/slash=",
	}, "\n")), 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	appcastPath := filepath.Join(tmp, "appcast.xml")
	cmd := exec.Command(
		"python3",
		filepath.Join(root, "scripts", "release", "macos_appcast.py"),
		filepath.Join(root, "packaging", "macos", "sparkle-appcast.xml.tmpl"),
		appcastPath,
		"Joice & Typer",
		"https://example.invalid/appcast.xml?channel=stable&arch=arm64",
		"https://example.invalid/download.zip?x=1&y=2",
		"public&key",
		metadataPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("macos_appcast.py failed: %v\n%s", err, out)
	}
	appcastData, err := os.ReadFile(appcastPath)
	if err != nil {
		t.Fatalf("read appcast: %v", err)
	}
	if err := xml.Unmarshal(appcastData, new(any)); err != nil {
		t.Fatalf("expected appcast XML to parse after escaping values: %v\n%s", err, appcastData)
	}
	if !strings.Contains(string(appcastData), "x=1&amp;y=2") {
		t.Fatalf("expected appcast URL attributes to be XML escaped\n%s", appcastData)
	}

	plistPath := filepath.Join(tmp, "Info.plist")
	cmd = exec.Command(
		"python3",
		filepath.Join(root, "scripts", "release", "macos_render_info_plist.py"),
		filepath.Join(root, "assets", "macos", "Info.plist.tmpl"),
		plistPath,
		"1.2.3",
		"https://example.invalid/appcast.xml?channel=stable&arch=arm64",
		"public&key",
		"true",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("macos_render_info_plist.py failed: %v\n%s", err, out)
	}
	plistData, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	if err := xml.Unmarshal(plistData, new(any)); err != nil {
		t.Fatalf("expected plist XML to parse after escaping values: %v\n%s", err, plistData)
	}
	if !strings.Contains(string(plistData), "channel=stable&amp;arch=arm64") {
		t.Fatalf("expected plist string values to be XML escaped\n%s", plistData)
	}
}

func TestMacPublishGitHubReleaseTargetUsesReleaseCheckAndGHUpload(t *testing.T) {
	root := repoRoot(t)
	version := currentVersion(t)
	releaseTag := "v" + version

	cmd := makeCommand(root, "-n", "mac-publish-github-release", "RELEASE_TAG="+releaseTag)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-publish-github-release: %v\n%s", err, out)
	}

	text := string(out)
	for _, required := range []string{
		`Release tag ` + releaseTag + ` matches VERSION ` + version,
		"scripts/release/macos_publish_github.sh",
		`RELEASE_TAG="` + releaseTag + `"`,
		"JoiceTyper-" + version + "-macos.zip",
		"JoiceTyper-" + version + ".dmg",
		"appcast.xml",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected mac GitHub publish flow to contain %q\noutput:\n%s", required, text)
		}
	}
}

func TestMacPreflightTargetsUseValidationScripts(t *testing.T) {
	root := repoRoot(t)
	version := currentVersion(t)
	releaseTag := "v" + version

	cmd := makeCommand(root, "-n", "mac-release-preflight", "mac-notarize-preflight", "mac-publish-preflight", "RELEASE_TAG="+releaseTag)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-release-preflight mac-notarize-preflight mac-publish-preflight: %v\n%s", err, out)
	}

	text := string(out)
	for _, required := range []string{
		"scripts/release/macos_preflight.sh",
		`archive`,
		`notarize`,
		`publish`,
		`RELEASE_TAG="` + releaseTag + `"`,
		`Release tag ` + releaseTag + ` matches VERSION ` + version,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected mac preflight flow to contain %q\noutput:\n%s", required, text)
		}
	}
}

func TestMacPublishPreflightDoesNotRequireExistingRelease(t *testing.T) {
	root := repoRoot(t)

	data, err := os.ReadFile(filepath.Join(root, "scripts", "release", "macos_preflight.sh"))
	if err != nil {
		t.Fatalf("read macos_preflight.sh: %v", err)
	}

	source := string(data)
	if strings.Contains(source, `gh release view "$RELEASE_TAG"`) {
		t.Fatalf("expected publish preflight not to require an existing GitHub release\nsource:\n%s", source)
	}
	for _, required := range []string{
		`gh auth status`,
		`require_var GITHUB_REPOSITORY`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("expected publish preflight to contain %q\nsource:\n%s", required, source)
		}
	}
}

func TestMacDevUpdateArtifactsTargetProducesUnsignedLocalOutputs(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "mac-dev-update-artifacts")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n mac-dev-update-artifacts: %v\n%s", err, out)
	}

	text := string(out)
	for _, required := range []string{
		"scripts/release/macos_archive_dev.sh",
		"scripts/release/macos_appcast.py",
		"build/macos-dryrun-update/JoiceTyper-",
		"-macos.zip",
		"build/macos-dryrun-update/appcast.xml",
		"https://example.invalid/joicetyper/appcast.xml",
		"DEV_ONLY_UNSIGNED",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected mac dry-run update flow to contain %q\noutput:\n%s", required, text)
		}
	}
	if strings.Contains(text, "Version bumped:") {
		t.Fatalf("expected mac dry-run update flow not to bump VERSION\noutput:\n%s", text)
	}
}

func TestMakeWindowsBuildRunsFrontendBuild(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "build-windows-amd64")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build-windows-amd64: %v\n%s", err, out)
	}

	text := string(out)
	if !strings.Contains(text, "cd ui && npm run build") {
		t.Fatalf("expected windows build to include frontend build\noutput:\n%s", text)
	}
	if !strings.Contains(text, "-H=windowsgui") {
		t.Fatalf("expected windows build to use the Windows GUI subsystem\noutput:\n%s", text)
	}
	if !strings.Contains(text, "--subsystem,windows") {
		t.Fatalf("expected windows build to force the Windows GUI subsystem at external link time\noutput:\n%s", text)
	}
}

func TestMakePackageWindowsUsesInstallerScript(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "package-windows")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n package-windows: %v\n%s", err, out)
	}

	text := string(out)
	if !strings.Contains(text, "packaging/windows/joicetyper.iss") {
		t.Fatalf("expected windows packaging to use packaging/windows/joicetyper.iss\noutput:\n%s", text)
	}
	if strings.Contains(text, "Version bumped:") {
		t.Fatalf("expected windows packaging not to bump VERSION during local dev packaging\noutput:\n%s", text)
	}
	if strings.Contains(text, "go build -ldflags") || strings.Contains(text, "CGO_ENABLED=1") {
		t.Fatalf("expected windows packaging to consume staged runtime artifacts instead of forcing a rebuild\noutput:\n%s", text)
	}
	if !strings.Contains(text, "fatal: missing staged Windows runtime artifact") {
		t.Fatalf("expected windows packaging to validate staged runtime artifacts\noutput:\n%s", text)
	}
	if !strings.Contains(text, "-AppVersion") {
		t.Fatalf("expected windows packaging to pass version into installer script\noutput:\n%s", text)
	}
}

func TestMakeWindowsPreflightAdvertisesSupportedToolchain(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "windows-preflight")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n windows-preflight: %v\n%s", err, out)
	}

	text := string(out)
	for _, required := range []string{
		"checking supported Win11 build environment...",
		"fatal: missing Windows C compiler",
		"fatal: missing Windows C++ compiler",
		"fatal: missing cmake",
		"fatal: missing mingw32-make.exe",
		"fatal: missing pkg-config or pkgconf",
		"fatal: missing Vulkan SDK root under /c/VulkanSDK",
		"fatal: missing Windows PortAudio source directory",
		"fatal: missing Inno Setup compiler (ISCC.exe)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected windows preflight to contain %q\noutput:\n%s", required, text)
		}
	}
}

func TestMakeWindowsReleasePackagingUsesReleasePolicy(t *testing.T) {
	root := repoRoot(t)

	cmd := makeCommand(root, "-n", "package-windows-release")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n package-windows-release: %v\n%s", err, out)
	}

	text := string(out)
	if !strings.Contains(text, `printf '%s\n' "$next" > "VERSION"`) {
		t.Fatalf("expected windows release packaging to bump VERSION\noutput:\n%s", text)
	}
	if !strings.Contains(text, "Release tag") && !strings.Contains(text, "release tag") {
		t.Fatalf("expected windows release packaging to run release-check\noutput:\n%s", text)
	}
	if !strings.Contains(text, "packaging/windows/joicetyper.iss") {
		t.Fatalf("expected windows release packaging to use installer script\noutput:\n%s", text)
	}
}

func TestMakeBuildSkipsFrontendInstallWhenStampPresent(t *testing.T) {
	root := repoRoot(t)
	stampPath := filepath.Join(root, "ui", "node_modules", ".package-lock.stamp")
	lockPath := filepath.Join(root, "ui", "package-lock.json")
	requiredPaths := []string{
		filepath.Join(root, "ui", "node_modules", ".bin", "vite"),
		filepath.Join(root, "ui", "node_modules", "react", "package.json"),
		filepath.Join(root, "ui", "node_modules", "react-dom", "package.json"),
		filepath.Join(root, "ui", "node_modules", "typescript", "package.json"),
	}

	lockInfo, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat package-lock.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(stampPath), 0755); err != nil {
		t.Fatalf("mkdir stamp dir: %v", err)
	}
	for _, path := range requiredPaths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("ok"), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		defer os.Remove(path)
	}
	if err := os.WriteFile(stampPath, []byte("ok"), 0644); err != nil {
		t.Fatalf("write stamp: %v", err)
	}
	newer := lockInfo.ModTime().Add(time.Hour)
	if err := os.Chtimes(stampPath, newer, newer); err != nil {
		t.Fatalf("chtimes stamp: %v", err)
	}
	defer os.Remove(stampPath)

	cmd := makeCommand(root, "-n", "build")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build: %v\n%s", err, out)
	}

	if strings.Contains(string(out), "cd ui && npm ci") {
		t.Fatalf("expected build to skip npm ci when install stamp is current\noutput:\n%s", out)
	}
}

func TestMakeBuildReinstallsFrontendWhenViteBinaryMissing(t *testing.T) {
	root := repoRoot(t)
	stampPath := filepath.Join(root, "ui", "node_modules", ".package-lock.stamp")
	lockPath := filepath.Join(root, "ui", "package-lock.json")
	viteBinPath := filepath.Join(root, "ui", "node_modules", ".bin", "vite")

	lockInfo, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat package-lock.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(stampPath), 0755); err != nil {
		t.Fatalf("mkdir stamp dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(viteBinPath), 0755); err != nil {
		t.Fatalf("mkdir vite bin dir: %v", err)
	}
	if err := os.Remove(viteBinPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove vite binary: %v", err)
	}
	if err := os.WriteFile(stampPath, []byte("ok"), 0644); err != nil {
		t.Fatalf("write stamp: %v", err)
	}
	newer := lockInfo.ModTime().Add(time.Hour)
	if err := os.Chtimes(stampPath, newer, newer); err != nil {
		t.Fatalf("chtimes stamp: %v", err)
	}
	defer os.Remove(stampPath)

	cmd := makeCommand(root, "-n", "build")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "npm ci") {
		t.Fatalf("expected build to reinstall frontend deps when vite binary is missing\noutput:\n%s", out)
	}
}
