package core

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wujunwei928/token-usage/internal/terminal"
)

// This file ports the reference progress display
// (rust/crates/ccusage-core/src/progress.rs): a single-line braille spinner on
// stderr that reports which agent loads are still running. Output never
// touches stdout, so JSON runs and piped output stay clean.

// spinnerFrames animates the braille dot cycle; spinnerInterval is the frame
// cadence.
var spinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 80 * time.Millisecond

// spinnerPrefixWidth is the display width of the frame plus its trailing
// space; fitStatusToWidth leaves one more column free so the cursor never
// sits in a terminal's wrap-pending last column.
const spinnerPrefixWidth = 2

type loadProgressState int

const (
	loadLoading loadProgressState = iota
	loadSucceeded
	loadFailed
)

type loadProgressAgent struct {
	name  string
	state loadProgressState
}

// progressErrOut is the render sink; progressStdoutIsTTY is the terminal
// probe. Both are variables so tests can capture frames and fake a TTY.
var (
	progressErrOut      io.Writer = os.Stderr
	progressStdoutIsTTY           = terminal.IsStdoutTerminal
)

// progressMu guards every field below plus the worker lifecycle. The worker
// goroutine also takes it per frame; nobody holds it across IO or channel
// waits.
var progressMu sync.Mutex

var (
	progressAgents    []loadProgressAgent
	progressStatus    string
	progressHasStatus bool
	progressFrame     int
	progressWaiters   int
	progressStop      chan struct{}
	progressDone      chan struct{}
)

// ShouldShowUsageLoadProgress mirrors should_show_usage_load_progress: table
// runs attached to a terminal show the spinner; JSON runs and redirected
// output never do.
func ShouldShowUsageLoadProgress(json bool, stdoutTTY bool) bool {
	return !json && stdoutTTY
}

// formatUsageLoadProgressText renders "Loading usage logs (done/total) ::
// still-loading agents", prefixed by the standalone status (the pricing
// refresh) when one is active.
func formatUsageLoadProgressText(states []loadProgressAgent, status string, hasStatus bool) string {
	if len(states) == 0 {
		if hasStatus {
			return status
		}
		return "Loading usage logs"
	}
	completed := 0
	var loading []string
	for i := range states {
		if states[i].state == loadLoading {
			loading = append(loading, states[i].name)
		} else {
			completed++
		}
	}
	base := fmt.Sprintf("Loading usage logs (%d/%d)", completed, len(states))
	if len(loading) > 0 {
		base = fmt.Sprintf("%s :: %s", base, strings.Join(loading, ", "))
	}
	if hasStatus {
		return status + " :: " + base
	}
	return base
}

// fitStatusToWidth shortens the status so the spinner line always occupies a
// single row: each frame clears its own line, so a status wide enough to wrap
// would leave the overflow row on screen and read as flicker.
func fitStatusToWidth(text string, width int) string {
	budget := width - spinnerPrefixWidth - 1
	if budget < 0 {
		budget = 0
	}
	return terminal.TruncateToWidth(text, budget)
}

// progressEnabled resolves the track_usage_load gate: quiet logging, JSON
// output, and non-terminal stdout all disable the display.
func progressEnabled(shared *SharedArgs) bool {
	if level := LogLevel(); level != nil && *level == 0 {
		return false
	}
	return ShouldShowUsageLoadProgress(shared != nil && shared.JSON, progressStdoutIsTTY())
}

// TrackUsageLoad wraps one agent's load with progress reporting, mirroring
// track_usage_load: the agent counts as loading until the closure returns,
// then flips to succeeded or failed.
func TrackUsageLoad[T any](agent string, shared *SharedArgs, load func() (T, error)) (T, error) {
	finish := BeginUsageLoad(agent, shared)
	result, err := load()
	finish(err != nil)
	return result, err
}

// BeginUsageLoad registers agent as loading and returns the finish callback
// to invoke with the load outcome (true = failed). Loader entry points pair
// it with a deferred call over their named error; it is a no-op when the
// display is disabled.
func BeginUsageLoad(agent string, shared *SharedArgs) func(failed bool) {
	if !progressEnabled(shared) {
		return func(bool) {}
	}
	progressMu.Lock()
	defer progressMu.Unlock()
	progressSetStateLocked(agent, loadLoading)
	progressWaiters++
	progressStartWorkerLocked()
	return func(failed bool) { progressFinish(agent, failed) }
}

// progressFinish flips the agent to its final state and tears the session
// down when this was the last in-flight load.
func progressFinish(agent string, failed bool) {
	state := loadSucceeded
	if failed {
		state = loadFailed
	}
	progressMu.Lock()
	progressSetStateLocked(agent, state)
	progressWaiters--
	stop, done := progressStopWorkerLocked()
	progressMu.Unlock()
	if stop != nil {
		<-done
	}
}

// TrackStatus shows a standalone status line while run executes, mirroring
// track_status (the LiteLLM pricing refresh). enabled carries the caller's
// log gate; like the reference, JSON runs still show it because it renders
// on stderr only.
func TrackStatus(enabled bool, status string, run func()) {
	if !enabled || !progressStdoutIsTTY() {
		run()
		return
	}
	progressMu.Lock()
	progressStatus, progressHasStatus = status, true
	progressWaiters++
	progressStartWorkerLocked()
	progressMu.Unlock()
	run()
	progressMu.Lock()
	progressStatus, progressHasStatus = "", false
	progressWaiters--
	stop, done := progressStopWorkerLocked()
	progressMu.Unlock()
	if stop != nil {
		<-done
	}
}

func progressSetStateLocked(agent string, state loadProgressState) {
	for i := range progressAgents {
		if progressAgents[i].name == agent {
			progressAgents[i].state = state
			return
		}
	}
	progressAgents = append(progressAgents, loadProgressAgent{agent, state})
}

// progressStartWorkerLocked launches the render loop when the first waiter
// arrives. progressStopWorkerLocked tears it down when the last one leaves:
// the shared state resets synchronously under the lock (so a session started
// right after never sees stale rows), while the final line-clear happens in
// the worker, which alone knows whether it rendered. Both assume progressMu
// is held; the returned channels are nil when the worker keeps running.
func progressStartWorkerLocked() {
	if progressStop != nil {
		return
	}
	progressStop = make(chan struct{})
	progressDone = make(chan struct{})
	stop, done := progressStop, progressDone
	go func() {
		defer close(done)
		rendered := progressRender()
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rendered = progressRender() || rendered
			case <-stop:
				if rendered {
					fmt.Fprint(progressErrOut, "\r\x1b[K\x1b[?25h")
				}
				return
			}
		}
	}()
}

func progressStopWorkerLocked() (chan struct{}, chan struct{}) {
	if progressWaiters > 0 || progressStop == nil {
		return nil, nil
	}
	stop, done := progressStop, progressDone
	progressStop, progressDone = nil, nil
	progressAgents = nil
	progressStatus, progressHasStatus = "", false
	progressFrame = 0
	close(stop)
	return stop, done
}

// progressRender paints one frame and reports whether it wrote anything.
func progressRender() bool {
	progressMu.Lock()
	if len(progressAgents) == 0 && !progressHasStatus {
		progressMu.Unlock()
		return false
	}
	text := fitStatusToWidth(
		formatUsageLoadProgressText(progressAgents, progressStatus, progressHasStatus),
		terminal.TerminalWidth())
	frame := spinnerFrames[progressFrame%len(spinnerFrames)]
	progressFrame++
	progressMu.Unlock()
	fmt.Fprintf(progressErrOut, "\r\x1b[K\x1b[?25l\x1b[36m%s\x1b[39m %s", frame, text)
	return true
}
