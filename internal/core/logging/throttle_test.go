package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestThrottlerEmitsFirstAndSuppressesWithin(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	th := NewThrottler(50 * time.Millisecond)

	if !th.Log(logger, slog.LevelInfo, "k", "msg") {
		t.Fatal("first log should emit")
	}
	if th.Log(logger, slog.LevelInfo, "k", "msg") {
		t.Fatal("second log within window should suppress")
	}
	lines := strings.Count(buf.String(), "\n")
	if lines != 1 {
		t.Fatalf("expected 1 line, got %d: %q", lines, buf.String())
	}
}

func TestThrottlerEmitsAfterWindowWithSuppressedCount(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	th := NewThrottler(25 * time.Millisecond)

	th.Log(logger, slog.LevelInfo, "k", "msg")
	for i := 0; i < 5; i++ {
		th.Log(logger, slog.LevelInfo, "k", "msg")
	}
	time.Sleep(35 * time.Millisecond)
	if !th.Log(logger, slog.LevelInfo, "k", "msg") {
		t.Fatal("log after window should emit")
	}
	out := buf.String()
	if !strings.Contains(out, `"suppressed_since":5`) {
		t.Fatalf("expected suppressed_since=5 in output, got: %s", out)
	}
}

func TestThrottlerSeparateKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	th := NewThrottler(time.Second)

	if !th.Log(logger, slog.LevelInfo, "a", "msg") {
		t.Fatal("a first should emit")
	}
	if !th.Log(logger, slog.LevelInfo, "b", "msg") {
		t.Fatal("b first should emit (separate key)")
	}
	if th.Log(logger, slog.LevelInfo, "a", "msg") {
		t.Fatal("a second should suppress")
	}
}

func TestThrottlerNilLoggerNoPanic(t *testing.T) {
	th := NewThrottler(time.Second)
	if th.Log(nil, slog.LevelInfo, "k", "msg") {
		t.Fatal("nil logger should not emit")
	}
}
