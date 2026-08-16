package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The unified agent command framework (ADR 0010): one command tree builder,
// one option parser, one window resolver for every agent subcommand. Each
// agent declares an agentCommandSpec; the standard path (load → shared
// pipeline → printAgentReport) covers the agents without custom rendering,
// and run overrides it for the exceptions (amp's Credits table, codex's
// Groups pipeline, opencode's entries-shaped JSON, …).

// agentFlagState carries the manually parsed agent flags alongside the shared
// flag state. Extra-option values live in a generic map so the state does not
// grow a field per agent flag.
type agentFlagState struct {
	values  map[string]string
	given   map[string]bool
	version bool
	help    bool
}

func (st *agentFlagState) set(long, value string) {
	if st.values == nil {
		st.values = map[string]string{}
		st.given = map[string]bool{}
	}
	st.values[long] = value
	st.given[long] = true
}

// has reports whether the extra option appeared at all (`--open-claw-path`
// with an empty value still counts as given).
func (st *agentFlagState) has(long string) bool { return st.given[long] }

// get returns the extra option's value ("" when absent).
func (st *agentFlagState) get(long string) string { return st.values[long] }

// agentOption describes one CLI option of an agent command, with the
// reference parser's canonical name used in error messages. bareDefault
// reproduces pflag's NoOptDefVal: the option used without a value takes the
// default instead of erroring (codex `--speed`).
type agentOption struct {
	long        string
	short       string
	value       bool
	bareDefault string
	apply       func(f *sharedFlags, st *agentFlagState, value string) error
}

// agentExtraOption declares a per-agent option appended after the shared
// table; store holds the parsed value via st.set. help, when non-empty,
// also registers the option with cobra so --help renders it exactly as the
// pre-framework tree did — on every command with helpSubs (codex --speed),
// on the parent only without it (pi --pi-path). Parsing stays manual either
// way, and help-less options (openclaw's --open-claw-path) never rendered.
type agentExtraOption struct {
	long        string
	short       string
	bareDefault string
	help        string
	helpSubs    bool
	store       func(st *agentFlagState, value string)
}

func (x agentExtraOption) toOption() agentOption {
	return agentOption{
		long:        x.long,
		short:       x.short,
		value:       true,
		bareDefault: x.bareDefault,
		apply: func(f *sharedFlags, st *agentFlagState, value string) error {
			x.store(st, value)
			return nil
		},
	}
}

func agentOptionTable(extras []agentExtraOption) []agentOption {
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
	for _, extra := range extras {
		options = append(options, extra.toOption())
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
			} else if index < len(args) && !strings.HasPrefix(args[index], "-") {
				value = args[index]
				index++
			} else if option.bareDefault != "" {
				value = option.bareDefault
			} else {
				return parseErr("Missing value for %s", option.long)
			}
		}
		if err := option.apply(f, st, value); err != nil {
			return err
		}
	}
	return nil
}

// agentCommandSpec declares one agent's CLI surface. The standard path needs
// load/profile/title (plus the two render flags); run replaces the standard
// render for the custom-table/custom-JSON agents.
type agentCommandSpec struct {
	agent   string
	display string
	short   string

	// extraOptions are appended after the shared table (--open-claw-path,
	// --pi-path, --speed).
	extraOptions []agentExtraOption

	// subShort overrides the subcommand Short line; nil uses the generic
	// "Show <display> usage grouped by <period>" wording. codex and opencode
	// keep their pre-framework "token usage grouped by day/…" phrasing.
	subShort func(kind core.ReportKind) string

	// Standard path: load the agent's entries; the framework runs the shared
	// pipeline and printAgentReport.
	load    func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) ([]core.LoadedEntry, error)
	profile common.ReportProfile
	title   string

	// sessionMeta: session rows carry the activity metadata in JSON.
	sessionMeta bool
	// totalsNullEmpty: an empty JSON report renders a null totals object.
	totalsNullEmpty bool

	// run overrides the standard path entirely.
	run func(f *sharedFlags, kind core.ReportKind, st *agentFlagState) error
}

// newAgentCommandTree builds the agent parent plus one subcommand per kind in
// the support matrix. Flag parsing is manual (DisableFlagParsing) so the
// reference parser's messages and strictness carry over; the shared flag set
// is still registered so discovered token-usage config applies.
func newAgentCommandTree(spec *agentCommandSpec) *cobra.Command {
	options := agentOptionTable(spec.extraOptions)
	parent := &cobra.Command{
		Use:                spec.agent,
		Short:              spec.short,
		DisableFlagParsing: true,
	}
	parentFlags := registerSharedFlags(parent)
	registerExtraOptionFlags(parent, spec.extraOptions, false)
	parent.RunE = func(cmd *cobra.Command, args []string) error {
		return runAgentParent(spec, options, parentFlags, cmd, args)
	}
	for _, kind := range agentReportKinds(spec.agent) {
		parent.AddCommand(newAgentReportSubcommand(spec, options, kind))
	}
	return parent
}

func newAgentReportSubcommand(spec *agentCommandSpec, options []agentOption, kind core.ReportKind) *cobra.Command {
	short := fmt.Sprintf("Show %s usage grouped by %s", spec.display, periodLabel(kind))
	if spec.subShort != nil {
		short = spec.subShort(kind)
	}
	cmd := &cobra.Command{
		Use:                kind.String(),
		Short:              short,
		DisableFlagParsing: true,
	}
	f := registerSharedFlags(cmd)
	registerExtraOptionFlags(cmd, spec.extraOptions, true)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runAgentReport(spec, options, f, c, kind, args)
	}
	return cmd
}

// registerExtraOptionFlags registers help-carrying extras on cmd's flag set
// for --help rendering only — parsing stays with the manual option table.
// Subcommands render only the helpSubs options (pi's --pi-path was a
// parent-only line in the pre-framework tree).
func registerExtraOptionFlags(cmd *cobra.Command, extras []agentExtraOption, subcommand bool) {
	for _, x := range extras {
		if x.help == "" {
			continue
		}
		if subcommand && !x.helpSubs {
			continue
		}
		name := strings.TrimPrefix(x.long, "--")
		cmd.Flags().String(name, x.bareDefault, x.help)
		cmd.Flags().Lookup(name).NoOptDefVal = x.bareDefault
	}
}

// shortTokenUsageGrouped is the pre-framework Short wording for codex and
// opencode ("Show Codex token usage grouped by day"); the generic wording
// says "usage grouped by date".
func shortTokenUsageGrouped(display string, kind core.ReportKind) string {
	period := map[core.ReportKind]string{
		core.KindDaily:   "day",
		core.KindWeekly:  "week",
		core.KindMonthly: "month",
		core.KindSession: "session",
	}[kind]
	return fmt.Sprintf("Show %s token usage grouped by %s", display, period)
}

func periodLabel(kind core.ReportKind) string {
	switch kind {
	case core.KindMonthly:
		return "month"
	case core.KindWeekly:
		return "week"
	case core.KindSession:
		return "session"
	default:
		return "date"
	}
}

// runAgentParent handles invocations whose first token is a flag (defaulting
// to the daily report) and rejects unsupported report names.
func runAgentParent(spec *agentCommandSpec, options []agentOption, f *sharedFlags, cmd *cobra.Command, args []string) error {
	kind := core.KindDaily
	rest := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		token := args[0]
		if routed, ok := kindForToken(token); ok && agentKindSupported(spec.agent, routed) {
			// Normally routed to the subcommand by cobra; treat defensively.
			kind = routed
			rest = args[1:]
		} else {
			return unsupportedAgentReportError(spec.agent, spec.display, token)
		}
	}
	return runAgentReport(spec, options, f, cmd, kind, rest)
}

func kindForToken(token string) (core.ReportKind, bool) {
	switch token {
	case "daily":
		return core.KindDaily, true
	case "weekly":
		return core.KindWeekly, true
	case "monthly":
		return core.KindMonthly, true
	case "session":
		return core.KindSession, true
	}
	return core.KindDaily, false
}

// runAgentReport parses the options, resolves config/last, and dispatches to
// the spec's custom run or the standard load→pipeline→render path.
func runAgentReport(spec *agentCommandSpec, options []agentOption, f *sharedFlags, cmd *cobra.Command, kind core.ReportKind, args []string) error {
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
	if err := f.validateLast(kind != core.KindSession); err != nil {
		return err
	}
	f.resolveLastSince(lastPeriodUnit(kind), core.Monday)
	if spec.run != nil {
		return spec.run(f, kind, st)
	}
	entries, err := spec.load(f, kind, st)
	if err != nil {
		return err
	}
	rows := common.ReportRows(entries, kind, f.shared, spec.profile)
	return printAgentReport(f, rows, kind, spec.title, spec.sessionMeta && kind == core.KindSession, spec.totalsNullEmpty)
}

func lastPeriodUnit(kind core.ReportKind) core.PeriodUnit {
	switch kind {
	case core.KindMonthly:
		return core.PeriodMonth
	case core.KindWeekly:
		return core.PeriodWeek
	default:
		return core.PeriodDay
	}
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

// unsupportedAgentReportError mirrors the reference parser's message for a
// report name the agent does not offer.
func unsupportedAgentReportError(use, display, report string) error {
	if report == "blocks" || report == "statusline" {
		return parseErr("The %q report is only available for Claude Code usage.\nUse \"token-usage %s daily\" for %s usage reports.", report, use, display)
	}
	return parseErr("The %q report is not available for %s usage.\nUse \"token-usage %s daily\" for %s usage reports.", report, display, use, display)
}
