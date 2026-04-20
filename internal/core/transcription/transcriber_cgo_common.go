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
)

type whisperTranscriber struct {
	mu         sync.Mutex
	ctx        *C.struct_whisper_context
	lang       string
	vocab      string
	decodeMode string
	punctMode  string
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

func NewTranscriber(ctx context.Context, modelPath string, modelSize string, language string, sampleRate int, decodeMode string, punctuationMode string, logger *slog.Logger) (apppkg.Transcriber, error) {
	l := logger.With("component", "transcriber")

	if err := ensureModel(ctx, modelPath, modelSize, l); err != nil {
		return nil, fmt.Errorf("transcriber.NewTranscriber: %w", err)
	}

	l.Info("loading model", "operation", "NewTranscriber", "model_path", modelPath)

	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	cparams := C.whisper_context_default_params()
	wctx := C.whisper_init_from_file_with_params(cPath, cparams)
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
	return &whisperTranscriber{
		ctx: wctx, lang: language, sampleRate: sampleRate, logger: l,
		decodeMode: decodeMode, punctMode: punctuationMode,
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

	t.logger.Info("transcribing", "operation", "Transcribe", "samples", len(audio))

	decodeCfg := decodeConfigForMode(t.decodeMode)
	params := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
	if decodeCfg.strategy == "beam" {
		params = C.whisper_full_default_params(C.WHISPER_SAMPLING_BEAM_SEARCH)
	}
	params.print_progress = C._Bool(false)
	params.print_timestamps = C._Bool(false)
	params.print_realtime = C._Bool(false)
	params.print_special = C._Bool(false)
	params.single_segment = C._Bool(false)
	if decodeCfg.strategy == "beam" {
		params.beam_search.beam_size = C.int(decodeCfg.beamSize)
	}

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

	result := C.whisper_full(t.ctx, params, (*C.float)(unsafe.Pointer(&audio[0])), C.int(len(audio)))
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
	t.logger.Info("transcribed", "operation", "Transcribe",
		"segments", n, "text_length", len(text))
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
