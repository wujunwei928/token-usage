package statusline

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/adapter/claude"
	"github.com/wujunwei/ccusage-go/internal/blocks"
	"github.com/wujunwei/ccusage-go/internal/core"
	"github.com/wujunwei/ccusage-go/internal/terminal"
)

const millisPerMinute = 60 * 1000

// renderStatusline computes the status line for one hook invocation. Like the
// reference render_statusline, data-loading failures degrade gracefully:
// session cost becomes N/A, today cost 0, and blocks are skipped.
func renderStatusline(hook *Hook, args *Args, shared *core.SharedArgs, now int64) (string, error) {
	var sessionCost *float64
	switch args.CostSource {
	case CostSourceCc:
		if hook.Cost != nil {
			cost := hook.Cost.TotalCostUSD
			sessionCost = &cost
		}
	case CostSourceCcusage:
		if cost, err := calculateSessionCost(hook.SessionID, shared); err == nil {
			sessionCost = &cost
		}
	case CostSourceAuto:
		if hook.Cost != nil {
			cost := hook.Cost.TotalCostUSD
			sessionCost = &cost
		} else if cost, err := calculateSessionCost(hook.SessionID, shared); err == nil {
			sessionCost = &cost
		}
	}

	var ccusageCost, ccCost *float64
	if args.CostSource == CostSourceBoth {
		if cost, err := calculateSessionCost(hook.SessionID, shared); err == nil {
			ccusageCost = &cost
		}
		if hook.Cost != nil {
			cost := hook.Cost.TotalCostUSD
			ccCost = &cost
		}
	}

	todayCost := statuslineTodayCost(args, shared, now)

	blockInfo, burnRateInfo := statuslineBlockInfo(args, shared, now)

	var contextInfo *string
	if hook.ContextWindow != nil {
		formatted := formatStatuslineContext(
			hook.ContextWindow.TotalInputTokens,
			hook.ContextWindow.ContextWindowSize, args, shared)
		contextInfo = &formatted
	} else if context := calculateContextTokensFromTranscript(
		hook.TranscriptPath, hook.Model.ID, shared.OfflineEffective(), shared); context != nil {
		formatted := formatStatuslineContext(
			context.TotalInputTokens, context.ContextWindowSize, args, shared)
		contextInfo = &formatted
	}

	var sessionDisplay string
	if args.CostSource == CostSourceBoth {
		cc := "N/A"
		if ccCost != nil {
			cc = core.FormatCurrency(*ccCost)
		}
		ccusage := "N/A"
		if ccusageCost != nil {
			ccusage = core.FormatCurrency(*ccusageCost)
		}
		sessionDisplay = fmt.Sprintf("(%s cc / %s ccusage)", cc, ccusage)
	} else if sessionCost != nil {
		sessionDisplay = core.FormatCurrency(*sessionCost)
	} else {
		sessionDisplay = "N/A"
	}

	modelLabel := resolveModelLabel(args.ModelLabelAliases, hook.Model.DisplayName)
	modelSegment := formatModelSegment(modelLabel, hook.Effort)

	contextDisplay := "N/A"
	if contextInfo != nil {
		contextDisplay = *contextInfo
	}
	return fmt.Sprintf(
		"🤖 %s | 💰 %s session / %s today / %s%s | 🧠 %s",
		modelSegment,
		sessionDisplay,
		core.FormatCurrency(todayCost),
		blockInfo,
		burnRateInfo,
		contextDisplay,
	), nil
}

// calculateSessionCost sums ccusage-computed costs for the hook session.
func calculateSessionCost(sessionID string, shared *core.SharedArgs) (float64, error) {
	entries, err := claude.LoadEntries(claude.LoadOptions{Shared: shared})
	if err != nil {
		return 0, err
	}
	total := negZeroSum()
	for i := range entries {
		entry := &entries[i]
		if (entry.Data.SessionID != nil && *entry.Data.SessionID == sessionID) ||
			entry.SessionID == sessionID {
			total += entry.Cost
		}
	}
	return total, nil
}

// statuslineTodayCost sums today's entry costs in the requested timezone.
// Load failures degrade to 0, matching `.unwrap_or(0.0)`.
func statuslineTodayCost(args *Args, shared *core.SharedArgs, now int64) float64 {
	todayShared := statuslineTodayShared(args, shared, now)
	entries, err := claude.LoadEntries(claude.LoadOptions{Shared: todayShared})
	if err != nil {
		return 0
	}
	since := ""
	if todayShared.Since != nil {
		since = *todayShared.Since
	}
	total := negZeroSum()
	for i := range entries {
		if strings.ReplaceAll(entries[i].Date, "-", "") == since {
			total += entries[i].Cost
		}
	}
	return total
}

func statuslineTodayShared(args *Args, shared *core.SharedArgs, now int64) *core.SharedArgs {
	today := strings.ReplaceAll(core.FormatDateTZ(now, core.ParseTZ(args.Timezone)), "-", "")
	t := today
	return &core.SharedArgs{
		Since:    &t,
		Until:    &today,
		Offline:  shared.Offline,
		NoOffline: shared.NoOffline,
		Timezone: args.Timezone,
	}
}

// statuslineBlockInfo renders the active block segment plus burn-rate hint.
func statuslineBlockInfo(args *Args, shared *core.SharedArgs, now int64) (string, string) {
	entries, err := claude.LoadEntries(claude.LoadOptions{Shared: shared})
	if err != nil {
		return "No active block", ""
	}
	blockList := identifyBlocksWithClock(entries, now)
	var active *blocks.Block
	for i := range blockList {
		if blockList[i].IsActive && !blockList[i].IsGap {
			active = &blockList[i]
			break
		}
	}
	if active == nil {
		return "No active block", ""
	}
	remaining := (active.EndTime - now) / millisPerMinute
	burn := ""
	if rate := blocks.CalculateBurnRate(active); rate != nil {
		segments := []string{fmt.Sprintf("%s/hr", core.FormatCurrency(rate.CostPerHour))}
		emoji, text := "🟢", "Normal"
		switch {
		case rate.TokensPerMinuteForIndicator < 2000.0:
			emoji, text = "🟢", "Normal"
		case rate.TokensPerMinuteForIndicator < 5000.0:
			emoji, text = "⚠️", "Moderate"
		default:
			emoji, text = "🚨", "High"
		}
		if args.VisualBurnRate == BurnRateEmoji || args.VisualBurnRate == BurnRateEmojiText {
			segments = append(segments, emoji)
		}
		if args.VisualBurnRate == BurnRateText || args.VisualBurnRate == BurnRateEmojiText {
			segments = append(segments, fmt.Sprintf("(%s)", text))
		}
		burn = fmt.Sprintf(" | 🔥 %s", strings.Join(segments, " "))
	}
	return fmt.Sprintf("%s block (%s)",
		core.FormatCurrency(active.CostUSD),
		blocks.FormatRemainingTime(remaining)), burn
}

// identifyBlocksWithClock runs block identification with the statusline clock
// so injected tests see deterministic active blocks.
func identifyBlocksWithClock(entries []core.LoadedEntry, now int64) []blocks.Block {
	restore := blocks.SwapNowFunc(func() int64 { return now })
	defer restore()
	return blocks.IdentifySessionBlocks(entries, blocks.DefaultSessionDurationHours)
}

// resolveModelLabel maps a display name through the configured aliases.
func resolveModelLabel(aliases map[string]string, displayName string) string {
	if aliases != nil {
		if alias, ok := aliases[displayName]; ok {
			return alias
		}
	}
	return displayName
}

// formatModelSegment appends the reasoning effort level when reported.
func formatModelSegment(modelLabel string, effort *HookEffort) string {
	if effort != nil && effort.Level != "" {
		return fmt.Sprintf("%s (%s)", modelLabel, effort.Level)
	}
	return modelLabel
}

func formatStatuslineContext(inputTokens, contextLimit uint64, args *Args, shared *core.SharedArgs) string {
	percentage := uint64(0)
	if contextLimit != 0 {
		// Rust's f64::round: half away from zero (Go's math.Round matches).
		percentage = uint64(math.Round(float64(inputTokens) / float64(contextLimit) * 100.0))
	}
	return fmt.Sprintf("%s (%s)",
		core.FormatNumber(inputTokens),
		colorize(shared, fmt.Sprintf("%d%%", percentage), statuslineContextColor(percentage, args)))
}

func statuslineContextColor(percentage uint64, args *Args) terminal.Color {
	if percentage < args.ContextLowThreshold {
		return terminal.ColorGreen
	}
	if percentage < args.ContextMediumThresh {
		return terminal.ColorYellow
	}
	return terminal.ColorRed
}

func colorize(shared *core.SharedArgs, value string, c terminal.Color) string {
	return terminal.Colorize(terminal.TerminalStyle{
		Color:    shared.Color,
		LogLevel: core.LogLevel(),
		NoColor:  shared.NoColor,
	}, value, c)
}

// calculateContextTokensFromTranscript derives context usage from the last
// assistant entry in the transcript, reversing from the file end.
func calculateContextTokensFromTranscript(path string, modelID *string, offline bool, shared *core.SharedArgs) *HookContext {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := splitLinesReverse(content)
	var pricing *core.PricingMap
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		value, ok := parseTranscriptLine(line)
		if !ok || !value.typeIsAssistant {
			continue
		}
		usage := value.usage
		if usage == nil || !usage.inputPresent {
			continue
		}
		contextWindowSize := uint64(200_000)
		if modelID != nil && *modelID != "" {
			if pricing == nil {
				pricing = core.LoadWithOverrides(offline, false, nil)
			}
			if limit, ok := pricing.ContextLimit(*modelID); ok {
				contextWindowSize = limit
			}
		}
		return &HookContext{
			TotalInputTokens:   usage.inputTokens + usage.cacheCreation + usage.cacheRead,
			ContextWindowSize:  contextWindowSize,
		}
	}
	return nil
}

func splitLinesReverse(content []byte) []string {
	lines := strings.Split(string(content), "\n")
	out := make([]string, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		out = append(out, lines[i])
	}
	return out
}

type transcriptUsage struct {
	inputTokens   uint64
	inputPresent  bool
	cacheCreation uint64
	cacheRead     uint64
}

type transcriptLine struct {
	typeIsAssistant bool
	usage           *transcriptUsage
}

// parseTranscriptLine mirrors the reference's serde_json::Value based scan:
// invalid JSON lines are skipped, "type" must be an assistant string, and the
// usage object needs an input_tokens integer (Value::as_u64 semantics).
func parseTranscriptLine(line string) (transcriptLine, bool) {
	var root map[string]any
	if err := json.Unmarshal([]byte(line), &root); err != nil {
		return transcriptLine{}, false
	}
	out := transcriptLine{}
	if t, ok := root["type"].(string); ok && t == "assistant" {
		out.typeIsAssistant = true
	}
	message, _ := root["message"].(map[string]any)
	if message == nil {
		return out, true
	}
	usageRaw, present := message["usage"]
	if !present {
		return out, true
	}
	usageMap, ok := usageRaw.(map[string]any)
	if !ok {
		return out, true
	}
	usage := &transcriptUsage{}
	input, ok := asU64(usageMap["input_tokens"])
	if !ok {
		return out, true
	}
	usage.inputTokens = input
	usage.inputPresent = true
	if v, ok := asU64(usageMap["cache_creation_input_tokens"]); ok {
		usage.cacheCreation = v
	}
	if v, ok := asU64(usageMap["cache_read_input_tokens"]); ok {
		usage.cacheRead = v
	}
	out.usage = usage
	return out, true
}

// asU64 mirrors serde_json Value::as_u64: only non-negative integers count.
func asU64(v any) (uint64, bool) {
	f, ok := v.(float64)
	if !ok || f < 0 || f > 18446744073709551615.0 || f != math.Trunc(f) {
		return 0, false
	}
	return uint64(f), true
}
