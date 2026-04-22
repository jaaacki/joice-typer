package buildinfra

import (
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

func TestMakeBuildTargetsBumpVersion(t *testing.T) {
	root := repoRoot(t)

	macOut, err := makeCommand(root, "-n", "build").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build: %v\n%s", err, macOut)
	}
	if !strings.Contains(string(macOut), "version-bump") {
		t.Fatalf("expected macOS build target to invoke version-bump\noutput:\n%s", macOut)
	}

	winOut, err := makeCommand(root, "-n", "build-windows-runtime-amd64").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build-windows-runtime-amd64: %v\n%s", err, winOut)
	}
	if !strings.Contains(string(winOut), "version-bump") {
		t.Fatalf("expected Windows runtime build target to invoke version-bump\noutput:\n%s", winOut)
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
	if !strings.Contains(text, "CGO_ENABLED=1") || !strings.Contains(text, "go build -ldflags") || !strings.Contains(text, "windows-runtime-stage-check") {
		t.Fatalf("expected windows packaging to use the runtime windows build path\noutput:\n%s", text)
	}
	if !strings.Contains(text, "PKG_CONFIG_LIBDIR=") || !strings.Contains(text, "portaudio-windows-static-install/lib/pkgconfig/portaudio-2.0.pc") {
		t.Fatalf("expected windows packaging to build against static Windows PortAudio metadata\noutput:\n%s", text)
	}
	if !strings.Contains(text, "-H=windowsgui") {
		t.Fatalf("expected windows packaging to use the Windows GUI subsystem\noutput:\n%s", text)
	}
	if !strings.Contains(text, "--subsystem,windows") {
		t.Fatalf("expected windows packaging to force the Windows GUI subsystem at external link time\noutput:\n%s", text)
	}
	if !strings.Contains(text, "/DAppVersion=") {
		t.Fatalf("expected windows packaging to pass version into installer\noutput:\n%s", text)
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
