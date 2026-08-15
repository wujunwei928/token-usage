package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/gemini"
	"github.com/wujunwei928/token-usage/internal/core"
)

func init() {
	registerAgentCommand(newGeminiCommand)
}

// agentKind mirrors AgentReportKind for the agent subcommands.
type agentKind int

const (
	agentKindDaily agentKind = iota
	agentKindWeekly
	agentKindMonthly
	agentKindSession
)

func (k agentKind) use() string {
	switch k {
	case agentKindMonthly:
		return "monthly"
	case agentKindSession:
		return "session"
	case agentKindWeekly:
		return "weekly"
	default:
		return "daily"
	}
}

// firstColumn returns the report's first table column title.
func (k agentKind) firstColumn() string {
	switch k {
	case agentKindMonthly:
		return "Month"
	case agentKindSession:
		return "Session"
	case agentKindWeekly:
		return "Week"
	default:
		return "Date"
	}
}

// rowsKey returns the JSON array key for the report.
func (k agentKind) rowsKey() string {
	switch k {
	case agentKindMonthly:
		return "monthly"
	case agentKindSession:
		return "sessions"
	case agentKindWeekly:
		return "weekly"
	default:
		return "daily"
	}
}

// periodKey returns the per-row period field for the report.
func (k agentKind) periodKey() string {
	switch k {
	case agentKindMonthly:
		return "month"
	case agentKindSession:
		return "sessionId"
	case agentKindWeekly:
		return "week"
	default:
		return "date"
	}
}

// agentFlagState carries the manually parsed agent flags alongside the shared
// flag state.
type agentFlagState struct {
	openClawPath      string
	openClawPathGiven bool
	version           bool
	help              bool
}

// agentOption describes one CLI option of an agent command, with the
// reference parser's canonical name used in error messages.
type agentOption struct {
	long  string
	short string
	value bool
	apply func(f *sharedFlags, st *agentFlagState, value string) error
}

func agentOptionTable(includeOpenClawPath bool) []agentOption {
	options := []agentOption{
		{long: "--since", short: "-s", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.since = value
			return nil
		}},
		{long: "--until", short: "-u", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.until = value
			return nil
		}},
		{long: "--last", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			parsed, err := strconv.ParseUint(value, 10, 32)
			if err != nil || parsed == 0 {
				return parseErr("Invalid value for --last '%s'. Expected a whole number of periods, 1 or greater.", value)
			}
			f.lastRaw = uint32(parsed)
			return nil
		}},
		{long: "--json", short: "-j", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.JSON = true
			return nil
		}},
		{long: "--mode", short: "-m", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.modeRaw = value
			return nil
		}},
		{long: "--debug", short: "-d", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.Debug = true
			return nil
		}},
		{long: "--debug-samples", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return parseErr("Invalid value for --debug-samples")
			}
			f.shared.DebugSamples = parsed
			return nil
		}},
		{long: "--order", short: "-o", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.orderRaw = value
			return nil
		}},
		{long: "--breakdown", short: "-b", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.Breakdown = true
			return nil
		}},
		{long: "--offline", short: "-O", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.Offline = true
			return nil
		}},
		{long: "--no-offline", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.NoOffline = true
			return nil
		}},
		{long: "--color", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.Color = true
			return nil
		}},
		{long: "--no-color", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.NoColor = true
			return nil
		}},
		{long: "--timezone", short: "-z", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.tzRaw = value
			return nil
		}},
		{long: "--jq", short: "-q", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.jqRaw = value
			return nil
		}},
		{long: "--config", value: true, apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.configRaw = value
			return nil
		}},
		{long: "--compact", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.Compact = true
			return nil
		}},
		{long: "--single-thread", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.SingleThread = true
			return nil
		}},
		{long: "--no-cost", apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			f.shared.NoCost = true
			return nil
		}},
	}
	if includeOpenClawPath {
		options = append(options, agentOption{
			long: "--open-claw-path", value: true,
			apply: func(f *sharedFlags, st *agentFlagState, value string) error {
				st.openClawPath = value
				st.openClawPathGiven = true
				return nil
			},
		})
	}
	return options
}

// parseAgentOptions replicates the reference ArgParser loop: strict left-to-right
// tokens, inline =values, "Expected option" for bare words, and the reference's
// unknown-option and missing-value messages.
func parseAgentOptions(options []agentOption, f *sharedFlags, st *agentFlagState, args []string) error {
	index := 0
	for index < len(args) {
		arg := args[index]
		index++
		if !strings.HasPrefix(arg, "-") {
			return parseErr("Expected option, got '%s'", arg)
		}
		name := arg
		inline := ""
		hasInline := false
		if position := strings.Index(arg, "="); position >= 0 {
			name = arg[:position]
			inline = arg[position+1:]
			hasInline = true
		}
		switch name {
		case "-v", "-V", "--version":
			st.version = true
			continue
		case "-h", "--help":
			st.help = true
			continue
		}
		var option *agentOption
		for i := range options {
			if options[i].long == name || (options[i].short != "" && options[i].short == name) {
				option = &options[i]
				break
			}
		}
		if option == nil {
			return parseErr("Unknown option '%s'", name)
		}
		value := ""
		if option.value {
			if hasInline {
				if inline == "" {
					return parseErr("Missing value for %s", option.long)
				}
				value = inline
			} else {
				if index >= len(args) || strings.HasPrefix(args[index], "-") {
					return parseErr("Missing value for %s", option.long)
				}
				value = args[index]
				index++
			}
		}
		if err := option.apply(f, st, value); err != nil {
			return err
		}
	}
	return nil
}

// agentCommandSpec describes one agent's CLI surface.
type agentCommandSpec struct {
	agent       string
	display     string
	short       string
	openClawArg bool
	run         func(f *sharedFlags, kind agentKind, st *agentFlagState) error
}

// newAgentCommandTree builds the agent parent plus its daily/monthly/session
// subcommands. Flag parsing is manual (DisableFlagParsing) so the reference
// parser's messages and strictness carry over; the shared flag set is still
// registered so discovered token-usage config applies.
func newAgentCommandTree(spec *agentCommandSpec) *cobra.Command {
	options := agentOptionTable(spec.openClawArg)
	parent := &cobra.Command{
		Use:                spec.agent,
		Short:              spec.short,
		DisableFlagParsing: true,
	}
	parentFlags := registerSharedFlags(parent)
	parent.RunE = func(cmd *cobra.Command, args []string) error {
		return runAgentParent(spec, options, parentFlags, cmd, args)
	}
	for _, kind := range []agentKind{agentKindDaily, agentKindMonthly, agentKindSession} {
		parent.AddCommand(newAgentReportSubcommand(spec, options, kind))
	}
	return parent
}

func newAgentReportSubcommand(spec *agentCommandSpec, options []agentOption, kind agentKind) *cobra.Command {
	cmd := &cobra.Command{
		Use:                kind.use(),
		Short:              fmt.Sprintf("Show %s usage grouped by %s", spec.display, periodLabel(kind)),
		DisableFlagParsing: true,
	}
	f := registerSharedFlags(cmd)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runAgentReport(spec, options, f, c, kind, args)
	}
	return cmd
}

func periodLabel(kind agentKind) string {
	switch kind {
	case agentKindMonthly:
		return "month"
	case agentKindSession:
		return "session"
	default:
		return "date"
	}
}

// runAgentParent handles invocations whose first token is a flag (defaulting
// to the daily report) and rejects unsupported report names.
func runAgentParent(spec *agentCommandSpec, options []agentOption, f *sharedFlags, cmd *cobra.Command, args []string) error {
	kind := agentKindDaily
	rest := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		token := args[0]
		switch token {
		case "daily", "monthly", "session":
			// Normally routed to the subcommand by cobra; treat defensively.
			kind = agentKindFor(token)
			rest = args[1:]
		default:
			return parseErr("The \"%s\" report is not available for %s usage.\nUse \"token-usage %s daily\" for %s usage reports.",
				token, spec.display, spec.agent, spec.display)
		}
	}
	return runAgentReport(spec, options, f, cmd, kind, rest)
}

func agentKindFor(token string) agentKind {
	switch token {
	case "monthly":
		return agentKindMonthly
	case "session":
		return agentKindSession
	default:
		return agentKindDaily
	}
}

// runAgentReport parses the options, resolves config/last, and dispatches.
func runAgentReport(spec *agentCommandSpec, options []agentOption, f *sharedFlags, cmd *cobra.Command, kind agentKind, args []string) error {
	st := &agentFlagState{}
	if err := parseAgentOptions(options, f, st, args); err != nil {
		return err
	}
	if st.version {
		fmt.Fprintf(cmd.OutOrStdout(), "token-usage %s\n", Version)
		return nil
	}
	if st.help {
		return cmd.Help()
	}
	if err := f.resolve(); err != nil {
		return err
	}
	if err := f.validateLast(kind != agentKindSession); err != nil {
		return err
	}
	unit := core.PeriodDay
	if kind == agentKindMonthly {
		unit = core.PeriodMonth
	}
	f.resolveLastSince(unit, core.Monday)
	return spec.run(f, kind, st)
}

// agentPricing loads the pricing map for agent runs, mirroring the reference
// load_with_overrides call in each agent's run().
func agentPricing(shared *core.SharedArgs) *core.PricingMap {
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.Offline, refreshLog, shared.PricingOverrides)
}

// filterAgentEntriesByDate applies the since/until window to loaded entries.
func filterAgentEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	out := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			out = append(out, entries[i])
		}
	}
	return out
}

// filterAgentSessionSummaries filters session rows by last-activity date,
// matching the reference qwen session path.
func filterAgentSessionSummaries(rows []core.UsageSummary, shared *core.SharedArgs) []core.UsageSummary {
	if shared.Since == nil && shared.Until == nil {
		return rows
	}
	out := make([]core.UsageSummary, 0, len(rows))
	for i := range rows {
		date := ""
		if rows[i].LastActivity != nil {
			date = strings.ReplaceAll(*rows[i].LastActivity, "-", "")
		}
		if core.DateWithinRange(date, shared.Since, shared.Until) {
			out = append(out, rows[i])
		}
	}
	return out
}

// agentSummaryPeriod mirrors summary_period: date, else week, else month, else
// session id, else the empty string.
func agentSummaryPeriod(row *core.UsageSummary) string {
	switch {
	case row.Date != nil:
		return *row.Date
	case row.Week != nil:
		return *row.Week
	case row.Month != nil:
		return *row.Month
	case row.SessionID != nil:
		return *row.SessionID
	default:
		return ""
	}
}

// printAgentReport sorts the rows and renders the JSON or table form shared by
// the agent commands.
func printAgentReport(f *sharedFlags, rows []core.UsageSummary, kind agentKind, title string, sessionMeta, totalsNullEmpty bool) error {
	shared := f.shared
	rows = core.SortSummaries(rows, shared.Order, agentSummaryPeriod)
	if core.WantsJSON(shared) {
		items := make([]core.J, len(rows))
		for i := range rows {
			items[i] = agentSummaryJSON(&rows[i], kind, sessionMeta)
		}
		totals := core.TotalsJSON(rows)
		if totalsNullEmpty && len(rows) == 0 {
			totals = core.JNullV
		}
		return core.PrintJSONOrJQ(core.JObjV(
			kind.rowsKey(), core.JArrV(items...),
			"totals", totals,
		), shared.JQ, shared.NoCost)
	}
	return core.PrintUsageTable(title, kind.firstColumn(), rows, shared, false, nil)
}

// agentSummaryJSON mirrors agent_summary_json: the period field first, token
// totals and model breakdowns, then optional credits/messageCount and (for
// session reports) the activity metadata.
func agentSummaryJSON(row *core.UsageSummary, kind agentKind, sessionMeta bool) core.J {
	pairs := []any{
		kind.periodKey(), core.JStrV(agentSummaryPeriod(row)),
		"inputTokens", core.JUintV(row.InputTokens),
		"outputTokens", core.JUintV(row.OutputTokens),
		"cacheCreationTokens", core.JUintV(row.CacheCreationTokens),
		"cacheReadTokens", core.JUintV(row.CacheReadTokens),
		"totalTokens", core.JUintV(row.TotalTokens()),
		"totalCost", core.JFloatV(row.TotalCost),
		"modelsUsed", agentModelsUsedJ(row.ModelsUsed),
		"modelBreakdowns", agentModelBreakdownsJ(row.ModelBreakdowns),
	}
	if row.Credits != nil {
		pairs = append(pairs, "credits", core.JFloatV(*row.Credits))
	}
	if row.MessageCount != nil {
		pairs = append(pairs, "messageCount", core.JUintV(*row.MessageCount))
	}
	if sessionMeta {
		pairs = append(pairs,
			"lastActivity", core.JOptStrV(row.LastActivity),
			"firstActivity", core.JOptStrV(row.FirstActivity),
			"projectPath", core.JOptStrV(row.ProjectPath),
		)
	}
	return core.JObjV(pairs...)
}

func agentModelsUsedJ(models []string) core.J {
	items := make([]core.J, 0, len(models))
	for _, model := range models {
		items = append(items, core.JStrV(model))
	}
	return core.JArrV(items...)
}

func agentModelBreakdownsJ(breakdowns []core.ModelBreakdown) core.J {
	items := make([]core.J, 0, len(breakdowns))
	for i := range breakdowns {
		b := &breakdowns[i]
		items = append(items, core.JObjV(
			"modelName", core.JStrV(b.ModelName),
			"inputTokens", core.JUintV(b.InputTokens),
			"outputTokens", core.JUintV(b.OutputTokens),
			"cacheCreationTokens", core.JUintV(b.CacheCreationTokens),
			"cacheReadTokens", core.JUintV(b.CacheReadTokens),
			"cost", core.JFloatV(b.Cost),
		))
	}
	return core.JArrV(items...)
}

// mapGeminiReportKind maps the CLI kind onto the adapter's kind.
func mapGeminiReportKind(kind agentKind) gemini.ReportKind {
	switch kind {
	case agentKindSession:
		return gemini.KindSession
	case agentKindMonthly:
		return gemini.KindMonthly
	case agentKindWeekly:
		return gemini.KindWeekly
	default:
		return gemini.KindDaily
	}
}

func newGeminiCommand() *cobra.Command {
	spec := &agentCommandSpec{
		agent:   "gemini",
		display: "Gemini CLI",
		short:   "Show Gemini CLI usage commands",
		run:     runGeminiReport,
	}
	return newAgentCommandTree(spec)
}

// runGeminiReport loads Gemini entries, filters by date, summarizes, prints.
func runGeminiReport(f *sharedFlags, kind agentKind, st *agentFlagState) error {
	shared := f.shared
	pricing := agentPricing(shared)
	entries, err := gemini.LoadEntries(shared, pricing)
	if err != nil {
		return err
	}
	entries = filterAgentEntriesByDate(entries, shared)
	rows := gemini.SummarizeEntries(entries, mapGeminiReportKind(kind))
	return printAgentReport(f, rows, kind, "Gemini CLI Token Usage Report", false, false)
}
