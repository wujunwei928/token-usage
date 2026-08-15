// Package statusline renders the Claude Code statusline hook output: a single
// compact line carrying model, session/today cost, the active billing block
// with burn rate, and context-window usage. It mirrors run_statusline in the
// reference implementation, including the ${TMPDIR}/token-usage-semaphore cache
// keyed on transcript mtime plus a refresh interval.
package statusline

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/wujunwei928/token-usage/internal/core"
)

// VisualBurnRate controls the burn-rate status decoration.
type VisualBurnRate int

// Visual burn rate modes.
const (
	BurnRateOff VisualBurnRate = iota
	BurnRateEmoji
	BurnRateText
	BurnRateEmojiText
)

// ParseVisualBurnRate maps the CLI string onto a VisualBurnRate.
func ParseVisualBurnRate(value string) (VisualBurnRate, bool) {
	switch value {
	case "off":
		return BurnRateOff, true
	case "emoji":
		return BurnRateEmoji, true
	case "text":
		return BurnRateText, true
	case "emoji-text":
		return BurnRateEmojiText, true
	}
	return BurnRateOff, false
}

// CostSource selects where the session cost comes from.
type CostSource int

// Cost sources.
const (
	CostSourceAuto CostSource = iota
	CostSourceTokenUsage
	CostSourceCc
	CostSourceBoth
)

// ParseCostSource maps the CLI string onto a CostSource. "ccusage" stays
// accepted as the legacy spelling of "token-usage" (ADR 0008).
func ParseCostSource(value string) (CostSource, bool) {
	switch value {
	case "auto":
		return CostSourceAuto, true
	case "token-usage", "ccusage":
		return CostSourceTokenUsage, true
	case "cc":
		return CostSourceCc, true
	case "both":
		return CostSourceBoth, true
	}
	return CostSourceAuto, false
}

// Args mirrors the reference StatuslineArgs.
type Args struct {
	Offline              bool
	NoOffline            bool
	VisualBurnRate       VisualBurnRate
	CostSource           CostSource
	Cache                bool
	NoCache              bool
	RefreshInterval      uint64
	ContextLowThreshold  uint64
	ContextMediumThresh  uint64
	Timezone             *string
	Config               *string
	Debug                bool
	ModelLabelAliases    map[string]string
}

// nowFunc is swappable for tests that need a deterministic clock.
var nowFunc = func() int64 { return core.UTCNow() }

// Run executes the statusline pipeline: read the hook JSON from stdin, consult
// the semaphore cache, render, and print the single status line.
func Run(stdin io.Reader, stdout io.Writer, args *Args) error {
	if args.ContextLowThreshold >= args.ContextMediumThresh {
		return &core.CLIError{Message: fmt.Sprintf(
			"Context low threshold (%d) must be less than medium threshold (%d)",
			args.ContextLowThreshold, args.ContextMediumThresh)}
	}

	raw, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	input := strings.TrimSpace(string(raw))
	if input == "" {
		return &core.CLIError{Message: "❌ No input provided"}
	}
	hook, err := ParseHook(input)
	if err != nil {
		return &core.CLIError{Message: "Invalid input format: " + err.Error()}
	}

	// The statusline shared args only carry the resolved offline flag; the
	// reference builds `offline: args.offline && !args.no_offline` and the
	// loaders read it directly.
	shared := statuslineShared(args)

	cacheEnabled := args.Cache && !args.NoCache
	cachePath := statuslineCachePath(hook.SessionID)
	currentMtime := transcriptMtimeMs(hook.TranscriptPath)
	var initialCache *StatuslineCache
	if cacheEnabled {
		initialCache = readStatuslineCache(cachePath)
	}

	if initialCache != nil {
		if output := cachedStatuslineOutput(initialCache, currentMtime, nowMillis(), args.RefreshInterval); output != nil {
			fmt.Fprintln(stdout, *output)
			return nil
		}
	}

	if cacheEnabled {
		markStatuslineCacheUpdating(cachePath, hook, currentMtime, initialCache)
	}

	statusline, renderErr := renderStatusline(hook, args, shared, nowFunc())
	if renderErr == nil {
		fmt.Fprintln(stdout, statusline)
		if cacheEnabled {
			writeStatuslineCache(cachePath, completedCache(hook, statusline, currentMtime, nowMillis()))
		}
		return nil
	}
	if initialCache != nil && initialCache.LastOutput != "" {
		fmt.Fprintln(stdout, initialCache.LastOutput)
	} else {
		fmt.Fprintln(stdout, "❌ Error generating status")
	}
	if cacheEnabled {
		releaseStatuslineCache(cachePath)
	}
	return renderErr
}

// negZeroSum is the empty f64 sum start the reference exhibits (an empty sum
// formats as "$-0.00").
func negZeroSum() float64 { return math.Copysign(0, -1) }

// statuslineShared maps `offline: args.offline && !args.no_offline` onto
// SharedArgs so the loader's OfflineEffective() resolves to exactly that
// value regardless of CCUSAGE_OFFLINE (the reference reads shared.offline
// directly, bypassing the env fallback the report commands use).
func statuslineShared(args *Args) *core.SharedArgs {
	shared := &core.SharedArgs{}
	if args.Offline && !args.NoOffline {
		shared.Offline = true
	} else {
		shared.NoOffline = true
	}
	return shared
}
