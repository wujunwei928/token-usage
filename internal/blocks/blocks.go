// Package blocks identifies and renders 5-hour billing blocks (the Block
// concept from CONTEXT.md): windows cut at floor-to-hour boundaries that split
// when usage exceeds the duration or goes idle longer than it.
package blocks

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/wujunwei/ccusage-go/internal/core"
	"github.com/wujunwei/ccusage-go/internal/terminal"
)

// WarningThreshold is the share of the token limit past which a block warns.
const WarningThreshold = 0.8

// CompactWidthThreshold is the terminal width below which blocks tables compact.
const CompactWidthThreshold = 120

// DefaultSessionDurationHours is the standard billing window length.
const DefaultSessionDurationHours = 5.0

// DefaultRecentDays is the --recent window in days.
const DefaultRecentDays = 3

// nowFunc is swappable for tests that need deterministic active blocks.
var nowFunc = func() int64 { return time.Now().UnixMilli() }

// SwapNowFunc replaces the package clock (block activity detection) and
// returns a restore function. Other packages (statusline) use it to keep
// block identification on their own injected clock.
func SwapNowFunc(f func() int64) func() {
	prev := nowFunc
	nowFunc = f
	return func() { nowFunc = prev }
}

// Block is one billing window (or a gap placeholder between them).
type Block struct {
	ID                 string
	StartTime          int64
	EndTime            int64
	ActualEndTime      *int64
	IsActive           bool
	IsGap              bool
	Entries            []core.LoadedEntry
	TokenCounts        core.TokenCounts
	CostUSD            float64
	Models             []string
	UsageLimitResetTime *int64
}

// BurnRate describes consumption inside an active block.
type BurnRate struct {
	TokensPerMinute             float64
	TokensPerMinuteForIndicator float64
	CostPerHour                 float64
}

// Projection extrapolates an active block to its end.
type Projection struct {
	TotalTokens      uint64
	TotalCost        float64
	RemainingMinutes uint64
}

// IdentifySessionBlocks sorts entries and cuts them into billing blocks.
func IdentifySessionBlocks(entries []core.LoadedEntry, sessionDurationHours float64) []Block {
	if len(entries) == 0 {
		return nil
	}
	sessionDuration := int64(sessionDurationHours * float64(millisPerHour))
	sorted := append([]core.LoadedEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })
	now := nowFunc()
	var blocks []Block
	var currentStart *int64
	var currentEntries []core.LoadedEntry

	for i := range sorted {
		entry := sorted[i]
		if currentStart != nil {
			last := *currentStart
			if len(currentEntries) > 0 {
				last = currentEntries[len(currentEntries)-1].Timestamp
			}
			sinceStart := entry.Timestamp - *currentStart
			sinceLast := entry.Timestamp - last
			if sinceStart > sessionDuration || sinceLast > sessionDuration {
				blocks = append(blocks, createBlock(*currentStart, currentEntries, now, sessionDuration))
				currentEntries = nil
				if sinceLast > sessionDuration {
					blocks = append(blocks, createGapBlock(last, entry.Timestamp, sessionDuration))
				}
				floored := floorToHour(entry.Timestamp)
				currentStart = &floored
			}
		} else {
			floored := floorToHour(entry.Timestamp)
			currentStart = &floored
		}
		currentEntries = append(currentEntries, entry)
	}
	if currentStart != nil && len(currentEntries) > 0 {
		blocks = append(blocks, createBlock(*currentStart, currentEntries, now, sessionDuration))
	}
	return blocks
}

const (
	millisPerMinute = 60 * 1000
	millisPerHour   = 60 * millisPerMinute
	millisPerDay    = 24 * millisPerHour
)

func floorToHour(ts int64) int64 {
	return ts - ts%millisPerHour
}

func createBlock(start int64, entries []core.LoadedEntry, now, duration int64) Block {
	end := start + duration
	var actualEnd *int64
	isActive := false
	if len(entries) > 0 {
		last := entries[len(entries)-1].Timestamp
		actualEnd = &last
		isActive = now-last < duration && now < end
	}
	var counts core.TokenCounts
	cost := 0.0
	var models []string
	seen := map[string]struct{}{}
	var usageLimitResetTime *int64
	for i := range entries {
		counts.AddUsage(entries[i].Data.Message.Usage)
		cost += entries[i].Cost
		if entries[i].Model != nil {
			model := core.ResolveModelName(*entries[i].Model)
			if _, ok := seen[model]; !ok {
				seen[model] = struct{}{}
				models = append(models, model)
			}
		}
		if usageLimitResetTime == nil {
			usageLimitResetTime = entries[i].UsageLimitResetTime
		}
	}
	return Block{
		ID:                  core.FormatRFC3339Millis(start),
		StartTime:           start,
		EndTime:             end,
		ActualEndTime:       actualEnd,
		IsActive:            isActive,
		IsGap:               false,
		Entries:             entries,
		TokenCounts:         counts,
		CostUSD:             cost,
		Models:              models,
		UsageLimitResetTime: usageLimitResetTime,
	}
}

func createGapBlock(last, next, duration int64) Block {
	start := last + duration
	return Block{
		ID:        "gap-" + core.FormatRFC3339Millis(start),
		StartTime: start,
		EndTime:   next,
		IsGap:     true,
	}
}

// FilterBlocksByDate keeps blocks whose local start date is inside the window.
func FilterBlocksByDate(blocks []Block, shared *core.SharedArgs) []Block {
	if shared.Since == nil && shared.Until == nil {
		return blocks
	}
	out := make([]Block, 0, len(blocks))
	for _, block := range blocks {
		date := core.FormatDateTZ(block.StartTime, core.ParseTZ(shared.Timezone))
		if core.DateWithinRange(strings.ReplaceAll(date, "-", ""), shared.Since, shared.Until) {
			out = append(out, block)
		}
	}
	return out
}

// SortBlocks orders blocks by start time and direction.
func SortBlocks(blocks []Block, order core.SortOrder) []Block {
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].StartTime < blocks[j].StartTime })
	if order == core.OrderDesc {
		for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
			blocks[i], blocks[j] = blocks[j], blocks[i]
		}
	}
	return blocks
}

// CalculateBurnRate measures consumption per minute inside a block.
func CalculateBurnRate(block *Block) *BurnRate {
	if len(block.Entries) == 0 || block.IsGap {
		return nil
	}
	first := block.Entries[0].Timestamp
	last := block.Entries[len(block.Entries)-1].Timestamp
	durationMinutes := float64(last-first) / float64(millisPerMinute)
	if durationMinutes <= 0 {
		return nil
	}
	totalTokens := float64(block.TokenCounts.Total())
	nonCache := float64(block.TokenCounts.InputTokens + block.TokenCounts.OutputTokens)
	return &BurnRate{
		TokensPerMinute:             totalTokens / durationMinutes,
		TokensPerMinuteForIndicator: nonCache / durationMinutes,
		CostPerHour:                 block.CostUSD / durationMinutes * 60.0,
	}
}

// ProjectBlockUsage extrapolates an active block to its scheduled end.
func ProjectBlockUsage(block *Block) *Projection {
	if !block.IsActive || block.IsGap {
		return nil
	}
	burn := CalculateBurnRate(block)
	if burn == nil {
		return nil
	}
	remainingMinutes := math.Round(float64(block.EndTime-nowFunc()) / float64(millisPerMinute))
	totalTokens := float64(block.TokenCounts.Total()) + burn.TokensPerMinute*remainingMinutes
	totalCost := block.CostUSD + (burn.CostPerHour/60.0)*remainingMinutes
	return &Projection{
		TotalTokens:      uint64(math.Round(totalTokens)),
		TotalCost:        math.Round(totalCost*100.0) / 100.0,
		RemainingMinutes: uint64(remainingMinutes),
	}
}

// ParseTokenLimit resolves --token-limit against the max-tokens baseline.
func ParseTokenLimit(value *string, maxTokens uint64) *uint64 {
	if value == nil || *value == "" || *value == "max" {
		if maxTokens > 0 {
			limit := maxTokens
			return &limit
		}
		return nil
	}
	var parsed uint64
	if _, err := fmt.Sscanf(*value, "%d", &parsed); err != nil {
		return nil
	}
	return &parsed
}

// FormatRemainingTime renders "{h}h {m}m left" style strings.
func FormatRemainingTime(minutes int64) string {
	hours := minutes / 60
	mins := minutes % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm left", hours, mins)
	}
	return fmt.Sprintf("%dm left", mins)
}

func localParts(ts int64) (year, month, day, hour, minute, second int) {
	t := time.UnixMilli(ts)
	return t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second()
}

func hour12(hour int) int {
	h := hour % 12
	if h == 0 {
		return 12
	}
	return h
}

func amPM(hour int) string {
	if hour < 12 {
		return "AM"
	}
	return "PM"
}

func formatLocalBlockStart(ts int64, compact bool) string {
	year, month, day, hour, minute, second := localParts(ts)
	if compact {
		return fmt.Sprintf("%02d/%02d, %02d:%02d %s", month, day, hour12(hour), minute, amPM(hour))
	}
	return fmt.Sprintf("%d/%d/%d, %d:%02d:%02d %s", month, day, year, hour12(hour), minute, second, amPM(hour))
}

func formatLocalBlockEnd(ts int64, compact bool) string {
	_, _, _, hour, minute, _ := localParts(ts)
	if compact {
		return fmt.Sprintf("%02d:%02d %s", hour12(hour), minute, amPM(hour))
	}
	return formatLocalBlockStart(ts, false)
}

func formatBlockTime(block *Block, compact bool) string {
	start := formatLocalBlockStart(block.StartTime, compact)
	if block.IsGap {
		end := formatLocalBlockEnd(block.EndTime, compact)
		duration := (block.EndTime - block.StartTime) / millisPerHour
		if compact {
			return fmt.Sprintf("%s-%s\n(%dh gap)", start, end, duration)
		}
		return fmt.Sprintf("%s - %s (%dh gap)", start, end, duration)
	}
	if block.IsActive {
		now := nowFunc()
		elapsed := (now - block.StartTime) / millisPerMinute
		remaining := (block.EndTime - now) / millisPerMinute
		elapsedHours := elapsed / 60
		elapsedMinutes := ((elapsed % 60) + 60) % 60
		remainingHours := remaining / 60
		remainingMinutes := ((remaining % 60) + 60) % 60
		if compact {
			return fmt.Sprintf("%s\n(%dh%dm/%dh%dm)", start, elapsedHours, elapsedMinutes, remainingHours, remainingMinutes)
		}
		return fmt.Sprintf("%s (%dh %dm elapsed, %dh %dm remaining)",
			start, elapsedHours, elapsedMinutes, remainingHours, remainingMinutes)
	}
	duration := int64(0)
	if block.ActualEndTime != nil {
		duration = (*block.ActualEndTime - block.StartTime) / millisPerMinute
	}
	hours := duration / 60
	minutes := ((duration % 60) + 60) % 60
	if compact {
		if hours > 0 {
			return fmt.Sprintf("%s\n(%dh%dm)", start, hours, minutes)
		}
		return fmt.Sprintf("%s\n(%dm)", start, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%s (%dh %dm)", start, hours, minutes)
	}
	return fmt.Sprintf("%s (%dm)", start, minutes)
}

func formatBlockModels(models []string) string {
	if len(models) == 0 {
		return "-"
	}
	return core.FormatModelsMultiline(models)
}

func styleFor(shared *core.SharedArgs) terminal.TerminalStyle {
	return terminal.TerminalStyle{Color: shared.Color, LogLevel: core.LogLevel(), NoColor: shared.NoColor}
}

func colorize(shared *core.SharedArgs, value string, c terminal.Color) string {
	return terminal.Colorize(styleFor(shared), value, c)
}

// PrintBlocksTable renders the blocks table (gap rows, ACTIVE rows, REMAINING
// and PROJECTED separator rows for active blocks).
func PrintBlocksTable(blocks []Block, tokenLimit *string, maxTokens uint64, shared *core.SharedArgs) error {
	if len(blocks) == 0 {
		fmt.Fprintln(os.Stderr, "No Claude usage data found.")
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	compact := core.ShouldUseCompactLayout(shared, terminal.IsStdoutTerminal(), terminalWidth, CompactWidthThreshold)
	actualLimit := ParseTokenLimit(tokenLimit, maxTokens)
	terminal.PrintBoxTitle("Claude Code Token Usage Report - Session Blocks", styleFor(shared))
	headers := []string{"Block Start", "Duration/Status", "Models", "Tokens"}
	aligns := []terminal.Align{terminal.AlignLeft, terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight}
	if actualLimit != nil && *actualLimit > 0 {
		headers = append(headers, "%")
		aligns = append(aligns, terminal.AlignRight)
	}
	headers = append(headers, "Cost")
	aligns = append(aligns, terminal.AlignRight)
	if shared.NoCost {
		headers = headers[:len(headers)-1]
		aligns = aligns[:len(aligns)-1]
	}
	table := terminal.NewTable(headers, aligns, styleFor(shared)).WithTerminalWidth(terminalWidth)
	for i := range blocks {
		block := &blocks[i]
		if block.IsGap {
			row := []string{
				colorize(shared, formatBlockTime(block, compact), terminal.ColorGrey),
				colorize(shared, "(inactive)", terminal.ColorGrey),
				colorize(shared, "-", terminal.ColorGrey),
				colorize(shared, "-", terminal.ColorGrey),
			}
			if actualLimit != nil && *actualLimit > 0 {
				row = append(row, colorize(shared, "-", terminal.ColorGrey))
			}
			if !shared.NoCost {
				row = append(row, colorize(shared, "-", terminal.ColorGrey))
			}
			table.Push(row)
			continue
		}
		total := block.TokenCounts.Total()
		status := ""
		if block.IsActive {
			status = colorize(shared, "ACTIVE", terminal.ColorGreen)
		}
		row := []string{
			formatBlockTime(block, compact),
			status,
			formatBlockModels(block.Models),
			core.FormatNumber(total),
		}
		if actualLimit != nil && *actualLimit > 0 {
			limit := *actualLimit
			percentage := float64(total) / float64(limit) * 100.0
			percentText := fmt.Sprintf("%.1f%%", percentage)
			if percentage > 100.0 {
				percentText = colorize(shared, percentText, terminal.ColorRed)
			}
			row = append(row, percentText)
		}
		if !shared.NoCost {
			row = append(row, core.FormatCurrency(block.CostUSD))
		}
		table.Push(row)

		if block.IsActive {
			if actualLimit != nil && *actualLimit > 0 {
				limit := *actualLimit
				table.Separator()
				remaining := limit - total
				if total > limit {
					remaining = 0
				}
				remainingPercent := float64(limit-total) / float64(limit) * 100.0
				if remainingPercent < 0 {
					remainingPercent = 0
				}
				remainingText := core.FormatNumber(remaining)
				if remaining == 0 {
					remainingText = colorize(shared, "0", terminal.ColorRed)
				}
				remainingRow := []string{
					colorize(shared, fmt.Sprintf("(assuming %s token limit)", core.FormatNumber(limit)), terminal.ColorGrey),
					colorize(shared, "REMAINING", terminal.ColorBlue),
					"",
					remainingText,
				}
				if remainingPercent > 0 {
					remainingRow = append(remainingRow, fmt.Sprintf("%.1f%%", remainingPercent))
				} else {
					remainingRow = append(remainingRow, colorize(shared, "0.0%", terminal.ColorRed))
				}
				if !shared.NoCost {
					remainingRow = append(remainingRow, "")
				}
				table.Push(remainingRow)
			}
			if projection := ProjectBlockUsage(block); projection != nil {
				table.Separator()
				projectedTokens := core.FormatNumber(projection.TotalTokens)
				if actualLimit != nil && *actualLimit > 0 && projection.TotalTokens > *actualLimit {
					projectedTokens = colorize(shared, projectedTokens, terminal.ColorRed)
				}
				projectedRow := []string{
					colorize(shared, "(assuming current burn rate)", terminal.ColorGrey),
					colorize(shared, "PROJECTED", terminal.ColorYellow),
					"",
					projectedTokens,
				}
				if actualLimit != nil && *actualLimit > 0 {
					percentage := float64(projection.TotalTokens) / float64(*actualLimit) * 100.0
					projectedRow = append(projectedRow, fmt.Sprintf("%.1f%%", percentage))
				}
				if !shared.NoCost {
					projectedRow = append(projectedRow, core.FormatCurrency(projection.TotalCost))
				}
				table.Push(projectedRow)
			}
		}
	}
	return table.Print(os.Stdout)
}

// PrintActiveBlockDetail renders the single-active-block detail view.
func PrintActiveBlockDetail(block *Block, tokenLimit *string, maxTokens uint64, shared *core.SharedArgs) {
	terminal.PrintBoxTitle("Current Session Block Status", styleFor(shared))
	now := nowFunc()
	elapsed := (now - block.StartTime) / millisPerMinute
	remaining := (block.EndTime - now) / millisPerMinute
	fmt.Printf("Block Started:   %s\n", formatUTCSecond(block.StartTime))
	fmt.Printf("Time Elapsed:    %dh %dm\n", elapsed/60, ((elapsed%60)+60)%60)
	fmt.Printf("Time Remaining:  %s\n", colorize(shared,
		fmt.Sprintf("%dh %dm", remaining/60, ((remaining%60)+60)%60), terminal.ColorGreen))
	fmt.Println()
	fmt.Println(colorize(shared, "Current Usage:", terminal.ColorBlue))
	fmt.Printf("  Input Tokens:     %s\n", core.FormatNumber(block.TokenCounts.InputTokens))
	fmt.Printf("  Output Tokens:    %s\n", core.FormatNumber(block.TokenCounts.OutputTokens))
	if !shared.NoCost {
		fmt.Printf("  Total Cost:       %s\n", core.FormatCurrency(block.CostUSD))
	}
	if rate := CalculateBurnRate(block); rate != nil {
		fmt.Println()
		fmt.Println(colorize(shared, "Burn Rate:", terminal.ColorBlue))
		fmt.Printf("  Tokens/minute:    %s\n", core.FormatNumber(uint64(math.Round(rate.TokensPerMinute))))
		if !shared.NoCost {
			fmt.Printf("  Cost/hour:        %s\n", core.FormatCurrency(rate.CostPerHour))
		}
	}
	if projection := ProjectBlockUsage(block); projection != nil {
		fmt.Println()
		fmt.Println(colorize(shared, "Projected Usage (if current rate continues):", terminal.ColorBlue))
		fmt.Printf("  Total Tokens:     %s\n", core.FormatNumber(projection.TotalTokens))
		if !shared.NoCost {
			fmt.Printf("  Total Cost:       %s\n", core.FormatCurrency(projection.TotalCost))
		}
		if limit := ParseTokenLimit(tokenLimit, maxTokens); limit != nil {
			current := block.TokenCounts.Total()
			remainingTokens := int64(*limit) - int64(current)
			if remainingTokens < 0 {
				remainingTokens = 0
			}
			percent := float64(projection.TotalTokens) / float64(*limit) * 100.0
			status := ""
			if projection.TotalTokens > *limit {
				status = colorize(shared, "EXCEEDS LIMIT", terminal.ColorRed)
			} else if float64(projection.TotalTokens) > float64(*limit)*WarningThreshold {
				status = colorize(shared, "WARNING", terminal.ColorYellow)
			} else {
				status = colorize(shared, "OK", terminal.ColorGreen)
			}
			fmt.Println()
			fmt.Println(colorize(shared, "Token Limit Status:", terminal.ColorBlue))
			fmt.Printf("  Limit:            %s tokens\n", core.FormatNumber(*limit))
			fmt.Printf("  Current Usage:    %s (%.1f%%)\n", core.FormatNumber(current),
				float64(current)/float64(*limit)*100.0)
			fmt.Printf("  Remaining:        %s tokens\n", core.FormatNumber(uint64(remainingTokens)))
			fmt.Printf("  Projected Usage:  %.1f%% %s\n", percent, status)
		}
	}
}

func formatUTCSecond(ts int64) string {
	t := time.UnixMilli(ts).UTC()
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d",
		t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
}
