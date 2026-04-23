package transcription

// truncateRunes returns s truncated to at most maxRunes runes, appending "…"
// when truncation occurs. Byte-safe for multi-byte UTF-8 sequences (Chinese,
// Japanese, accented characters) — range over a Go string iterates by rune,
// and `s[:i]` at a rune boundary i is valid UTF-8.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i] + "…"
		}
		count++
	}
	return s
}
