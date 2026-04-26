//go:build darwin || (windows && cgo)

package transcription

/*
#include <whisper.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"strings"
	"sync"
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
	longFormSeconds       = 8 // medium utterances should preserve more context than short push-to-talk clips
	shortFormMaxTokens    = 8
	shortFormMaxLen       = 32
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
}

type decodeConfig struct {
	strategy string
	beamSize int
}

type runtimeDecodeMode struct {
	applySilenceGate bool
	logAudioRMS      bool
}

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
	return min(shortFormMaxTokens, max(4, int(math.Ceil(durationSeconds*2.5))+2))
}

func decodeConfigForMode(mode string) decodeConfig {
	if mode == "greedy" {
		return decodeConfig{strategy: "greedy"}
	}
	return decodeConfig{strategy: "beam", beamSize: whisperBeamSize}
}

func effectiveDecodeConfig(mode string, longForm bool, outputMode string) decodeConfig {
	if outputMode != "translation" && !longForm {
		return decodeConfigForMode("greedy")
	}
	return decodeConfigForMode(mode)
}

func whisperThreadCount(longForm bool) int {
	n := max(1, runtime.NumCPU())
	if longForm {
		return min(n, 4)
	}
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

	l.Info("model loaded", "operation", "NewTranscriber")

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
		inflight: make(chan struct{}, 1), vocab: "",
	}, nil
}

type transcribeResult struct {
	text string
	err  error
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

	ch := make(chan transcribeResult, 1)
	go func() {
		defer func() { <-t.inflight }() // release semaphore when done
		text, err := t.transcribeBlocking(audio)
		ch <- transcribeResult{text, err}
	}()

	select {
	case <-ctx.Done():
		// Don't release semaphore here — goroutine still running
		return "", &apppkg.ErrDependencyTimeout{
			Component: "transcriber",
			Operation: "Transcribe",
			Wrapped:   ctx.Err(),
		}
	case result := <-ch:
		return result.text, result.err
	}
}

func (t *whisperTranscriber) transcribeBlocking(audio []float32) (string, error) {
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
	longForm := durationSeconds >= longFormSeconds
	mode := runtimeDecodeModeForOutputMode(t.outputMode)

	decodeCfg := effectiveDecodeConfig(t.decodeMode, longForm, t.outputMode)
	params := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
	if decodeCfg.strategy == "beam" {
		params = C.whisper_full_default_params(C.WHISPER_SAMPLING_BEAM_SEARCH)
	}
	params.print_progress = C._Bool(false)
	params.print_timestamps = C._Bool(false)
	params.print_realtime = C._Bool(false)
	params.print_special = C._Bool(false)
	params.n_threads = C.int(whisperThreadCount(longForm))
	params.no_timestamps = C._Bool(true)

	audioCtxFrames := 1500
	t.logger.Info("transcribing", "operation", "Transcribe", "samples", len(audio), "audio_rms", rms, "duration_sec", durationSeconds, "long_form", longForm)
	if rms < minTranscribeRMS {
		t.logger.Info("skipping transcription — audio below energy threshold", "operation", "Transcribe", "rms", rms)
		return "", nil
	}
	if mode.logAudioRMS {
		params.no_context = C._Bool(true)
		params.single_segment = C._Bool(false)
		params.translate = C._Bool(true)
	} else {
		params.no_context = C._Bool(true)
		params.single_segment = C._Bool(true)
		params.suppress_blank = C._Bool(true)
		params.suppress_nst = C._Bool(true)
		if decodeCfg.strategy == "greedy" {
			params.greedy.best_of = C.int(1)
			params.temperature = C.float(0.0)
		}
		if longForm {
			params.no_context = C._Bool(false)
			params.no_timestamps = C._Bool(false)
			params.single_segment = C._Bool(false)
		} else {
			params.max_tokens = C.int(shortFormTokenLimit(durationSeconds))
			params.max_len = C.int(shortFormMaxLen)
			params.split_on_word = C._Bool(true)
			params.duration_ms = C.int(durationSeconds*1000) + C.int(250)
			params.temperature_inc = C.float(0.0)
			params.no_speech_thold = C.float(0.55)
			params.entropy_thold = C.float(2.4)
			params.logprob_thold = C.float(-1.0)
		}
		audioCtxFrames = int(durationSeconds*50) + 64
		if audioCtxFrames > 1500 {
			audioCtxFrames = 1500
		}
		params.audio_ctx = C.int(audioCtxFrames)
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
	for i := 0; i < n; i++ {
		cText := C.whisper_full_get_segment_text(t.ctx, C.int(i))
		if cText == nil {
			continue
		}
		sb.WriteString(C.GoString(cText))
		if sb.Len() > maxTranscribeBytes {
			t.logger.Warn("output size capped", "operation", "Transcribe", "bytes", sb.Len(), "max", maxTranscribeBytes)
			break
		}
	}

	text := sanitizeTranscript(strings.TrimSpace(sb.String()))
	text = applyPunctuationMode(t.punctMode, text)
	t.logger.Info("transcribed", "operation", "Transcribe",
		"segments", n, "text_length", len(text), "decode_ms", decodeMs, "threads", int(params.n_threads), "audio_ctx", audioCtxFrames, "decode_strategy", decodeCfg.strategy, "max_tokens", int(params.max_tokens), "max_len", int(params.max_len))
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
	t.logger.Info("closing", "operation", "Close")

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
