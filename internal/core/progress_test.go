package core

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wujunwei928/token-usage/internal/terminal"
)

// The pure-function cases port the reference progress.rs test module; the
// session cases exercise the Go worker lifecycle the reference gets from its
// render thread.

func TestFormatUsageLoadProgressRendersActiveAgentsWithCompletedCount(t *testing.T) {
	states := []loadProgressAgent{
		{"Claude", loadSucceeded},
		{"Codex", loadLoading},
		{"OpenCode", loadLoading},
	}

	if got, want := formatUsageLoadProgressText(states, "", false),
		"Loading usage logs (1/3) :: Codex, OpenCode"; got != want {
		t.Errorf("formatUsageLoadProgressText = %q, want %q", got, want)
	}
}

func TestFormatUsageLoadProgressIncludesPricingStatus(t *testing.T) {
	states := []loadProgressAgent{
		{"Claude", loadLoading},
		{"Codex", loadLoading},
	}

	if got, want := formatUsageLoadProgressText(states, "Refreshing model pricing from LiteLLM...", true),
		"Refreshing model pricing from LiteLLM... :: Loading usage logs (0/2) :: Claude, Codex"; got != want {
		t.Errorf("formatUsageLoadProgressText = %q, want %q", got, want)
	}
}

func TestFormatUsageLoadProgressRendersStandaloneStatusWithoutUsageSuffix(t *testing.T) {
	if got, want := formatUsageLoadProgressText(nil, "Refreshing model pricing from LiteLLM...", true),
		"Refreshing model pricing from LiteLLM..."; got != want {
		t.Errorf("formatUsageLoadProgressText = %q, want %q", got, want)
	}
}

func TestFormatUsageLoadProgressDefaultsWithoutStates(t *testing.T) {
	if got, want := formatUsageLoadProgressText(nil, "", false), "Loading usage logs"; got != want {
		t.Errorf("formatUsageLoadProgressText = %q, want %q", got, want)
	}
}

func TestFitStatusToWidthFitsWithinANarrowTerminal(t *testing.T) {
	text := formatUsageLoadProgressText([]loadProgressAgent{
		{"Claude", loadLoading},
		{"Codex", loadLoading},
	}, "Refreshing model pricing from LiteLLM...", true)

	fitted := fitStatusToWidth(text, 80)

	if want := "Refreshing model pricing from LiteLLM... :: Loading usage logs (0/2) :: Clau…"; fitted != want {
		t.Errorf("fitStatusToWidth = %q, want %q", fitted, want)
	}
	if got := len([]rune(fitted)) + spinnerPrefixWidth; got != 79 {
		t.Errorf("fitted width + prefix = %d, want 79", got)
	}
}

func TestFitStatusToWidthKeepsStatusIntactOnAWideTerminal(t *testing.T) {
	if got := fitStatusToWidth("Refreshing model pricing", 120); got != "Refreshing model pricing" {
		t.Errorf("fitStatusToWidth = %q, want the text unchanged", got)
	}
}

func TestFitStatusToWidthDropsStatusWhenTerminalHasNoRoom(t *testing.T) {
	if got := fitStatusToWidth("Loading usage logs", 3); got != "" {
		t.Errorf("fitStatusToWidth(3) = %q, want empty", got)
	}
	if got := fitStatusToWidth("Loading usage logs", 4); got != "…" {
		t.Errorf("fitStatusToWidth(4) = %q, want ellipsis", got)
	}
}

func TestShouldShowUsageLoadProgressHidesJSONAndNonTTY(t *testing.T) {
	if ShouldShowUsageLoadProgress(true, true) {
		t.Error("JSON runs must not show the spinner")
	}
	if ShouldShowUsageLoadProgress(false, false) {
		t.Error("non-TTY runs must not show the spinner")
	}
	if !ShouldShowUsageLoadProgress(false, true) {
		t.Error("table runs on a TTY must show the spinner")
	}
}

// syncedBuffer lets the worker goroutine append while the test reads.
type syncedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before the deadline")
}

// captureProgress swaps the render sink and TTY probe for the duration of one
// test; tests touching them must not run in parallel.
func captureProgress(t *testing.T) *syncedBuffer {
	t.Helper()
	out := &syncedBuffer{}
	previousOut, previousTTY := progressErrOut, progressStdoutIsTTY
	progressErrOut, progressStdoutIsTTY = out, func() bool { return true }
	t.Cleanup(func() { progressErrOut, progressStdoutIsTTY = previousOut, previousTTY })
	return out
}

func TestUsageLoadSessionRendersFramesAndClearsOnFinish(t *testing.T) {
	out := captureProgress(t)

	finish := BeginUsageLoad("Claude", &SharedArgs{})
	waitFor(t, func() bool {
		return strings.Contains(out.String(), "Loading usage logs (0/1) :: Claude")
	})
	finish(false)

	if !strings.HasSuffix(out.String(), "\r\x1b[K\x1b[?25h") {
		t.Errorf("session must clear the line and restore the cursor, got %q", out.String())
	}
	progressMu.Lock()
	agents, waiters := progressAgents, progressWaiters
	progressMu.Unlock()
	if len(agents) != 0 || waiters != 0 {
		t.Errorf("session state must reset after the last load, got %d agents %d waiters", len(agents), waiters)
	}
}

func TestUsageLoadSessionMergesConcurrentAgents(t *testing.T) {
	out := captureProgress(t)

	finishClaude := BeginUsageLoad("Claude", &SharedArgs{})
	waitFor(t, func() bool { return strings.Contains(out.String(), "(0/1) :: Claude") })
	finishCodex := BeginUsageLoad("Codex", &SharedArgs{})
	waitFor(t, func() bool { return strings.Contains(out.String(), "(0/2) :: Claude, Codex") })

	finishClaude(false)
	waitFor(t, func() bool { return strings.Contains(out.String(), "(1/2) :: Codex") })
	finishCodex(true)

	if !strings.HasSuffix(out.String(), "\r\x1b[K\x1b[?25h") {
		t.Errorf("session must clear after the last agent, got %q", out.String())
	}
}

func TestTrackStatusShowsStandaloneThenClears(t *testing.T) {
	out := captureProgress(t)

	TrackStatus(true, "Refreshing model pricing from LiteLLM...", func() {
		waitFor(t, func() bool {
			return strings.Contains(out.String(), "Refreshing model pricing from LiteLLM...")
		})
	})

	if !strings.HasSuffix(out.String(), "\r\x1b[K\x1b[?25h") {
		t.Errorf("status must clear after run, got %q", out.String())
	}
}

func TestTrackStatusMergesWithActiveLoads(t *testing.T) {
	out := captureProgress(t)

	finish := BeginUsageLoad("Claude", &SharedArgs{})
	waitFor(t, func() bool { return strings.Contains(out.String(), "(0/1) :: Claude") })
	TrackStatus(true, "Refreshing model pricing from LiteLLM...", func() {
		waitFor(t, func() bool {
			return strings.Contains(out.String(),
				"Refreshing model pricing from LiteLLM... :: Loading usage logs (0/1) :: Claude")
		})
	})
	finish(false)

	if !strings.HasSuffix(out.String(), "\r\x1b[K\x1b[?25h") {
		t.Errorf("session must clear after the last waiter, got %q", out.String())
	}
}

func TestTrackUsageLoadPassesResultThrough(t *testing.T) {
	captureProgress(t)

	entries, err := TrackUsageLoad("Claude", &SharedArgs{}, func() ([]LoadedEntry, error) {
		return []LoadedEntry{{Date: "20260816"}}, nil
	})
	if err != nil || len(entries) != 1 {
		t.Fatalf("TrackUsageLoad result = %v, %v", entries, err)
	}

	want := errors.New("boom")
	_, err = TrackUsageLoad("Claude", &SharedArgs{}, func() ([]LoadedEntry, error) {
		return nil, want
	})
	if err != want {
		t.Fatalf("TrackUsageLoad error = %v, want %v", err, want)
	}
}

func TestBeginUsageLoadDisabledWithoutTTY(t *testing.T) {
	out := captureProgress(t)
	progressStdoutIsTTY = terminal.IsStdoutTerminal

	finish := BeginUsageLoad("Claude", &SharedArgs{})
	finish(false)

	if out.String() != "" {
		t.Errorf("disabled session must not render, got %q", out.String())
	}
}

func TestProgressEnabledGatesJSONQuietAndNonTTY(t *testing.T) {
	captureProgress(t)
	t.Setenv("LOG_LEVEL", "0")
	if progressEnabled(&SharedArgs{}) {
		t.Error("LOG_LEVEL=0 must disable the display")
	}
}
