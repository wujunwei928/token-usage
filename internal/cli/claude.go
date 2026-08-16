package cli

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/claude"
	"github.com/wujunwei928/token-usage/internal/blocks"
	"github.com/wujunwei928/token-usage/internal/config"
	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/statusline"
)

// ParseError reports a CLI usage error: printed without prefix, exit code 2.
type ParseError struct{ Message string }

func (e *ParseError) Error() string { return e.Message }

func parseErr(format string, args ...any) error {
	return &ParseError{fmt.Sprintf(format, args...) + "\nRun 'token-usage --help' for usage."}
}

// sharedFlags holds the raw flag values resolved into SharedArgs at run time.
type sharedFlags struct {
	shared    *core.SharedArgs
	since     string
	until     string
	modeRaw   string
	orderRaw  string
	tzRaw     string
	configRaw string
	jqRaw     string
	lastRaw   uint32
}

func registerSharedFlags(cmd *cobra.Command) *sharedFlags {
	f := &sharedFlags{shared: &core.SharedArgs{}}
	flags := cmd.Flags()
	flags.StringVarP(&f.since, "since", "s", "", "Filter from date (YYYYMMDD format)")
	flags.StringVarP(&f.until, "until", "u", "", "Filter until date (YYYYMMDD format)")
	flags.BoolVarP(&f.shared.JSON, "json", "j", false, "Output in JSON format (default: false)")
	flags.StringVarP(&f.modeRaw, "mode", "m", "auto", "Cost calculation mode (default: auto, choices: auto | calculate | display)")
	flags.Lookup("mode").NoOptDefVal = "auto"
	flags.BoolVarP(&f.shared.Debug, "debug", "d", false, "Show pricing mismatch information for debugging (default: false)")
	flags.IntVar(&f.shared.DebugSamples, "debug-samples", 5, "Number of sample discrepancies to show in debug output (default: 5)")
	flags.Lookup("debug-samples").NoOptDefVal = "5"
	flags.StringVarP(&f.orderRaw, "order", "o", "asc", "Sort order (default: asc, choices: desc | asc)")
	flags.Lookup("order").NoOptDefVal = "asc"
	flags.BoolVarP(&f.shared.Breakdown, "breakdown", "b", false, "Show per-model cost breakdown (default: false)")
	flags.BoolVarP(&f.shared.Offline, "offline", "O", false, "Use cached pricing data for Claude models instead of fetching from API (default: false)")
	flags.BoolVar(&f.shared.NoOffline, "no-offline", false, "Negatable of -O, --offline")
	flags.BoolVar(&f.shared.SingleThread, "single-thread", false, "Disable parallel JSONL file loading (default: false)")
	flags.BoolVar(&f.shared.Color, "color", false, "Enable colored output. FORCE_COLOR=1 has the same effect. (default: auto)")
	flags.BoolVar(&f.shared.NoColor, "no-color", false, "Disable colored output. NO_COLOR=1 has the same effect. (default: auto)")
	flags.StringVarP(&f.tzRaw, "timezone", "z", "", "Timezone for date grouping (e.g., UTC, America/New_York, Asia/Tokyo) (default: system timezone)")
	flags.StringVar(&f.configRaw, "config", "", "Path to configuration file (default: auto-discovery)")
	flags.StringVarP(&f.jqRaw, "jq", "q", "", "Pipe JSON output through jq with the given filter")
	flags.BoolVar(&f.shared.Compact, "compact", false, "Force compact mode for narrow displays (better for screenshots) (default: false)")
	flags.BoolVar(&f.shared.NoCost, "no-cost", false, "Hide cost information in table and JSON output (default: false)")
	flags.Uint32Var(&f.lastRaw, "last", 0, "Show only the most recent N periods of the report (1 is today, this week, or this month)")
	// Ticket-10 hook (single call site): apply ccusage.json after flag parsing.
	WireCommandConfig(cmd, f.shared)
	return f
}

// resolve folds raw flag strings into SharedArgs and validates them.
func (f *sharedFlags) resolve() error {
	if f.since != "" {
		f.shared.Since = &f.since
	}
	if f.until != "" {
		f.shared.Until = &f.until
	}
	mode, ok := core.ParseCostMode(f.modeRaw)
	if !ok {
		return parseErr("Invalid cost mode '%s'", f.modeRaw)
	}
	f.shared.Mode = mode
	order, ok := core.ParseSortOrder(f.orderRaw)
	if !ok {
		return parseErr("Invalid sort order '%s'", f.orderRaw)
	}
	f.shared.Order = order
	if f.tzRaw != "" {
		f.shared.Timezone = &f.tzRaw
	}
	if f.configRaw != "" {
		f.shared.Config = &f.configRaw
	}
	if f.jqRaw != "" {
		f.shared.JQ = &f.jqRaw
	}
	if f.lastRaw != 0 {
		f.shared.Last = &f.lastRaw
	}
	return nil
}

// validateLast enforces the --last rules shared by every report command.
func (f *sharedFlags) validateLast(supported bool) error {
	if f.shared.Last == nil {
		return nil
	}
	if !supported {
		return parseErr("The --last option is only available for the daily, weekly, and monthly reports.")
	}
	if f.shared.Since != nil || f.shared.Until != nil {
		return parseErr("The --last option cannot be combined with --since or --until.")
	}
	return nil
}

// resolveLastSince turns --last into a compact since bound anchored on today
// in the report timezone.
func (f *sharedFlags) resolveLastSince(unit core.PeriodUnit, startOfWeek core.WeekDay) {
	if f.shared.Last == nil {
		return
	}
	tz := core.ParseTZ(f.shared.Timezone)
	today := core.FormatDateTZ(core.UTCNow(), tz)
	if since, ok := core.LastPeriodsSince(unit, *f.shared.Last, today, startOfWeek); ok {
		f.shared.Since = &since
	}
}

func newClaudeCommand() *cobra.Command {
	claudeCmd := &cobra.Command{
		Use:   "claude",
		Short: "Show Claude Code usage commands",
	}
	claudeCmd.AddCommand(
		newClaudeReportCommand(reportDaily),
		newClaudeReportCommand(reportWeekly),
		newClaudeReportCommand(reportMonthly),
		newClaudeSessionCommand(),
		newClaudeBlocksCommand(),
		newClaudeStatuslineCommand(),
	)
	return claudeCmd
}

// newClaudeStatuslineCommand builds `claude statusline`: it registers only the
// statusline-specific flags (shared report flags are rejected, like the
// reference parser) and applies token-usage config under CLI precedence.
func newClaudeStatuslineCommand() *cobra.Command {
	var offline bool
	var noOffline bool
	var visualBurnRateRaw string
	var costSourceRaw string
	var cache bool
	var noCache bool
	var refreshIntervalRaw string
	var contextLowRaw string
	var contextMediumRaw string
	var timezone string
	var configPath string
	var debug bool
	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "Display compact status line for Claude Code hooks with hybrid time+file caching (Beta)",
	}
	flags := cmd.Flags()
	flags.BoolVarP(&offline, "offline", "O", true, "Use cached pricing data for Claude models instead of fetching from API (default: true)")
	flags.BoolVar(&noOffline, "no-offline", false, "Negatable of -O, --offline")
	flags.StringVarP(&visualBurnRateRaw, "visual-burn-rate", "B", "off", "Controls the visualization of the burn rate status (default: off, choices: off | emoji | text | emoji-text)")
	flags.Lookup("visual-burn-rate").NoOptDefVal = "off"
	flags.StringVar(&costSourceRaw, "cost-source", "auto", "Session cost source (default: auto, choices: auto | token-usage | cc | both; legacy value \"ccusage\" still accepted)")
	flags.Lookup("cost-source").NoOptDefVal = "auto"
	flags.BoolVar(&cache, "cache", true, "Enable cache for status line output (default: true)")
	flags.BoolVar(&noCache, "no-cache", false, "Negatable of --cache")
	flags.StringVar(&refreshIntervalRaw, "refresh-interval", "1", "Refresh interval in seconds for cache expiry (default: 1)")
	flags.Lookup("refresh-interval").NoOptDefVal = "1"
	flags.StringVar(&contextLowRaw, "context-low-threshold", "50", "Context usage percentage below which status is shown in green (0-100) (default: 50)")
	flags.StringVar(&contextMediumRaw, "context-medium-threshold", "80", "Context usage percentage below which status is shown in yellow (0-100) (default: 80)")
	flags.StringVarP(&timezone, "timezone", "z", "", "Timezone for date grouping (IANA)")
	flags.StringVar(&configPath, "config", "", "Path to configuration file (default: auto-discovery)")
	flags.BoolVarP(&debug, "debug", "d", false, "Show pricing mismatch information for debugging (default: false)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Validate raw flag values first (reference exits 2 on bad values).
		burnRate, ok := statusline.ParseVisualBurnRate(visualBurnRateRaw)
		if !ok {
			return parseErr("Invalid visual burn rate '%s'", visualBurnRateRaw)
		}
		costSource, ok := statusline.ParseCostSource(costSourceRaw)
		if !ok {
			return parseErr("Invalid cost source '%s'", costSourceRaw)
		}
		refreshInterval, err := strconv.ParseUint(refreshIntervalRaw, 10, 64)
		if err != nil {
			return parseErr("Invalid value for --refresh-interval")
		}
		contextLow, err := strconv.ParseUint(contextLowRaw, 10, 8)
		if err != nil {
			return parseErr("Invalid value for --context-low-threshold")
		}
		contextMedium, err := strconv.ParseUint(contextMediumRaw, 10, 8)
		if err != nil {
			return parseErr("Invalid value for --context-medium-threshold")
		}

		// Config fills anything the CLI did not set (CLI > config > default).
		cfg := config.StatuslineArgs{
			Offline:                offline,
			NoOffline:              noOffline,
			VisualBurnRate:         config.VisualBurnRate(burnRate),
			CostSource:             config.CostSource(costSource),
			Cache:                  cache,
			NoCache:                noCache,
			RefreshInterval:        refreshInterval,
			ContextLowThreshold:    uint8(contextLow),
			ContextMediumThreshold: uint8(contextMedium),
			Debug:                  debug,
		}
		if timezone != "" {
			tz := timezone
			cfg.Timezone = &tz
		}
		config.ApplyConfig("statusline", "claude", nil, func(name string) bool {
			return cmd.Flags().Changed(name)
		}, &cfg)

		// The config and statusline enums are the same iota ladders over the
		// reference vocabulary, so the conversions are value-preserving.
		statuslineArgs := &statusline.Args{
			Offline:             cfg.Offline,
			NoOffline:           cfg.NoOffline,
			VisualBurnRate:      statusline.VisualBurnRate(cfg.VisualBurnRate),
			CostSource:          statusline.CostSource(cfg.CostSource),
			Cache:               cfg.Cache,
			NoCache:             cfg.NoCache,
			RefreshInterval:     cfg.RefreshInterval,
			ContextLowThreshold: uint64(cfg.ContextLowThreshold),
			ContextMediumThresh: uint64(cfg.ContextMediumThreshold),
			Timezone:            cfg.Timezone,
			Debug:               cfg.Debug,
			ModelLabelAliases:   cfg.ModelLabelAliases,
		}
		var configArg *string
		if configPath != "" {
			cfgPath := configPath
			configArg = &cfgPath
		}
		statuslineArgs.Config = configArg
		return statusline.Run(os.Stdin, os.Stdout, statuslineArgs)
	}
	return cmd
}

func newClaudeSessionCommand() *cobra.Command {
	var sessionID string
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Show usage report grouped by conversation session",
	}
	f := registerSharedFlags(cmd)
	cmd.Flags().StringVarP(&sessionID, "id", "i", "", "Filter to specific session ID")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := f.resolve(); err != nil {
			return err
		}
		if err := f.validateLast(false); err != nil {
			return err
		}
		entries, err := claude.LoadEntries(claude.LoadOptions{Shared: f.shared})
		if err != nil {
			return err
		}
		if sessionID != "" {
			return runClaudeSessionID(sessionID, f.shared, entries)
		}
		// Session rows always present newest-first regardless of --order.
		f.shared.Order = core.OrderDesc
		var grouped []*core.SessionAccumulator
		indexes := map[[2]string]int{}
		for i := range entries {
			entry := &entries[i]
			key := [2]string{entry.ProjectPath, entry.SessionID}
			index, ok := indexes[key]
			if !ok {
				index = len(grouped)
				indexes[key] = index
				grouped = append(grouped, &core.SessionAccumulator{})
			}
			grouped[index].AddEntry(entry)
		}
		rows := make([]core.UsageSummary, 0, len(grouped))
		for _, acc := range grouped {
			rows = append(rows, acc.IntoSummary())
		}
		if f.shared.Since != nil || f.shared.Until != nil {
			filtered := make([]core.UsageSummary, 0, len(rows))
			for i := range rows {
				date := ""
				if rows[i].LastActivity != nil {
					date = strings.ReplaceAll(*rows[i].LastActivity, "-", "")
				}
				if core.DateWithinRange(date, f.shared.Since, f.shared.Until) {
					filtered = append(filtered, rows[i])
				}
			}
			rows = filtered
		}
		kept := rows[:0]
		for i := range rows {
			if rows[i].InputTokens+rows[i].OutputTokens+rows[i].CacheCreationTokens+rows[i].CacheReadTokens > 0 {
				kept = append(kept, rows[i])
			}
		}
		rows = kept
		sortSessionsByCost(rows, f.shared.Order)

		if core.WantsJSON(f.shared) {
			items := make([]core.J, len(rows))
			for i := range rows {
				items[i] = core.SessionSummaryJSON(&rows[i])
			}
			return core.PrintJSONOrJQ(core.JObjV(
				"sessions", core.JArrV(items...),
				"totals", core.TotalsJSON(rows),
			), f.shared.JQ, f.shared.NoCost)
		}
		return core.PrintUsageTable("Claude Code Token Usage Report - By Session", "Session",
			rows, f.shared, false, nil)
	}
	return cmd
}

func sortSessionsByCost(rows []core.UsageSummary, order core.SortOrder) {
	sort.SliceStable(rows, func(i, j int) bool {
		if order == core.OrderDesc {
			return rows[j].TotalCost < rows[i].TotalCost
		}
		return rows[i].TotalCost < rows[j].TotalCost
	})
}

func runClaudeSessionID(id string, shared *core.SharedArgs, entries []core.LoadedEntry) error {
	var sessionEntries []core.LoadedEntry
	for i := range entries {
		entry := &entries[i]
		if (entry.Data.SessionID != nil && *entry.Data.SessionID == id) || entry.SessionID == id {
			sessionEntries = append(sessionEntries, *entry)
		}
	}
	sort.SliceStable(sessionEntries, func(i, j int) bool {
		return sessionEntries[i].Timestamp < sessionEntries[j].Timestamp
	})
	if len(sessionEntries) == 0 {
		if core.WantsJSON(shared) {
			fmt.Println("null")
		} else {
			fmt.Fprintf(os.Stderr, "No session found with ID: %s\n", id)
		}
		return nil
	}
	totalCost := math.Copysign(0, -1)
	var totalTokens uint64
	for i := range sessionEntries {
		totalCost += sessionEntries[i].Cost
		totalTokens += core.TotalUsageTokens(sessionEntries[i].Data.Message.Usage)
	}
	if core.WantsJSON(shared) {
		items := make([]core.J, len(sessionEntries))
		for i := range sessionEntries {
			e := &sessionEntries[i]
			model := "unknown"
			if e.Data.Message.Model != nil {
				model = *e.Data.Message.Model
			}
			costUSD := 0.0
			if e.Data.CostUSD != nil {
				costUSD = *e.Data.CostUSD
			}
			items[i] = core.JObjV(
				"timestamp", core.JStrV(e.Data.Timestamp),
				"inputTokens", core.JUintV(e.Data.Message.Usage.InputTokens),
				"outputTokens", core.JUintV(e.Data.Message.Usage.OutputTokens),
				"cacheCreationTokens", core.JUintV(e.Data.Message.Usage.CacheCreationInputTokens),
				"cacheReadTokens", core.JUintV(e.Data.Message.Usage.CacheReadInputTokens),
				"model", core.JStrV(model),
				"costUSD", core.JFloatV(costUSD),
			)
		}
		return core.PrintJSONOrJQ(core.JObjV(
			"sessionId", core.JStrV(id),
			"totalCost", core.JFloatV(totalCost),
			"totalTokens", core.JUintV(totalTokens),
			"entries", core.JArrV(items...),
		), shared.JQ, shared.NoCost)
	}
	fmt.Printf("Claude Code Session Usage - %s\n", id)
	if !shared.NoCost {
		fmt.Printf("Total Cost: %s\n", core.FormatCurrency(totalCost))
	}
	fmt.Printf("Total Tokens: %s\n", core.FormatNumber(totalTokens))
	fmt.Printf("Total Entries: %d\n", len(sessionEntries))
	return nil
}

type reportKind int

const (
	reportDaily reportKind = iota
	reportWeekly
	reportMonthly
)

func (k reportKind) meta() (use, short, title, firstColumn, jsonKey string) {
	switch k {
	case reportDaily:
		return "daily", "Show usage report grouped by date",
			"Claude Code Token Usage Report - Daily", "Date", "daily"
	case reportWeekly:
		return "weekly", "Show usage report grouped by week",
			"Claude Code Token Usage Report - Weekly", "Week", "weekly"
	default:
		return "monthly", "Show usage report grouped by month",
			"Claude Code Token Usage Report - Monthly", "Month", "monthly"
	}
}

func newClaudeReportCommand(kind reportKind) *cobra.Command {
	use, short, title, firstColumn, jsonKey := kind.meta()
	var instances bool
	var projectFilter string
	var projectAliases string
	var startOfWeekRaw string
	cmd := &cobra.Command{Use: use, Short: short}
	f := registerSharedFlags(cmd)
	if kind == reportWeekly {
		cmd.Flags().StringVarP(&startOfWeekRaw, "start-of-week", "w", "sunday",
			"Start day of the week (default: sunday, choices: sunday | monday | tuesday | wednesday | thursday | friday | saturday)")
		cmd.Flags().Lookup("start-of-week").NoOptDefVal = "sunday"
	}
	if kind == reportDaily {
		cmd.Flags().BoolVarP(&instances, "instances", "i", false, "Show usage breakdown by project/instance (default: false)")
		cmd.Flags().StringVarP(&projectFilter, "project", "p", "", "Filter to specific project name")
		cmd.Flags().StringVar(&projectAliases, "project-aliases", "", "Comma-separated project aliases (e.g., 'token-usage=Usage Tracker,myproject=My Project')")
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := f.resolve(); err != nil {
			return err
		}
		if err := f.validateLast(true); err != nil {
			return err
		}
		startOfWeek := core.Sunday
		if kind == reportWeekly {
			day, ok := core.ParseWeekDay(startOfWeekRaw)
			if !ok {
				return parseErr("Invalid week day '%s'", startOfWeekRaw)
			}
			startOfWeek = day
		}

		var project *string
		if projectFilter != "" {
			project = &projectFilter
		}
		groupByProject := kind == reportDaily && (instances || project != nil)

		var unit core.PeriodUnit
		switch kind {
		case reportDaily:
			unit = core.PeriodDay
		case reportWeekly:
			unit = core.PeriodWeek
		default:
			unit = core.PeriodMonth
		}
		f.resolveLastSince(unit, startOfWeek)

		var rows []core.UsageSummary
		if kind == reportDaily {
			// Daily runs its own single-pass pipeline (daily.rs in the
			// reference): it also parses agent-progress lines and tiebreaks
			// dedup on cost before speed.
			dailyRows, err := claude.LoadDailySummaries(f.shared, project, groupByProject)
			if err != nil {
				return err
			}
			rows = dailyRows
		} else {
			entries, err := claude.LoadEntries(claude.LoadOptions{Shared: f.shared, ProjectFilter: project})
			if err != nil {
				return err
			}
			rows = core.SummarizeByKey(entries,
				func(e *core.LoadedEntry) string {
					if groupByProject {
						return e.Date + "|" + e.Project
					}
					return e.Date
				},
				func(key string) (string, *string) {
					if groupByProject {
						date, projectPath, _ := strings.Cut(key, "|")
						return date, &projectPath
					}
					return key, nil
				})
		}
		if kind != reportDaily {
			// Weekly/monthly filter the DAILY rows first, then bucket.
			rows = core.FilterAndSortSummaries(rows, f.shared, func(r *core.UsageSummary) string {
				if r.Date != nil {
					return *r.Date
				}
				return ""
			})
			bucket := core.BucketWeekly
			if kind == reportMonthly {
				bucket = core.BucketMonthly
			}
			rows = core.SummarizeSummariesByBucket(rows, bucket, startOfWeek)
			rows = core.SortSummaries(rows, f.shared.Order, func(r *core.UsageSummary) string {
				if kind == reportMonthly && r.Month != nil {
					return *r.Month
				}
				if r.Week != nil {
					return *r.Week
				}
				return ""
			})
		} else {
			rows = core.FilterAndSortSummaries(rows, f.shared, func(r *core.UsageSummary) string {
				if r.Date != nil {
					return *r.Date
				}
				return ""
			})
		}

		if core.WantsJSON(f.shared) {
			if groupByProject && anyProject(rows) {
				return core.PrintJSONOrJQ(core.JObjV(
					"projects", core.GroupProjectOutput(rows),
					"totals", core.TotalsJSON(rows),
				), f.shared.JQ, f.shared.NoCost)
			}
			items := make([]core.J, len(rows))
			for i := range rows {
				items[i] = core.SummaryJSON(&rows[i])
			}
			return core.PrintJSONOrJQ(core.JObjV(
				jsonKey, core.JArrV(items...),
				"totals", core.TotalsJSON(rows),
			), f.shared.JQ, f.shared.NoCost)
		}

		var aliasesPtr *string
		if projectAliases != "" {
			aliasesPtr = &projectAliases
		}
		return core.PrintUsageTable(title, firstColumn, rows, f.shared, instances, aliasesPtr)
	}
	return cmd
}

func anyProject(rows []core.UsageSummary) bool {
	for i := range rows {
		if rows[i].Project != nil {
			return true
		}
	}
	return false
}

func newClaudeBlocksCommand() *cobra.Command {
	var active bool
	var recent bool
	var tokenLimit string
	var sessionLength float64
	cmd := &cobra.Command{
		Use:   "blocks",
		Short: "Show usage report grouped by session billing blocks",
	}
	f := registerSharedFlags(cmd)
	cmd.Flags().BoolVarP(&active, "active", "a", false, "Show only active block with projections (default: false)")
	cmd.Flags().BoolVarP(&recent, "recent", "r", false, "Show blocks from last 3 days (including active) (default: false)")
	cmd.Flags().StringVarP(&tokenLimit, "token-limit", "t", "", "Token limit for quota warnings (e.g., 500000 or \"max\")")
	cmd.Flags().Float64VarP(&sessionLength, "session-length", "n", blocks.DefaultSessionDurationHours,
		"Session block duration in hours (default: 5)")
	cmd.Flags().Lookup("session-length").NoOptDefVal = "5"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := f.resolve(); err != nil {
			return err
		}
		if err := f.validateLast(false); err != nil {
			return err
		}
		if sessionLength <= 0 {
			return &core.CLIError{Message: "Session length must be a positive number"}
		}
		entries, err := claude.LoadEntries(claude.LoadOptions{Shared: f.shared})
		if err != nil {
			return err
		}
		blockList := blocks.IdentifySessionBlocks(entries, sessionLength)
		blockList = blocks.FilterBlocksByDate(blockList, f.shared)
		blockList = blocks.SortBlocks(blockList, f.shared.Order)
		if recent {
			cutoff := core.UTCNow() - int64(blocks.DefaultRecentDays)*24*3600*1000
			kept := blockList[:0]
			for i := range blockList {
				if blockList[i].StartTime >= cutoff || blockList[i].IsActive {
					kept = append(kept, blockList[i])
				}
			}
			blockList = kept
		}
		if active {
			kept := blockList[:0]
			for i := range blockList {
				if blockList[i].IsActive {
					kept = append(kept, blockList[i])
				}
			}
			blockList = kept
		}
		var maxTokens uint64
		for i := range blockList {
			b := &blockList[i]
			if !b.IsGap && !b.IsActive && b.TokenCounts.Total() > maxTokens {
				maxTokens = b.TokenCounts.Total()
			}
		}
		var tokenLimitPtr *string
		if tokenLimit != "" {
			tokenLimitPtr = &tokenLimit
		}
		if core.WantsJSON(f.shared) {
			items := make([]core.J, len(blockList))
			for i := range blockList {
				items[i] = blocks.BlockJSON(&blockList[i], tokenLimitPtr, maxTokens)
			}
			return core.PrintJSONOrJQ(core.JObjV("blocks", core.JArrV(items...)), f.shared.JQ, f.shared.NoCost)
		}
		if active && len(blockList) == 0 {
			fmt.Println("No active session block found.")
			return nil
		}
		if active && len(blockList) == 1 {
			blocks.PrintActiveBlockDetail(&blockList[0], tokenLimitPtr, maxTokens, f.shared)
			return nil
		}
		return blocks.PrintBlocksTable(blockList, tokenLimitPtr, maxTokens, f.shared)
	}
	return cmd
}
