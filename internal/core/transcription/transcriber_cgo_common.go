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
	longFormSeconds       = 8                // medium utterances should preserve more context than short push-to-talk clips
)

type whisperTranscriber struct {
	mu         sync.Mutex
	ctx        *C.struct_whisper_context
	lang       string
	vocab      string
	decodeMode string
	punctMode  string
	translate  bool
	sampleRate int
	logger     *slog.Logger
	inflight   chan struct{} // semaphore: capacity 1
}

type decodeConfig struct {
	strategy string
	beamSize int
}

func decodeConfigForMode(mode string) decodeConfig {
	if mode == "greedy" {
		return decodeConfig{strategy: "greedy"}
	}
	return decodeConfig{strategy: "beam", beamSize: whisperBeamSize}
}

func whisperThreadCount(longForm bool) int {
	n := max(1, runtime.NumCPU())
	if longForm {
		return min(n, 4)
	}
	return min(n, 2)
}

func WhisperSystemInfo() string {
	return strings.TrimSpace(C.GoString(C.whisper_print_system_info()))
}

func NewTranscriber(ctx context.Context, modelPath string, modelSize string, language string, sampleRate int, decodeMode string, punctuationMode string, translate bool, logger *slog.Logger) (apppkg.Transcriber, error) {
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

	l.Info("model loaded", "operation", "NewTranscriber",
		"model_size", modelSize, "language", language, "decode_mode", decodeMode,
		"punctuation_mode", punctuationMode, "translate", translate)

	// Pre-warm the GPU shader pipeline with two dummy inferences that match
	// real usage sizes. Vulkan allocates compute scratch buffers per context
	// size, so we must warm with audio_ctx values close to what real
	// dictation will actually use — not a minimal stub.
	//
	// Pass 1 (audio_ctx=200, ~4s): covers typical short push-to-talk clips.
	// Pass 2 (audio_ctx=500, ~10s): covers long-form clips; second pass is
	// fast because shaders and buffers from pass 1 are already resident.
	if runtime.GOOS == "windows" {
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

		l.Info("warming GPU pipeline — pass 2 (long-form)", "operation", "NewTranscriber")
		pass2Start := time.Now()
		warmParams.audio_ctx = C.int(500)
		warmParams.single_segment = C._Bool(false)
		warmParams.no_timestamps = C._Bool(false)
		warmParams.no_context = C._Bool(false)
		if r := C.whisper_full(wctx, warmParams, (*C.float)(unsafe.Pointer(&warmSamples[0])), C.int(len(warmSamples))); r != 0 {
			l.Warn("GPU warmup pass 2 returned non-zero", "operation", "NewTranscriber", "result", int(r))
		}
		l.Info("GPU warmup pass 2 done", "operation", "NewTranscriber", "ms", time.Since(pass2Start).Milliseconds())
		l.Info("GPU pipeline fully warmed", "operation", "NewTranscriber", "total_ms", time.Since(warmStart).Milliseconds())
	}

	return &whisperTranscriber{
		ctx: wctx, lang: language, sampleRate: sampleRate, logger: l,
		decodeMode: decodeMode, punctMode: punctuationMode,
		translate: translate,
		inflight:  make(chan struct{}, 1), vocab: "",
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

	var sumSq float64
	for _, s := range audio {
		sumSq += float64(s) * float64(s)
	}
	rms := 0.0
	if len(audio) > 0 {
		rms = sumSq / float64(len(audio))
	}
	durationSeconds := 0.0
	if t.sampleRate > 0 {
		durationSeconds = float64(len(audio)) / float64(t.sampleRate)
	}
	longForm := durationSeconds >= longFormSeconds
	t.logger.Info("transcribing", "operation", "Transcribe", "samples", len(audio), "audio_rms_sq", rms, "duration_sec", durationSeconds, "long_form", longForm)

	decodeCfg := decodeConfigForMode(t.decodeMode)
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
	params.no_context = C._Bool(true)
	params.single_segment = C._Bool(true)
	params.suppress_blank = C._Bool(true)
	params.suppress_nst = C._Bool(true)
	params.translate = C._Bool(t.translate)
	if decodeCfg.strategy == "greedy" {
		params.greedy.best_of = C.int(1)
		params.temperature = C.float(0.0)
	}
	if longForm {
		params.no_context = C._Bool(false)
		params.no_timestamps = C._Bool(false)
		params.single_segment = C._Bool(false)
	}
	if decodeCfg.strategy == "beam" {
		params.beam_search.beam_size = C.int(decodeCfg.beamSize)
	}

	// Size the mel encoder context to the actual audio duration, but keep a
	// minimum floor of silence padding. Whisper's decoder learns to terminate
	// on a "speech → silence → end" pattern; if we shrink audio_ctx to exactly
	// the speech length, the decoder loses the silence signal and can loop
	// (repeating the last phrase until EOT is finally emitted, if ever).
	//
	// minAudioCtxFrames = 300 (≈6s of mel context) gives whisper enough trailing
	// silence padding to end naturally on sub-6s clips, while still keeping
	// encoder work an order of magnitude below the default 1500 frames (30s).
	const minAudioCtxFrames = 300
	audioCtxFrames := int(durationSeconds*50) + 64
	if audioCtxFrames < minAudioCtxFrames {
		audioCtxFrames = minAudioCtxFrames
	}
	if audioCtxFrames > 1500 {
		audioCtxFrames = 1500
	}
	params.audio_ctx = C.int(audioCtxFrames)

	// Cap tokens per segment to stop whisper's "hello. hello." repetition
	// hallucination — with temperature=0 and no_timestamps=true on short
	// clips, greedy decoding can get stuck looping before EOT. Conservatively:
	// ~15 tokens/second of audio is already fast speech; add generous padding.
	// Whisper's temperature_inc/entropy_thold defaults already trigger a
	// retry at higher temperature when the segment's compression ratio spikes,
	// so no need to override those here (verified against whisper.cpp source).
	maxTokens := int(durationSeconds*15) + 32
	params.max_tokens = C.int(maxTokens)

	if t.lang != "" {
		cLang := C.CString(t.lang)
		defer C.free(unsafe.Pointer(cLang))
		params.language = cLang
	}

	if t.vocab != "" {
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
			continue // skip nil segments instead of crashing
		}
		sb.WriteString(C.GoString(cText))
		if sb.Len() > maxTranscribeBytes {
			t.logger.Warn("output size capped", "operation", "Transcribe", "bytes", sb.Len(), "max", maxTranscribeBytes)
			break
		}
	}

	text := sanitizeTranscript(strings.TrimSpace(sb.String()))
	text = applyPunctuationMode(t.punctMode, text)

	// Estimate the decoded token count. Whisper's BPE averages ~4 characters
	// per token, so len(text)/4 is a decent proxy. When we're close to the
	// cap, the output was likely truncated — log a warning so users can see
	// why their sentence looks cut off.
	estimatedTokens := len(text) / 4
	if estimatedTokens >= int(params.max_tokens)-2 {
		t.logger.Warn("output may be truncated by max_tokens cap",
			"operation", "Transcribe",
			"estimated_tokens", estimatedTokens,
			"max_tokens", int(params.max_tokens),
			"text_length", len(text))
	}

	decodeModeDetail := decodeCfg.strategy
	if decodeCfg.strategy == "beam" {
		decodeModeDetail = fmt.Sprintf("beam(size=%d)", decodeCfg.beamSize)
	} else if decodeCfg.strategy == "greedy" {
		decodeModeDetail = fmt.Sprintf("greedy(best_of=%d)", int(params.greedy.best_of))
	}
	t.logger.Info("transcribed", "operation", "Transcribe",
		"segments", n, "text_length", len(text), "decode_ms", decodeMs,
		"threads", int(params.n_threads), "audio_ctx", audioCtxFrames,
		"max_tokens", int(params.max_tokens),
		"decode_mode", decodeModeDetail,
		"temperature", float32(params.temperature),
		"translate", t.translate,
		"language", t.lang,
		"long_form", longForm)

	// Preview of the raw text at Debug level (privacy-sensitive; not in
	// regular logs). Rune-safe truncation so CJK/accented text isn't split
	// mid-codepoint.
	t.logger.Debug("transcribed text preview", "operation", "Transcribe",
		"text_preview", truncateRunes(text, 120))

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
