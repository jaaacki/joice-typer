package transcription

import "testing"

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"empty", "", 10, ""},
		{"under limit ascii", "hello", 10, "hello"},
		{"at limit ascii", "hello", 5, "hello"},
		{"over limit ascii", "hello world", 5, "hello…"},
		{"zero limit", "hello", 0, ""},
		{"negative limit", "hello", -1, ""},
		// Multi-byte: each Chinese char is 3 UTF-8 bytes. Byte-based slicing
		// would split mid-codepoint here. Rune-based must not.
		{"cjk under limit", "你好", 5, "你好"},
		{"cjk at limit", "你好世界", 4, "你好世界"},
		{"cjk over limit", "你好世界来", 3, "你好世…"},
		// Accented Latin: é is 2 UTF-8 bytes.
		{"accented over limit", "café du monde", 4, "café…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}
