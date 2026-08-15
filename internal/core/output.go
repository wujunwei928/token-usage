package core

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/terminal"
)

// UsageCompactWidthThreshold is the TTY width below which usage tables compact.
const UsageCompactWidthThreshold = 100

// WantsJSON reports whether the run should emit JSON (or pipe through jq).
func WantsJSON(shared *SharedArgs) bool {
	return shared.JSON || shared.JQ != nil
}

// ShouldUseCompactLayout resolves the compact-mode decision. Non-TTY output
// never auto-compacts; --compact forces it.
func ShouldUseCompactLayout(shared *SharedArgs, isStdoutTTY bool, terminalWidth, compactWidthThreshold int) bool {
	return shared.Compact || (isStdoutTTY && terminalWidth < compactWidthThreshold)
}

// TerminalStyleFromShared builds the render style from flags and env.
func TerminalStyleFromShared(shared *SharedArgs) terminal.TerminalStyle {
	return terminal.TerminalStyle{
		Color:    shared.Color,
		LogLevel: LogLevel(),
		NoColor:  shared.NoColor,
	}
}

// FormatNumber renders an integer with comma grouping.
func FormatNumber(value uint64) string {
	s := fmt.Sprintf("%d", value)
	var out []byte
	for i, ch := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, ch)
	}
	return string(out)
}

// FormatCurrency renders cost as "$X.YZ" (two decimals).
func FormatCurrency(value float64) string {
	return fmt.Sprintf("$%.2f", value)
}

// ShortModelName strips the "claude-"/"anthropic/claude-" prefix and a
// trailing 8-character date-ish segment.
func ShortModelName(model string) string {
	model = strings.TrimPrefix(model, "anthropic/claude-")
	model = strings.TrimPrefix(model, "claude-")
	parts := strings.Split(model, "-")
	if len(parts) >= 3 && len(parts[len(parts)-1]) == 8 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return model
}

// FormatModelsMultiline renders the multi-line Models cell: short names,
// sorted, deduped, each prefixed "- ".
func FormatModelsMultiline(models []string) string {
	short := make([]string, 0, len(models))
	for _, model := range models {
		short = append(short, ShortModelName(model))
	}
	sort.Strings(short)
	deduped := short[:0]
	for i, m := range short {
		if i == 0 || m != short[i-1] {
			deduped = append(deduped, m)
		}
	}
	lines := make([]string, len(deduped))
	for i, m := range deduped {
		lines[i] = "- " + m
	}
	return strings.Join(lines, "\n")
}

// PrintUsageTable renders the standard report table (title box, rows,
// breakdown rows, totals) exactly like the reference implementation.
func PrintUsageTable(title, firstColumn string, rows []UsageSummary, shared *SharedArgs, groupProjects bool, projectAliases *string) error {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "No usage data found.")
		return nil
	}
	terminalWidth := terminal.TerminalWidth()
	isTTY := terminal.IsStdoutTerminal()
	compact := ShouldUseCompactLayout(shared, isTTY, terminalWidth, UsageCompactWidthThreshold)
	includeLastActivity := false
	for i := range rows {
		if rows[i].LastActivity != nil {
			includeLastActivity = true
			break
		}
	}
	terminal.PrintBoxTitle(title, TerminalStyleFromShared(shared))

	var headers []string
	var aligns []terminal.Align
	if compact {
		headers = []string{firstColumn, "Models", "Input", "Output", "Cost (USD)"}
		aligns = []terminal.Align{terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight}
	} else {
		headers = []string{firstColumn, "Models", "Input", "Output", "Cache Create", "Cache Read", "Total Tokens", "Cost (USD)"}
		aligns = []terminal.Align{terminal.AlignLeft, terminal.AlignLeft, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight, terminal.AlignRight}
	}
	if shared.NoCost {
		headers = headers[:len(headers)-1]
		aligns = aligns[:len(aligns)-1]
	}
	if includeLastActivity {
		headers = append(headers, "Last Activity")
		aligns = append(aligns, terminal.AlignLeft)
	}
	table := terminal.NewTable(headers, aligns, TerminalStyleFromShared(shared)).
		WithTerminalWidth(terminalWidth).
		WithDateCompaction(true)
	aliases := ParseProjectAliases(projectAliases)
	currentProject := ""
	haveCurrentProject := false
	for i := range rows {
		row := &rows[i]
		if groupProjects && row.Project != nil && (!haveCurrentProject || currentProject != *row.Project) {
			if haveCurrentProject {
				table.Separator()
			}
			table.Push(projectHeaderRow(table.ColumnCount(), FormatProjectName(*row.Project, aliases), shared))
			currentProject = *row.Project
			haveCurrentProject = true
		}
		label := ""
		switch {
		case row.Date != nil:
			label = *row.Date
		case row.Month != nil:
			label = *row.Month
		case row.Week != nil:
			label = *row.Week
		case row.SessionID != nil:
			label = *row.SessionID
		}
		models := FormatModelsMultiline(row.ModelsUsed)
		totalTokens := row.TotalTokens()
		var values []string
		if compact {
			values = []string{
				label,
				models,
				FormatNumber(row.InputTokens),
				FormatNumber(row.OutputTokens),
				FormatCurrency(row.TotalCost),
			}
		} else {
			values = []string{
				label,
				models,
				FormatNumber(row.InputTokens),
				FormatNumber(row.OutputTokens),
				FormatNumber(row.CacheCreationTokens),
				FormatNumber(row.CacheReadTokens),
				FormatNumber(totalTokens),
				FormatCurrency(row.TotalCost),
			}
		}
		if shared.NoCost {
			values = values[:len(values)-1]
		}
		if includeLastActivity {
			la := ""
			if row.LastActivity != nil {
				la = truncateRFC3339ToDate(*row.LastActivity)
			}
			values = append(values, la)
		}
		table.Push(values)
		if shared.Breakdown {
			pushBreakdownRows(table, row, compact, includeLastActivity, shared)
		}
	}

	input, output, cacheCreate, cacheRead, extra, totalCost := totalsParts(rows)
	totalTokens := input + output + cacheCreate + cacheRead + extra
	table.Separator()
	style := TerminalStyleFromShared(shared)
	var totalRow []string
	if compact {
		totalRow = []string{
			terminal.Colorize(style, "Total", terminal.ColorYellow),
			"",
			terminal.Colorize(style, FormatNumber(input), terminal.ColorYellow),
			terminal.Colorize(style, FormatNumber(output), terminal.ColorYellow),
			terminal.Colorize(style, FormatCurrency(totalCost), terminal.ColorYellow),
		}
	} else {
		totalRow = []string{
			terminal.Colorize(style, "Total", terminal.ColorYellow),
			"",
			terminal.Colorize(style, FormatNumber(input), terminal.ColorYellow),
			terminal.Colorize(style, FormatNumber(output), terminal.ColorYellow),
			terminal.Colorize(style, FormatNumber(cacheCreate), terminal.ColorYellow),
			terminal.Colorize(style, FormatNumber(cacheRead), terminal.ColorYellow),
			terminal.Colorize(style, FormatNumber(totalTokens), terminal.ColorYellow),
			terminal.Colorize(style, FormatCurrency(totalCost), terminal.ColorYellow),
		}
	}
	if shared.NoCost {
		totalRow = totalRow[:len(totalRow)-1]
	}
	if includeLastActivity {
		totalRow = append(totalRow, "")
	}
	table.Push(totalRow)
	if err := table.Print(os.Stdout); err != nil {
		return err
	}
	PrintMissingPricingWarnings(rows, shared.OfflineEffective())
	if compact {
		fmt.Fprintln(os.Stderr, "\nRunning in Compact Mode")
		fmt.Fprintln(os.Stderr, "Expand terminal width to see cache metrics and total tokens")
	}
	return nil
}

func totalsParts(rows []UsageSummary) (input, output, cacheCreate, cacheRead, extra uint64, totalCost float64) {
	for i := range rows {
		input += rows[i].InputTokens
		output += rows[i].OutputTokens
		cacheCreate += rows[i].CacheCreationTokens
		cacheRead += rows[i].CacheReadTokens
		extra += rows[i].ExtraTotalTokens
		totalCost += rows[i].TotalCost
	}
	return
}

func projectHeaderRow(columnCount int, project string, shared *SharedArgs) []string {
	row := make([]string, columnCount)
	if columnCount > 0 {
		row[0] = terminal.Colorize(TerminalStyleFromShared(shared), "Project: "+project, terminal.ColorBlue)
	}
	return row
}

func pushBreakdownRows(table *terminal.SimpleTable, row *UsageSummary, compact, includeLastActivity bool, shared *SharedArgs) {
	style := TerminalStyleFromShared(shared)
	for i := range row.ModelBreakdowns {
		breakdown := &row.ModelBreakdowns[i]
		total := breakdown.InputTokens + breakdown.OutputTokens + breakdown.CacheCreationTokens + breakdown.CacheReadTokens
		prefix := terminal.Colorize(style, "  └─ "+ShortModelName(breakdown.ModelName), terminal.ColorGrey)
		var values []string
		if compact {
			values = []string{
				prefix,
				"",
				terminal.Colorize(style, FormatNumber(breakdown.InputTokens), terminal.ColorGrey),
				terminal.Colorize(style, FormatNumber(breakdown.OutputTokens), terminal.ColorGrey),
				terminal.Colorize(style, FormatCurrency(breakdown.Cost), terminal.ColorGrey),
			}
		} else {
			values = []string{
				prefix,
				"",
				terminal.Colorize(style, FormatNumber(breakdown.InputTokens), terminal.ColorGrey),
				terminal.Colorize(style, FormatNumber(breakdown.OutputTokens), terminal.ColorGrey),
				terminal.Colorize(style, FormatNumber(breakdown.CacheCreationTokens), terminal.ColorGrey),
				terminal.Colorize(style, FormatNumber(breakdown.CacheReadTokens), terminal.ColorGrey),
				terminal.Colorize(style, FormatNumber(total), terminal.ColorGrey),
				terminal.Colorize(style, FormatCurrency(breakdown.Cost), terminal.ColorGrey),
			}
		}
		if shared.NoCost {
			values = values[:len(values)-1]
		}
		if includeLastActivity {
			values = append(values, "")
		}
		table.Push(values)
	}
}

// PrintMissingPricingWarnings emits the deduped missing-pricing warnings.
func PrintMissingPricingWarnings(rows []UsageSummary, offline bool) {
	var models []string
	for i := range rows {
		for b := range rows[i].ModelBreakdowns {
			if rows[i].ModelBreakdowns[b].MissingPricing {
				models = append(models, rows[i].ModelBreakdowns[b].ModelName)
			}
		}
	}
	PrintMissingPricingWarningsForModels(models, offline)
}

// PrintMissingPricingWarningsForModels warns once per distinct model.
func PrintMissingPricingWarningsForModels(models []string, offline bool) {
	set := map[string]struct{}{}
	for _, m := range models {
		set[m] = struct{}{}
	}
	sorted := make([]string, 0, len(set))
	for m := range set {
		sorted = append(sorted, m)
	}
	sort.Strings(sorted)
	for _, model := range sorted {
		if offline {
			fmt.Fprintf(os.Stderr, "WARN  Missing embedded pricing for %s; cost excludes this model. Run without --offline or update ccusage after pricing is added.\n", model)
		} else {
			fmt.Fprintf(os.Stderr, "WARN  Missing pricing for %s; cost excludes this model. Update pricing or run again after LiteLLM has the model.\n", model)
		}
	}
}

func truncateRFC3339ToDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// ParseProjectAliases parses "a=X,b=Y" into a lookup map, skipping pairs
// with an empty key or value.
func ParseProjectAliases(raw *string) map[string]string {
	aliases := map[string]string{}
	if raw == nil {
		return aliases
	}
	for _, pair := range strings.Split(*raw, ",") {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			aliases[key] = value
		}
	}
	return aliases
}

// FormatProjectName resolves a project name through aliases then shortening.
func FormatProjectName(project string, aliases map[string]string) string {
	if alias, ok := aliases[project]; ok {
		return alias
	}
	parsed := parseProjectName(project)
	if alias, ok := aliases[parsed]; ok {
		return alias
	}
	return parsed
}

func parseProjectName(project string) string {
	if project == "" || project == "unknown" {
		return "Unknown Project"
	}
	cleaned := project
	switch {
	case isWindowsUsersPath(cleaned):
		segments := strings.Split(cleaned, "\\")
		if index := indexOf(segments, "Users"); index >= 0 {
			if index+3 < len(segments) {
				cleaned = strings.Join(segments[index+3:], "-")
			} else if index+2 < len(segments) {
				cleaned = strings.Join(segments[index+2:], "-")
			}
		}
	case strings.HasPrefix(cleaned, "-Users-") || strings.HasPrefix(cleaned, "/Users/"):
		separator := "-"
		if strings.HasPrefix(cleaned, "/Users/") {
			separator = "/"
		}
		var segments []string
		for _, segment := range strings.Split(cleaned, separator) {
			if segment != "" {
				segments = append(segments, segment)
			}
		}
		if index := indexOf(segments, "Users"); index >= 0 {
			if index+3 < len(segments) {
				cleaned = strings.Join(segments[index+3:], "-")
			} else if index+2 < len(segments) {
				cleaned = strings.Join(segments[index+2:], "-")
			}
		}
	default:
		cleaned = strings.Trim(cleaned, "/\\-")
	}
	parts := strings.Split(cleaned, "-")
	if len(parts) >= 5 && isHexDashDot(cleaned) {
		cleaned = strings.Join(parts[maxInt(0, len(parts)-2):], "-")
	}
	if main, _, found := strings.Cut(cleaned, "--"); found {
		cleaned = main
	}
	if strings.Contains(cleaned, "-") && len(cleaned) > 20 {
		var meaningful []string
		for _, segment := range strings.Split(cleaned, "-") {
			if len(segment) > 2 && !isExcludedSegment(segment) {
				meaningful = append(meaningful, segment)
			}
		}
		if len(meaningful) >= 2 {
			lastTwo := strings.Join(meaningful[len(meaningful)-2:], "-")
			if len(lastTwo) >= 6 {
				cleaned = lastTwo
			} else if len(meaningful) >= 3 {
				cleaned = strings.Join(meaningful[len(meaningful)-3:], "-")
			}
		}
	}
	cleaned = strings.Trim(cleaned, "/\\-")
	if cleaned == "" {
		return project
	}
	return cleaned
}

func isWindowsUsersPath(project string) bool {
	b := []byte(project)
	return (len(b) >= 10 && b[1] == ':' && b[2] == '\\' && strings.HasPrefix(string(b[3:]), "Users\\")) ||
		strings.HasPrefix(project, "\\Users\\")
}

func indexOf(list []string, want string) int {
	for i, item := range list {
		if item == want {
			return i
		}
	}
	return -1
}

func isHexDashDot(s string) bool {
	for _, ch := range s {
		if !isASCIIHexDigit(byte(ch)) && ch != '-' && ch != '.' {
			return false
		}
	}
	return true
}

func isASCIIHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isExcludedSegment(segment string) bool {
	switch strings.ToLower(segment) {
	case "dev", "development", "feat", "feature", "fix", "bug", "test",
		"staging", "prod", "production", "main", "master", "branch":
		return true
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// DebugLog prints a debug line to stderr when --debug is set.
func DebugLog(shared *SharedArgs, message string) {
	if shared.Debug {
		fmt.Fprintln(os.Stderr, message)
	}
}
