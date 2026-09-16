package worker

import (
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Errorf("got %q", got)
	}
	// "é" is 2 bytes: cutting at byte 2 would split it.
	if got := truncate("aé", 2); got != "a…" || !utf8.ValidString(got) {
		t.Errorf("got %q", got)
	}
}
