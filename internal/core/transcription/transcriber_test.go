//go:build darwin || (windows && cgo)

package transcription

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeConfigForMode_Greedy(t *testing.T) {
	cfg := decodeConfigForMode("greedy")
	if cfg.strategy != "greedy" {
		t.Fatalf("expected greedy strategy, got %q", cfg.strategy)
	}
	if cfg.beamSize != 0 {
		t.Fatalf("expected beam size 0 for greedy, got %d", cfg.beamSize)
	}
}

func TestDecodeConfigForMode_Beam(t *testing.T) {
	cfg := decodeConfigForMode("beam")
	if cfg.strategy != "beam" {
		t.Fatalf("expected beam strategy, got %q", cfg.strategy)
	}
	if cfg.beamSize != whisperBeamSize {
		t.Fatalf("expected beam size %d, got %d", whisperBeamSize, cfg.beamSize)
	}
}

func TestAudioRMS(t *testing.T) {
	if got := audioRMS([]float32{0, 0, 0}); got != 0 {
		t.Fatalf("expected zero RMS for silence, got %v", got)
	}
	if got := audioRMS([]float32{1, -1}); got != 1 {
		t.Fatalf("expected RMS 1, got %v", got)
	}
}

func TestEffectiveDecodeConfig_ForcesShortTranscriptionToGreedy(t *testing.T) {
	cfg := effectiveDecodeConfig("beam", false, "transcription")
	if cfg.strategy != "greedy" {
		t.Fatalf("expected short transcription to force greedy, got %q", cfg.strategy)
	}
}

func TestEffectiveDecodeConfig_PreservesLongTranscriptionMode(t *testing.T) {
	cfg := effectiveDecodeConfig("beam", true, "transcription")
	if cfg.strategy != "beam" {
		t.Fatalf("expected long transcription to preserve beam, got %q", cfg.strategy)
	}
}

func TestShortFormTokenLimit_UsesFullDuration(t *testing.T) {
	if got := shortFormTokenLimit(0.9); got != 5 {
		t.Fatalf("expected 0.9s clip to allow 5 tokens, got %d", got)
	}
	if got := shortFormTokenLimit(1.3); got != 6 {
		t.Fatalf("expected 1.3s clip to allow 6 tokens, got %d", got)
	}
	if got := shortFormTokenLimit(4); got != shortFormMaxTokens {
		t.Fatalf("expected token limit to cap at %d, got %d", shortFormMaxTokens, got)
	}
}

func TestRuntimeDecodeModeForOutputMode_Translation(t *testing.T) {
	mode := runtimeDecodeModeForOutputMode("translation")
	if !mode.applySilenceGate {
		t.Fatal("expected translation mode to apply silence gate")
	}
	if !mode.logAudioRMS {
		t.Fatal("expected translation mode to log RMS")
	}
}

func TestRuntimeDecodeModeForOutputMode_Transcription(t *testing.T) {
	mode := runtimeDecodeModeForOutputMode("transcription")
	if mode.applySilenceGate {
		t.Fatal("expected transcription mode not to apply silence gate")
	}
	if mode.logAudioRMS {
		t.Fatal("expected transcription mode not to use translation-only RMS behavior")
	}
}

func TestApplyPunctuationMode_Off(t *testing.T) {
	input := "hello world"
	if got := applyPunctuationMode("off", input); got != input {
		t.Fatalf("expected punctuation off to preserve text, got %q", got)
	}
}

func TestApplyPunctuationMode_Conservative(t *testing.T) {
	input := "hello world"
	want := "Hello world."
	if got := applyPunctuationMode("conservative", input); got != want {
		t.Fatalf("expected conservative cleanup %q, got %q", want, got)
	}
}

func TestApplyPunctuationMode_Opinionated(t *testing.T) {
	input := "hello world but i think we should wait"
	want := "Hello world, but I think we should wait."
	if got := applyPunctuationMode("opinionated", input); got != want {
		t.Fatalf("expected opinionated cleanup %q, got %q", want, got)
	}
}

func TestApplyPunctuationMode_ConservativePreservesLineBreaksAndSpacing(t *testing.T) {
	input := "hello  i am here\nsecond line"
	want := "Hello  I am here.\nSecond line."
	if got := applyPunctuationMode("conservative", input); got != want {
		t.Fatalf("expected conservative cleanup %q, got %q", want, got)
	}
}

func TestApplyPunctuationMode_ConservativePreservesOuterWhitespace(t *testing.T) {
	input := "  hello world\nsecond line  \n"
	want := "  Hello world.\nSecond line.  \n"
	if got := applyPunctuationMode("conservative", input); got != want {
		t.Fatalf("expected conservative cleanup %q, got %q", want, got)
	}
}

func TestValidateCachedModel_DoesNotTrustSidecarWithoutHashing(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-test.bin")
	modelBytes := []byte("actual-model-content")
	if err := os.WriteFile(modelPath, modelBytes, 0644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	info, err := os.Stat(modelPath)
	if err != nil {
		t.Fatalf("stat model: %v", err)
	}

	originalManifest := modelManifest
	defer func() { modelManifest = originalManifest }()
	modelManifest = map[string]modelSpec{
		"test": {
			sha256:   strings.Repeat("a", 64),
			exactLen: info.Size(),
		},
	}

	sidecar := filepath.Join(dir, "ggml-test.bin.sha256")
	cached := modelManifest["test"].sha256 + ":" + "0" + ":" + "0"
	if err := os.WriteFile(sidecar, []byte(cached), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	if validateCachedModel(modelPath, "test", slog.Default()) {
		t.Fatal("expected validation to fail after hashing real file")
	}
	if _, err := os.Stat(modelPath + ".bad"); err != nil {
		t.Fatalf("expected bad model quarantine, got %v", err)
	}
}

func TestValidateCachedModel_WritesActualHashSidecar(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-test.bin")
	modelBytes := []byte("actual-model-content")
	if err := os.WriteFile(modelPath, modelBytes, 0644); err != nil {
		t.Fatalf("write model: %v", err)
	}

	sum := sha256.Sum256(modelBytes)
	expectedHash := hex.EncodeToString(sum[:])

	originalManifest := modelManifest
	defer func() { modelManifest = originalManifest }()
	modelManifest = map[string]modelSpec{
		"test": {
			sha256:   expectedHash,
			exactLen: int64(len(modelBytes)),
		},
	}

	if !validateCachedModel(modelPath, "test", slog.Default()) {
		t.Fatal("expected valid model to pass")
	}
	sidecarData, err := os.ReadFile(modelPath + ".sha256")
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if got := strings.TrimSpace(string(sidecarData)); got != expectedHash {
		t.Fatalf("expected sidecar hash %q, got %q", expectedHash, got)
	}
}

func TestQuarantineModel_RemovalFailuresDoNotPanic(t *testing.T) {
	originalRename := renameFile
	originalRemove := removeFile
	defer func() {
		renameFile = originalRename
		removeFile = originalRemove
	}()

	var renamedFrom, renamedTo, removedPath string
	renameFile = func(oldpath, newpath string) error {
		renamedFrom, renamedTo = oldpath, newpath
		return nil
	}
	removeFile = func(path string) error {
		removedPath = path
		return os.ErrPermission
	}

	quarantineModel("/tmp/model.bin", "/tmp/model.bin.sha256", slog.Default(), "test")

	if renamedFrom != "/tmp/model.bin" || renamedTo != "/tmp/model.bin.bad" {
		t.Fatalf("expected model rename, got from=%q to=%q", renamedFrom, renamedTo)
	}
	if removedPath != "/tmp/model.bin.sha256" {
		t.Fatalf("expected hash sidecar removal attempt, got %q", removedPath)
	}
}

func TestValidateCachedModel_SizeMismatchLogsQuarantineFailure(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "ggml-test.bin")
	if err := os.WriteFile(modelPath, []byte("short"), 0644); err != nil {
		t.Fatalf("write model: %v", err)
	}

	originalManifest := modelManifest
	originalRename := renameFile
	defer func() {
		modelManifest = originalManifest
		renameFile = originalRename
	}()
	modelManifest = map[string]modelSpec{
		"test": {
			sha256:   strings.Repeat("a", 64),
			exactLen: 99,
		},
	}

	renameFile = func(oldpath, newpath string) error {
		return errors.New("rename denied")
	}

	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	if validateCachedModel(modelPath, "test", logger) {
		t.Fatal("expected size mismatch to fail validation")
	}
	if !strings.Contains(logs.String(), "failed to quarantine bad model") {
		t.Fatalf("expected quarantine failure to be logged, got logs:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "rename denied") {
		t.Fatalf("expected rename failure detail in logs, got logs:\n%s", logs.String())
	}
}
