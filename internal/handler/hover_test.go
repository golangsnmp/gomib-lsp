package handler

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateRuneSafe(t *testing.T) {
	// "é" is 2 bytes (0xC3 0xA9). Cutting at byte 1 lands mid-rune.
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"short ascii unchanged", "hello", 10, "hello"},
		{"exact length unchanged", "hello", 5, "hello"},
		{"ascii truncated", "hello world", 5, "hello..."},
		{"cut at rune boundary", "café", 3, "caf..."}, // s[3] is start of "é" rune, clean cut
		{"cut between runes", "ééé", 4, "éé..."},      // boundary at byte 4 (after 2nd "é")
		{"cut mid-rune backs up", "ééé", 3, "é..."},   // would land mid-rune; back to byte 2
		{"3-byte rune cut", "日本語", 8, "日本..."},        // 3+3=6 boundary, n=8 lands inside 3rd rune
		{"4-byte rune cut", "🚀🚀🚀", 5, "🚀..."},         // first rune is 4 bytes
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.in, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncate(%q, %d) = %q, not valid UTF-8", tt.in, tt.n, got)
			}
		})
	}
}
