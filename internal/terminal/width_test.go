package terminal

import (
	"strings"
	"testing"
)

func TestVisibleWidthHandlesCombiningMarksAndCJK(t *testing.T) {
	if got := VisibleWidth("é"); got != 1 {
		t.Errorf("VisibleWidth(e + combining acute) = %d; want 1", got)
	}
	if got := VisibleWidth("表"); got != 2 {
		t.Errorf("VisibleWidth(表) = %d; want 2", got)
	}
}

func TestVisibleWidthSkipsANSIEscapes(t *testing.T) {
	if got := VisibleWidth("\x1b[34mDate\x1b[0m"); got != 4 {
		t.Errorf("VisibleWidth with ANSI escapes = %d; want 4", got)
	}
}

func TestTruncateToWidthKeepsValuesThatAlreadyFit(t *testing.T) {
	if got := TruncateToWidth("Loading usage logs", 40); got != "Loading usage logs" {
		t.Errorf("TruncateToWidth(fitting value) = %q; want unchanged", got)
	}
}

func TestTruncateToWidthStaysWithinTheRequestedWidth(t *testing.T) {
	if got := TruncateToWidth("Loading usage logs", 10); got != "Loading u…" {
		t.Errorf("TruncateToWidth(10) = %q; want %q", got, "Loading u…")
	}
}

func TestTruncateToWidthWritesNothingWhenNoColumnIsAvailable(t *testing.T) {
	if got := TruncateToWidth("Loading usage logs", 0); got != "" {
		t.Errorf("TruncateToWidth(0) = %q; want empty", got)
	}
	if got := TruncateToWidth("Loading usage logs", 1); got != "…" {
		t.Errorf("TruncateToWidth(1) = %q; want ellipsis", got)
	}
}

func TestTruncateToWidthNeverSplitsAWideCharAcrossTheBoundary(t *testing.T) {
	if got := TruncateToWidth("表表表表", 5); got != "表表…" {
		t.Errorf("TruncateToWidth(表表表表, 5) = %q; want %q", got, "表表…")
	}
}

func TestTruncateToWidthPreservesANSIReset(t *testing.T) {
	truncated := TruncateToWidth("\x1b[33mvery-long-value\x1b[0m", 8)
	if !strings.HasSuffix(truncated, "\x1b[0m…") {
		t.Errorf("truncated %q should end with ANSI reset + ellipsis", truncated)
	}
	// Port of the Rust insta snapshot snapshots_ansi_truncation_boundary.
	if want := "\x1b[33mvery-lo\x1b[0m…"; truncated != want {
		t.Errorf("TruncateToWidth ANSI boundary = %q; want %q", truncated, want)
	}
}

func TestCharDisplayWidthHandlesStandardWidthCases(t *testing.T) {
	cases := []struct {
		r    rune
		want int
	}{
		{'a', 1},
		{'表', 2},
		{'\u0301', 0},
		{'\x07', 0},
		{'±', 1},
	}
	for _, c := range cases {
		if got := charDisplayWidth(c.r); got != c.want {
			t.Errorf("charDisplayWidth(%q) = %d; want %d", c.r, got, c.want)
		}
	}
}
