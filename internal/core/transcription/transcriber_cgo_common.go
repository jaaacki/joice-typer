//go:build darwin || (windows && cgo)

package transcription

/*
#include <whisper.h>
#include <stdlib.h>
#include <stdbool.h>
#include <stdatomic.h>

typedef struct whisper_abort_flag {
	atomic_bool cancelled;
} whisper_abort_flag;

static whisper_abort_flag * whisper_abort_flag_new(void) {
	whisper_abort_flag *flag = (whisper_abort_flag *)calloc(1, sizeof(whisper_abort_flag));
	if (flag != NULL) {
		atomic_init(&flag->cancelled, false);
	}
	return flag;
}

static void whisper_abort_flag_cancel(whisper_abort_flag *flag) {
	if (flag != NULL) {
		atomic_store_explicit(&flag->cancelled, true, memory_order_release);
	}
}

static bool whisper_abort_flag_is_cancelled(whisper_abort_flag *flag) {
	if (flag == NULL) {
		return false;
	}
	return atomic_load_explicit(&flag->cancelled, memory_order_acquire);
}

static bool whisper_abort_callback_bridge(void *data) {
	if (data == NULL) {
		return false;
	}
	whisper_abort_flag *flag = (whisper_abort_flag *)data;
	return atomic_load_explicit(&flag->cancelled, memory_order_acquire);
}

static ggml_abort_callback whisper_abort_callback_ptr(void) {
	return whisper_abort_callback_bridge;
}
*/
import "C"

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	apppkg "voicetype/internal/core/runtime"
)

const (
	maxTranscribeSeconds  = 90               // reject audio longer than 90s
	maxTranscribeSegments = 500              // cap whisper segments to prevent runaway
	maxTranscribeBytes    = 50000            // cap output text to ~50KB
	transcribeTimeout     = 90 * time.Second // hard deadline for whisper_full
	whisperBeamSize       = 5
	shortFormMaxTokens    = 64
	shortFormMaxLen       = 256
	minTranscribeRMS      = 0.001
)

type whisperTranscriber struct {
	mu         sync.Mutex
	ctx        *C.struct_whisper_context
	lang       string
	vocab      string
	decodeMode string
	punctMode  string
	outputMode string
	sampleRate int
	logger     *slog.Logger
	inflight   chan struct{} // semaphore: capacity 1
	generation uint64
	nextRunID  uint64
}

type decodeConfig struct {
	strategy string
	beamSize int
}

type runtimeDecodeMode struct {
	applySilenceGate bool
	logAudioRMS      bool
}

var nextTranscriberGeneration uint64

func audioRMS(audio []float32) float64 {
	if len(audio) == 0 {
		return 0
	}
	var sumSq float64
	for _, s := range audio {
		sumSq += float64(s) * float64(s)
	}
	return math.Sqrt(sumSq / float64(len(audio)))
}

func shortFormTokenLimit(durationSeconds float64) int {
	return min(shortFormMaxTokens, max(16, int(math.Ceil(durationSeconds*6))+8))
}

func transcriptHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

func transcriptPreview(text string) string {
	if os.Getenv("JOICETYPER_LOG_TRANSCRIPT_PREVIEW") != "1" {
		return ""
	}
	const maxRunes = 80
	text = strings.ReplaceAll(text, "\n", " ")
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func decodeConfigForMode(mode string) decodeConfig {
	if mode == "greedy" {
		return decodeConfig{strategy: "greedy"}
	}
	return decodeConfig{strategy: "beam", beamSize: whisperBeamSize}
}

func effectiveDecodeConfig(mode string, outputMode string) decodeConfig {
	return decodeConfigForMode(mode)
}

func whisperThreadCount() int {
	n := max(1, runtime.NumCPU())
	return min(n, 2)
}

func runtimeDecodeModeForOutputMode(outputMode string) runtimeDecodeMode {
	if outputMode == "translation" {
		return runtimeDecodeMode{applySilenceGate: true, logAudioRMS: true}
	}
	return runtimeDecodeMode{}
}

func WhisperSystemInfo() string {
	return strings.TrimSpace(C.GoString(C.whisper_print_system_info()))
}

func NewTranscriber(ctx context.Context, modelPath string, modelSize string, language string, sampleRate int, decodeMode string, punctuationMode string, outputMode string, logger *slog.Logger) (apppkg.Transcriber, error) {
	l := logger.With("component", "transcriber")

	if err := ensureModel(ctx, modelPath, modelSize, l); err != nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: %w", err)
	}

	l.Info("loading model", "operation", "NewTranscriber", "model_path", modelPath)
	sysInfo := WhisperSystemInfo()
	if sysInfo != "" {
		l.Info("whisper system info", "operation", "NewTranscriber", "system_info", sysInfo)
	}
	if runtime.GOOS == "windows" {
		logWindowsBackendInventory(l)
		beginWindowsWhisperBackendLogging(l)
	}

	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	cparams := C.whisper_context_default_params()
	if runtime.GOOS == "windows" {
		gpuIndex, gpuBackend, ok := windowsSelectedGPUDevice()
		if !ok {
			return nil, &apppkg.ErrDependencyUnavailable{
				Component: "transcriber",
				Operation: "NewTranscriber",
				Wrapped:   fmt.Errorf("no Vulkan-capable GPU or iGPU backend available"),
			}
		}
		cparams.use_gpu = C.bool(true)
		cparams.gpu_device = C.int(gpuIndex)
		cparams.flash_attn = C.bool(false)
		l.Info("requesting GPU backend", "operation", "NewTranscriber", "gpu_device", gpuIndex, "gpu_name", gpuBackend.Name, "gpu_description", gpuBackend.Description, "flash_attn", false)
	}
	wctx := C.whisper_init_from_file_with_params(cPath, cparams)
	if runtime.GOOS == "windows" {
		backendState := endWindowsWhisperBackendLogging()
		if backendState.noGPUFound {
			l.Warn("GPU backend reported no GPU found — performance will be degraded", "operation", "NewTranscriber")
		} else if backendState.usingBackend != "" {
			l.Info("GPU backend activated", "operation", "NewTranscriber", "backend", backendState.usingBackend, "using_vulkan", backendState.usingVulkan)
		}
	}
	if wctx == nil {
		quarantineModel(modelPath, "", l, "NewTranscriber")
		return nil, &apppkg.ErrBadPayload{Component: "transcriber", Operation: "NewTranscriber", Detail: "model corrupt or incompatible"}
	}

	// Validate language against whisper's own language list
	if language != "" {
		cLang := C.CString(language)
		defer C.free(unsafe.Pointer(cLang))
		if C.whisper_lang_id(cLang) < 0 {
			C.whisper_free(wctx)
			return nil, &apppkg.ErrBadPayload{Component: "transcriber", Operation: "NewTranscriber", Detail: fmt.Sprintf("unsupported language %q", language)}
		}
	}

	generation := atomic.AddUint64(&nextTranscriberGeneration, 1)
	l.Info("model loaded", "operation", "NewTranscriber", "generation", generation, "model_size", modelSize, "language", language, "decode_mode", decodeMode, "punctuation_mode", punctuationMode, "output_mode", outputMode)

	// Pre-warm the GPU pipeline with two dummy inferences. On Windows this
	// allocates Vulkan compute scratch buffers; on macOS it compiles and caches
	// Metal shaders. Without this, the first real dictation after launch pays
	// the full compilation cost and feels noticeably slow.
	//
	// Both passes MUST keep no_context=true and single_segment=true. Otherwise
	// whisper writes the tokens it hallucinates from 0.2s of silence into
	// state->prompt_past, which then biases the first real decode toward
	// rubbish output. We sacrifice some long-form-path warming to avoid
	// state pollution — the long-form path warms on first long use.
	//
	// Pass 1 (audio_ctx=200, ~4s clip): covers typical short push-to-talk.
	// Pass 2 (audio_ctx=500, ~10s clip): warms the bigger encoder buffer
	// shape; kernels compiled in pass 1 are already resident.
	{
		l.Info("warming GPU pipeline — pass 1 (short-form)", "operation", "NewTranscriber")
		warmStart := time.Now()
		warmSamples := make([]float32, 3200) // 0.2s of silence at 16kHz

		warmParams := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
		warmParams.print_progress = C._Bool(false)
		warmParams.print_realtime = C._Bool(false)
		warmParams.print_special = C._Bool(false)
		warmParams.n_threads = C.int(2)
		warmParams.no_timestamps = C._Bool(true)
		warmParams.no_context = C._Bool(true)
		warmParams.single_segment = C._Bool(true)
		warmParams.suppress_blank = C._Bool(true)
		warmParams.suppress_nst = C._Bool(true)
		warmParams.temperature = C.float(0.0)
		warmParams.greedy.best_of = C.int(1)
		warmParams.audio_ctx = C.int(200)
		if r := C.whisper_full(wctx, warmParams, (*C.float)(unsafe.Pointer(&warmSamples[0])), C.int(len(warmSamples))); r != 0 {
			l.Warn("GPU warmup pass 1 returned non-zero", "operation", "NewTranscriber", "result", int(r))
		}
		l.Info("GPU warmup pass 1 done", "operation", "NewTranscriber", "ms", time.Since(warmStart).Milliseconds())

		l.Info("warming GPU pipeline — pass 2 (encoder buffer)", "operation", "NewTranscriber")
		pass2Start := time.Now()
		warmParams.audio_ctx = C.int(500)
		// Intentionally KEEP no_context=true and single_segment=true here to
		// avoid polluting state->prompt_past with silence-hallucination tokens.
		if r := C.whisper_full(wctx, warmParams, (*C.float)(unsafe.Pointer(&warmSamples[0])), C.int(len(warmSamples))); r != 0 {
			l.Warn("GPU warmup pass 2 returned non-zero", "operation", "NewTranscriber", "result", int(r))
		}
		l.Info("GPU warmup pass 2 done", "operation", "NewTranscriber", "ms", time.Since(pass2Start).Milliseconds())
		l.Info("GPU pipeline fully warmed", "operation", "NewTranscriber", "total_ms", time.Since(warmStart).Milliseconds())
	}

	return &whisperTranscriber{
		ctx: wctx, lang: language, sampleRate: sampleRate, logger: l,
		decodeMode: decodeMode, punctMode: punctuationMode, outputMode: outputMode,
		inflight: make(chan struct{}, 1), vocab: "", generation: generation,
	}, nil
}

type transcribeResult struct {
	text string
	err  error
}

type whisperAbortFlag struct {
	ptr *C.whisper_abort_flag
}

func newWhisperAbortFlag() (*whisperAbortFlag, error) {
	ptr := C.whisper_abort_flag_new()
	if ptr == nil {
		return nil, fmt.Errorf("transcriber.NewAbortFlag: allocate abort flag")
	}
	return &whisperAbortFlag{ptr: ptr}, nil
}

func (f *whisperAbortFlag) cancel() {
	if f != nil && f.ptr != nil {
		C.whisper_abort_flag_cancel(f.ptr)
	}
}

func (f *whisperAbortFlag) cancelled() bool {
	return f != nil && f.ptr != nil && bool(C.whisper_abort_flag_is_cancelled(f.ptr))
}

func (f *whisperAbortFlag) free() {
	if f != nil && f.ptr != nil {
		C.free(unsafe.Pointer(f.ptr))
		f.ptr = nil
	}
}

func (t *whisperTranscriber) Transcribe(ctx context.Context, audio []float32) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("transcriber.Transcribe: empty audio buffer")
	}

	// Cap input audio duration using the actual configured sample rate
	rate := t.sampleRate
	if rate <= 0 {
		rate = 16000
	}
	maxSamples := rate * maxTranscribeSeconds
	if len(audio) > maxSamples {
		return "", &apppkg.ErrBadPayload{
			Component: "transcriber",
			Operation: "Transcribe",
			Detail:    fmt.Sprintf("audio too long: %d samples (%ds at %dHz), max %ds", len(audio), len(audio)/rate, rate, maxTranscribeSeconds),
		}
	}

	// Apply default deadline if caller didn't set one
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, transcribeTimeout)
		defer cancel()
	}

	// Bulkhead: only 1 in-flight transcription. If a previous CGO call
	// is still running (timed out but goroutine alive), reject immediately
	// instead of stacking goroutines behind the mutex.
	select {
	case t.inflight <- struct{}{}:
		// acquired
	default:
		return "", &apppkg.ErrDependencyTimeout{
			Component: "transcriber",
			Operation: "Transcribe",
			Wrapped:   fmt.Errorf("previous transcription still in flight"),
		}
	}

	abortFlag, err := newWhisperAbortFlag()
	if err != nil {
		<-t.inflight
		return "", err
	}

	ch := make(chan transcribeResult, 1)
	go func() {
		defer func() {
			abortFlag.free()
			<-t.inflight
		}()
		text, err := t.transcribeBlocking(audio, abortFlag)
		ch <- transcribeResult{text, err}
	}()

	select {
	case <-ctx.Done():
		abortFlag.cancel()
		select {
		case result := <-ch:
			if result.err != nil {
				t.logger.Warn("whisper aborted after context cancellation",
					"operation", "Transcribe", "generation", t.generation, "error", result.err)
			}
		case <-time.After(5 * time.Second):
			t.logger.Error("whisper did not return after abort callback was signalled",
				"operation", "Transcribe", "generation", t.generation)
		}
		return "", &apppkg.ErrDependencyTimeout{
			Component: "transcriber",
			Operation: "Transcribe",
			Wrapped:   ctx.Err(),
		}
	case result := <-ch:
		return result.text, result.err
	}
}

func (t *whisperTranscriber) transcribeBlocking(audio []float32, abortFlag *whisperAbortFlag) (string, error) {
	runID := atomic.AddUint64(&t.nextRunID, 1)
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ctx == nil {
		return "", fmt.Errorf("transcriber.Transcribe: transcriber is closed")
	}

	rms := audioRMS(audio)
	durationSeconds := 0.0
	if t.sampleRate > 0 {
		durationSeconds = float64(len(audio)) / float64(t.sampleRate)
	}
	mode := runtimeDecodeModeForOutputMode(t.outputMode)

	decodeCfg := effectiveDecodeConfig(t.decodeMode, t.outputMode)
	params := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
	if decodeCfg.strategy == "beam" {
		params = C.whisper_full_default_params(C.WHISPER_SAMPLING_BEAM_SEARCH)
	}
	params.print_progress = C._Bool(false)
	params.print_timestamps = C._Bool(false)
	params.print_realtime = C._Bool(false)
	params.print_special = C._Bool(false)
	params.abort_callback = C.whisper_abort_callback_ptr()
	params.abort_callback_user_data = unsafe.Pointer(abortFlag.ptr)
	params.n_threads = C.int(whisperThreadCount())
	params.no_timestamps = C._Bool(true)

	audioCtxFrames := int(durationSeconds*50) + 64
	if audioCtxFrames > 1500 {
		audioCtxFrames = 1500
	}
	if audioCtxFrames < 96 {
		audioCtxFrames = 96
	}
	t.logger.Info("transcribing", "operation", "Transcribe",
		"generation", t.generation,
		"run_id", runID,
		"samples", len(audio),
		"audio_rms", rms,
		"duration_sec", durationSeconds,
		"profile", t.outputMode,
		"configured_decode_mode", t.decodeMode,
		"effective_decode_strategy", decodeCfg.strategy,
		"punctuation_mode", t.punctMode,
		"vocab_length", len(t.vocab))
	if rms < minTranscribeRMS {
		t.logger.Info("skipping transcription — audio below energy threshold", "operation", "Transcribe", "rms", rms)
		return "", nil
	}
	if mode.logAudioRMS {
		params.no_context = C._Bool(true)
		params.single_segment = C._Bool(false)
		params.translate = C._Bool(true)
		params.audio_ctx = C.int(audioCtxFrames)
	} else {
		params.no_context = C._Bool(true)
		params.single_segment = C._Bool(true)
		params.suppress_blank = C._Bool(true)
		params.suppress_nst = C._Bool(true)
		params.audio_ctx = C.int(audioCtxFrames)
		params.max_tokens = C.int(shortFormTokenLimit(durationSeconds))
		params.max_len = C.int(shortFormMaxLen)
		params.split_on_word = C._Bool(true)
		params.duration_ms = C.int(durationSeconds*1000) + C.int(250)
		params.temperature_inc = C.float(0.0)
		params.no_speech_thold = C.float(0.55)
		params.entropy_thold = C.float(2.4)
		params.logprob_thold = C.float(-1.0)
		if decodeCfg.strategy == "greedy" {
			params.greedy.best_of = C.int(1)
			params.temperature = C.float(0.0)
		}
	}

	if decodeCfg.strategy == "beam" {
		params.beam_search.beam_size = C.int(decodeCfg.beamSize)
	}

	if t.lang != "" {
		cLang := C.CString(t.lang)
		defer C.free(unsafe.Pointer(cLang))
		params.language = cLang
	}

	if t.vocab != "" && t.outputMode != "translation" {
		cPrompt := C.CString(t.vocab)
		defer C.free(unsafe.Pointer(cPrompt))
		params.initial_prompt = cPrompt
	}

	decodeStart := time.Now()
	result := C.whisper_full(t.ctx, params, (*C.float)(unsafe.Pointer(&audio[0])), C.int(len(audio)))
	decodeMs := time.Since(decodeStart).Milliseconds()
	if result != 0 {
		return "", fmt.Errorf("transcriber.Transcribe: whisper_full returned %d", result)
	}

	n := int(C.whisper_full_n_segments(t.ctx))
	if n > maxTranscribeSegments {
		t.logger.Warn("segment count capped", "operation", "Transcribe", "actual", n, "max", maxTranscribeSegments)
		n = maxTranscribeSegments
	}

	var sb strings.Builder
	segmentsKept := 0
	for i := 0; i < n; i++ {
		cText := C.whisper_full_get_segment_text(t.ctx, C.int(i))
		if cText == nil {
			continue
		}
		segmentsKept++
		sb.WriteString(C.GoString(cText))
		if sb.Len() > maxTranscribeBytes {
			t.logger.Warn("output size capped", "operation", "Transcribe", "bytes", sb.Len(), "max", maxTranscribeBytes)
			break
		}
	}

	text := sanitizeTranscript(strings.TrimSpace(sb.String()))
	text = applyPunctuationMode(t.punctMode, text)
	t.logger.Info("transcribed", "operation", "Transcribe",
		"generation", t.generation,
		"run_id", runID,
		"segments", n,
		"segments_kept", segmentsKept,
		"text_length", len(text),
		"text_hash", transcriptHash(text),
		"text_preview", transcriptPreview(text),
		"decode_ms", decodeMs,
		"threads", int(params.n_threads),
		"audio_ctx", audioCtxFrames,
		"decode_strategy", decodeCfg.strategy,
		"configured_decode_mode", t.decodeMode,
		"no_context", bool(params.no_context),
		"single_segment", bool(params.single_segment),
		"max_tokens", int(params.max_tokens),
		"max_len", int(params.max_len),
		"duration_ms", int(params.duration_ms),
		"temperature_inc", float32(params.temperature_inc))
	return text, nil
}

// sanitizeTranscript ensures the output is valid UTF-8 with no control characters.
func sanitizeTranscript(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	// Strip C0/C1 control characters except whitespace (tab, newline, carriage return)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func applyPunctuationMode(mode string, text string) string {
	switch mode {
	case "", "conservative":
		return conservativePunctuation(text)
	case "opinionated":
		return opinionatedPunctuation(text)
	default:
		return text
	}
}

func conservativePunctuation(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	text = capitalizeSentenceStarts(text)
	text = capitalizeStandaloneI(text)
	return punctuateLines(text)
}

func opinionatedPunctuation(text string) string {
	text = conservativePunctuation(text)
	for _, marker := range []string{" but ", " so ", " because ", " then "} {
		replacement := "," + marker
		if strings.Contains(text, replacement) {
			continue
		}
		text = strings.Replace(text, marker, replacement, 1)
	}
	text = capitalizeSentenceStarts(text)
	text = capitalizeStandaloneI(text)
	return text
}

func capitalizeSentenceStarts(text string) string {
	runes := []rune(text)
	capNext := true
	for i, r := range runes {
		if unicode.IsLetter(r) && capNext {
			runes[i] = unicode.ToUpper(r)
			capNext = false
			continue
		}
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			capNext = true
		}
	}
	return string(runes)
}

func capitalizeStandaloneI(text string) string {
	runes := []rune(text)
	for i, r := range runes {
		if r != 'i' {
			continue
		}
		prevIsWord := i > 0 && isWordRune(runes[i-1])
		nextIsWord := i+1 < len(runes) && isWordRune(runes[i+1])
		if !prevIsWord && !nextIsWord {
			runes[i] = 'I'
		}
	}
	return string(runes)
}

func hasTerminalPunctuation(text string) bool {
	if text == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(text)
	return last == '.' || last == '!' || last == '?'
}

func punctuateLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = punctuateLine(line)
	}
	return strings.Join(lines, "\n")
}

func punctuateLine(line string) string {
	trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
	if trimmed == "" || hasTerminalPunctuation(trimmed) {
		return line
	}
	return trimmed + "." + line[len(trimmed):]
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// IsInflight returns true if a transcription is currently running (bulkhead occupied).
func (t *whisperTranscriber) IsInflight() bool {
	select {
	case t.inflight <- struct{}{}:
		<-t.inflight // release immediately
		return false
	default:
		return true
	}
}

func (t *whisperTranscriber) SetVocabulary(vocab string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.vocab = vocab
}

func (t *whisperTranscriber) Close() error {
	t.logger.Info("closing", "operation", "Close", "generation", t.generation, "inflight", t.IsInflight())

	// Poll TryLock with a hard deadline. No helper goroutine, no race.
	// TryLock is non-blocking — if the mutex is held by a hung CGO call,
	// it returns false immediately. We poll every 100ms up to 5s.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if t.mu.TryLock() {
			if t.ctx != nil {
				C.whisper_free(t.ctx)
				t.ctx = nil
			}
			t.mu.Unlock()
			t.logger.Info("closed", "operation", "Close", "generation", t.generation)
			return nil
		}
		if time.Now().After(deadline) {
			t.logger.Error("close timed out waiting for transcription lock — whisper context leaked",
				"component", "transcriber", "operation", "Close")
			return &apppkg.ErrDependencyTimeout{
				Component: "transcriber",
				Operation: "Close",
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
