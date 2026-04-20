package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogTail_ReturnsLast500Lines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "voicetype.log")

	var builder strings.Builder
	for i := 1; i <= 600; i++ {
		fmt.Fprintf(&builder, "line %03d\n", i)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	content, truncated, err := ReadLogTail(path, 500)
	if err != nil {
		t.Fatalf("ReadLogTail: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncated tail to be reported")
	}

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if got, want := len(lines), 500; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	if lines[0] != "line 101" {
		t.Fatalf("first line = %q, want %q", lines[0], "line 101")
	}
	if lines[len(lines)-1] != "line 600" {
		t.Fatalf("last line = %q, want %q", lines[len(lines)-1], "line 600")
	}
}

func TestReadLogTail_ReturnsTruncatedFalseWhenNoLinesOmitted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "voicetype.log")

	content := "line 001\nline 002\nline 003\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	got, truncated, err := ReadLogTail(path, 500)
	if err != nil {
		t.Fatalf("ReadLogTail: %v", err)
	}
	if truncated {
		t.Fatal("expected truncated=false when no lines are omitted")
	}
	if got != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
}

func TestReadFullLog_ReturnsAllContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "voicetype.log")

	content := "line 001\nline 002\nline 003\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	got, err := ReadFullLog(path)
	if err != nil {
		t.Fatalf("ReadFullLog: %v", err)
	}
	if got != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
}

func TestReadLogTail_MissingFileIsClean(t *testing.T) {
	content, truncated, err := ReadLogTail(filepath.Join(t.TempDir(), "missing.log"), 500)
	if err != nil {
		t.Fatalf("ReadLogTail: %v", err)
	}
	if content != "" {
		t.Fatalf("content = %q, want empty", content)
	}
	if truncated {
		t.Fatal("expected truncated=false for missing file")
	}
}
func TestReadFullLog_MissingFileIsClean(t *testing.T) {
	content, err := ReadFullLog(filepath.Join(t.TempDir(), "missing.log"))
	if err != nil {
		t.Fatalf("ReadFullLog: %v", err)
	}
	if content != "" {
		t.Fatalf("content = %q, want empty", content)
	}
}
