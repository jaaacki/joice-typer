package transcription

import "testing"

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
