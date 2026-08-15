// The Apply step: how a loaded config reaches the parsed CLI arguments.
//
// Two entry points share one implementation:
//
//   - ApplyConfig(command, agent, shared, changed, commandArgs...) is the
//     documented wiring point for the CLI layer. Call it in a command's RunE
//     after cobra parsed the flags; `changed` is typically
//     func(name string) bool { return cmd.Flags().Changed(name) } and receives
//     FLAG names ("no-offline", "start-of-week"). It fills shared args plus
//     one typed command-args struct with every value the CLI did NOT set.
//
//   - ApplyToFlags(agent, report, flags, shared) is what the interim cobra
//     hook (internal/cli.WireCommandConfig) calls: it pushes config values
//     through the parsed pflag.FlagSet with flags.Set, so flag-bound variables
//     observe them exactly like CLI input. CLI-set flags are never touched.
//
// Both reproduce the reference precedence CLI > config > defaults, with env
// (CCUSAGE_OFFLINE and friends) still resolved later at use time.
package config

import (
	"os"
	"strconv"

	"github.com/spf13/pflag"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// DailyArgs mirrors the daily-specific report args.
type DailyArgs struct {
	Instances      bool
	Project        *string
	ProjectAliases *string
}

// WeeklyArgs mirrors the weekly-specific report args.
type WeeklyArgs struct {
	StartOfWeek core.WeekDay
}

// BlocksArgs mirrors the blocks-specific report args; SessionLength should
// start from blocks.DefaultSessionDurationHours.
type BlocksArgs struct {
	Active        bool
	Recent        bool
	TokenLimit    *string
	SessionLength float64
}

// StatuslineArgs mirrors the reference StatuslineArgs; NewStatuslineArgs
// carries its defaults.
type StatuslineArgs struct {
	Offline                bool
	NoOffline              bool
	VisualBurnRate         VisualBurnRate
	CostSource             CostSource
	Cache                  bool
	NoCache                bool
	RefreshInterval        uint64
	ContextLowThreshold    uint8
	ContextMediumThreshold uint8
	Timezone               *string
	Debug                  bool
	ModelLabelAliases      map[string]string
}

// NewStatuslineArgs mirrors StatuslineArgs::default().
func NewStatuslineArgs() StatuslineArgs {
	return StatuslineArgs{
		Offline:                true,
		VisualBurnRate:         VisualBurnRateOff,
		CostSource:             CostSourceAuto,
		Cache:                  true,
		RefreshInterval:        1,
		ContextLowThreshold:    50,
		ContextMediumThreshold: 80,
	}
}

// AgentArgs mirrors the agent-adapter specific args.
type AgentArgs struct {
	CodexSpeed   CodexSpeed
	PIPath       *string
	OpenClawPath *string
}

// keyToFlag maps config option keys (camelCase) onto CLI flag names; it is the
// vocabulary ApplyConfig expects from its `changed` callback.
var keyToFlag = map[string]string{
	"since":                  "since",
	"until":                  "until",
	"json":                   "json",
	"mode":                   "mode",
	"debug":                  "debug",
	"debugSamples":           "debug-samples",
	"order":                  "order",
	"breakdown":              "breakdown",
	"offline":                "offline",
	"noOffline":              "no-offline",
	"color":                  "color",
	"noColor":                "no-color",
	"timezone":               "timezone",
	"jq":                     "jq",
	"compact":                "compact",
	"singleThread":           "single-thread",
	"noCost":                 "no-cost",
	"instances":              "instances",
	"project":                "project",
	"projectAliases":         "project-aliases",
	"startOfWeek":            "start-of-week",
	"active":                 "active",
	"recent":                 "recent",
	"tokenLimit":             "token-limit",
	"sessionLength":          "session-length",
	"visualBurnRate":         "visual-burn-rate",
	"costSource":             "cost-source",
	"cache":                  "cache",
	"noCache":                "no-cache",
	"refreshInterval":        "refresh-interval",
	"contextLowThreshold":    "context-low-threshold",
	"contextMediumThreshold": "context-medium-threshold",
	"speed":                  "speed",
	"piPath":                 "pi-path",
	"openClawPath":           "open-claw-path",
}

// FlagName maps a config option key to its CLI flag name (identity when the
// key has no flag, e.g. pricingOverrides).
func FlagName(key string) string {
	if flag, ok := keyToFlag[key]; ok {
		return flag
	}
	return key
}

// ApplyConfig applies the discovered ccusage.json onto the parsed args.
//
// command is the report name (daily, weekly, monthly, session, blocks,
// statusline); agent is the agent name for agent commands ("claude") and ""
// for all-agent reports. The config file comes from --config in os.Args or
// discovery. changed(name) receives a flag name and reports whether the CLI
// set it explicitly; nil means "nothing was set". commandArgs optionally
// carries exactly one of *DailyArgs, *WeeklyArgs, *BlocksArgs,
// *StatuslineArgs, or *AgentArgs.
func ApplyConfig(command, agent string, shared *core.SharedArgs, changed func(string) bool, commandArgs ...any) {
	raw := command
	if agent != "" {
		raw = agent + " " + command
	}
	ctx := FromArgs(os.Args[1:])
	maps := ctx.OptionMapsFor(raw, agent, command)
	isSet := func(flag string) bool { return false }
	if changed != nil {
		isSet = func(key string) bool { return changed(FlagName(key)) }
	}
	applyMaps(maps, shared, isSet, commandArgs, ctx)
}

// ApplyToFlags applies the discovered config through a parsed flag set: every
// option whose flag exists on the command and was not set by the CLI is
// assigned via flags.Set so the flag-bound variables observe it. Pricing
// overrides and pi.stores land on shared directly. The returned error is the
// fatal pi.stores error for the all-agent reports (nil otherwise).
func ApplyToFlags(agent, report string, flags *pflag.FlagSet, shared *core.SharedArgs) error {
	raw := report
	if agent != "" {
		raw = agent + " " + report
	}
	ctx := FromArgs(os.Args[1:])
	maps := ctx.OptionMapsFor(raw, agent, report)

	cliSet := map[string]bool{}
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			cliSet[f.Name] = true
		}
	})
	for _, m := range maps {
		applyMapToFlags(m, flags, cliSet)
	}
	if shared != nil {
		for _, m := range maps {
			if overrides := SharedOptionsFromMap(m).PricingOverrides; overrides != nil {
				if shared.PricingOverrides == nil {
					shared.PricingOverrides = map[string]core.PricingOverride{}
				}
				MergePricingOverrides(shared.PricingOverrides, overrides)
			}
		}
		if ctx.err == nil {
			shared.PIStores = ctx.piStores
		}
	}
	if ctx.err != nil && CommandUsesNamedPIStores(agent, report) {
		return ctx.err
	}
	return nil
}

func applyMaps(maps []map[string]any, shared *core.SharedArgs, isSet func(string) bool, commandArgs []any, ctx *Context) {
	if shared != nil {
		for _, m := range maps {
			applySharedOptions(shared, SharedOptionsFromMap(m), isSet)
		}
		if ctx != nil && ctx.err == nil {
			shared.PIStores = ctx.piStores
		}
	}
	for _, args := range commandArgs {
		switch a := args.(type) {
		case *DailyArgs:
			for _, m := range maps {
				applyDailyOptions(a, DailySpecificOptionsFromMap(m), isSet)
			}
		case *WeeklyArgs:
			for _, m := range maps {
				applyWeeklyOptions(a, WeeklySpecificOptionsFromMap(m), isSet)
			}
		case *BlocksArgs:
			for _, m := range maps {
				applyBlocksOptions(a, BlocksSpecificOptionsFromMap(m), isSet)
			}
		case *StatuslineArgs:
			for _, m := range maps {
				applyStatuslineOptions(a, StatuslineSpecificOptionsFromMap(m), isSet)
			}
		case *AgentArgs:
			for _, m := range maps {
				applyAgentOptions(a, m, isSet)
			}
		}
	}
}

func applySharedOptions(shared *core.SharedArgs, opts SharedOptions, isSet func(string) bool) {
	if opts.Since != nil && !isSet("since") {
		value := NormalizeDateBound(*opts.Since)
		shared.Since = &value
	}
	if opts.Until != nil && !isSet("until") {
		value := NormalizeDateBound(*opts.Until)
		shared.Until = &value
	}
	if opts.JSON != nil && !isSet("json") {
		shared.JSON = *opts.JSON
	}
	if opts.Mode != nil && !isSet("mode") {
		shared.Mode = *opts.Mode
	}
	if opts.Debug != nil && !isSet("debug") {
		shared.Debug = *opts.Debug
	}
	if opts.DebugSamples != nil && !isSet("debugSamples") {
		shared.DebugSamples = int(*opts.DebugSamples)
	}
	if opts.Order != nil && !isSet("order") {
		shared.Order = *opts.Order
	}
	if opts.Breakdown != nil && !isSet("breakdown") {
		shared.Breakdown = *opts.Breakdown
	}
	if opts.Offline != nil && !isSet("offline") {
		shared.Offline = *opts.Offline
	}
	if opts.NoOffline != nil && !isSet("noOffline") {
		shared.NoOffline = *opts.NoOffline
	}
	if opts.Color != nil && !isSet("color") {
		shared.Color = *opts.Color
	}
	if opts.NoColor != nil && !isSet("noColor") {
		shared.NoColor = *opts.NoColor
	}
	if opts.Timezone != nil && !isSet("timezone") {
		shared.Timezone = opts.Timezone
	}
	if opts.JQ != nil && !isSet("jq") {
		shared.JQ = opts.JQ
	}
	if opts.Compact != nil && !isSet("compact") {
		shared.Compact = *opts.Compact
	}
	if opts.SingleThread != nil && !isSet("singleThread") {
		shared.SingleThread = *opts.SingleThread
	}
	if opts.NoCost != nil && !isSet("noCost") {
		shared.NoCost = *opts.NoCost
	}
	if opts.PricingOverrides != nil {
		if shared.PricingOverrides == nil {
			shared.PricingOverrides = map[string]core.PricingOverride{}
		}
		MergePricingOverrides(shared.PricingOverrides, opts.PricingOverrides)
	}
	// opts.All is accepted for compatibility and intentionally not applied,
	// matching the reference.
}

func applyDailyOptions(args *DailyArgs, opts DailySpecificOptions, isSet func(string) bool) {
	if opts.Instances != nil && !isSet("instances") {
		args.Instances = *opts.Instances
	}
	if opts.Project != nil && !isSet("project") {
		args.Project = opts.Project
	}
	if opts.ProjectAliases != nil && !isSet("projectAliases") {
		args.ProjectAliases = opts.ProjectAliases
	}
}

func applyWeeklyOptions(args *WeeklyArgs, opts WeeklySpecificOptions, isSet func(string) bool) {
	if opts.StartOfWeek != nil && !isSet("startOfWeek") {
		args.StartOfWeek = *opts.StartOfWeek
	}
}

func applyBlocksOptions(args *BlocksArgs, opts BlocksSpecificOptions, isSet func(string) bool) {
	if opts.Active != nil && !isSet("active") {
		args.Active = *opts.Active
	}
	if opts.Recent != nil && !isSet("recent") {
		args.Recent = *opts.Recent
	}
	if opts.TokenLimit != nil && !isSet("tokenLimit") {
		args.TokenLimit = opts.TokenLimit
	}
	if opts.SessionLength != nil && !isSet("sessionLength") {
		args.SessionLength = *opts.SessionLength
	}
}

func applyStatuslineOptions(args *StatuslineArgs, opts StatuslineSpecificOptions, isSet func(string) bool) {
	if opts.Offline != nil && !isSet("offline") {
		args.Offline = *opts.Offline
	}
	if opts.NoOffline != nil && !isSet("noOffline") {
		args.NoOffline = *opts.NoOffline
	}
	if opts.VisualBurnRate != nil && !isSet("visualBurnRate") {
		args.VisualBurnRate = *opts.VisualBurnRate
	}
	if opts.CostSource != nil && !isSet("costSource") {
		args.CostSource = *opts.CostSource
	}
	if opts.Cache != nil && !isSet("cache") {
		args.Cache = *opts.Cache
	}
	if opts.NoCache != nil && !isSet("noCache") {
		args.NoCache = *opts.NoCache
	}
	if opts.RefreshInterval != nil && !isSet("refreshInterval") {
		args.RefreshInterval = *opts.RefreshInterval
	}
	// Thresholds above 255 do not fit u8 and are dropped, exactly like the
	// reference's u8::try_from(...).ok().
	if opts.ContextLowThreshold != nil && *opts.ContextLowThreshold <= 255 && !isSet("contextLowThreshold") {
		args.ContextLowThreshold = uint8(*opts.ContextLowThreshold)
	}
	if opts.ContextMediumThreshold != nil && *opts.ContextMediumThreshold <= 255 && !isSet("contextMediumThreshold") {
		args.ContextMediumThreshold = uint8(*opts.ContextMediumThreshold)
	}
	if opts.Timezone != nil && !isSet("timezone") {
		args.Timezone = opts.Timezone
	}
	if opts.Debug != nil && !isSet("debug") {
		args.Debug = *opts.Debug
	}
	if opts.ModelLabelAliases != nil {
		args.ModelLabelAliases = opts.ModelLabelAliases
	}
}

func applyAgentOptions(args *AgentArgs, m map[string]any, isSet func(string) bool) {
	if speed := CodexSpecificOptionsFromMap(m).Speed; speed != nil && !isSet("speed") {
		args.CodexSpeed = *speed
	}
	if path := PiSpecificOptionsFromMap(m).PIPath; path != nil && !isSet("piPath") {
		args.PIPath = path
	}
	if path := OpenClawSpecificOptionsFromMap(m).OpenClawPath; path != nil && !isSet("openClawPath") {
		args.OpenClawPath = path
	}
}

// ---------------------------------------------------------------------------
// Flag-set application (interim cobra hook)
// ---------------------------------------------------------------------------

type flagSpec struct {
	flag string
	get  func(m map[string]any) (string, bool)
}

func boolFlagSpec(key, flag string) flagSpec {
	return flagSpec{flag, func(m map[string]any) (string, bool) {
		if value := boolOption(m, key); value != nil {
			return strconv.FormatBool(*value), true
		}
		return "", false
	}}
}

func stringFlagSpec(key, flag string) flagSpec {
	return flagSpec{flag, func(m map[string]any) (string, bool) {
		if value := stringOption(m, key); value != nil {
			return *value, true
		}
		return "", false
	}}
}

func dateFlagSpec(key, flag string) flagSpec {
	return flagSpec{flag, func(m map[string]any) (string, bool) {
		if value := stringOption(m, key); value != nil {
			return NormalizeDateBound(*value), true
		}
		return "", false
	}}
}

func enumFlagSpec(key, flag string, valid func(string) bool) flagSpec {
	return flagSpec{flag, func(m map[string]any) (string, bool) {
		value := stringOption(m, key)
		if value == nil || !valid(*value) {
			return "", false
		}
		return *value, true
	}}
}

func u64FlagSpec(key, flag string) flagSpec {
	return flagSpec{flag, func(m map[string]any) (string, bool) {
		if value := u64Option(m, key); value != nil {
			return strconv.FormatUint(*value, 10), true
		}
		return "", false
	}}
}

func thresholdFlagSpec(key, flag string) flagSpec {
	return flagSpec{flag, func(m map[string]any) (string, bool) {
		value := u64Option(m, key)
		if value == nil || *value > 255 {
			return "", false
		}
		return strconv.FormatUint(*value, 10), true
	}}
}

func f64FlagSpec(key, flag string) flagSpec {
	return flagSpec{flag, func(m map[string]any) (string, bool) {
		if value := f64Option(m, key); value != nil {
			return strconv.FormatFloat(*value, 'f', -1, 64), true
		}
		return "", false
	}}
}

// flagSpecs maps every flag-assignable config key to its flag formatting.
var flagSpecs = []flagSpec{
	dateFlagSpec("since", "since"),
	dateFlagSpec("until", "until"),
	boolFlagSpec("json", "json"),
	enumFlagSpec("mode", "mode", func(s string) bool { _, ok := core.ParseCostMode(s); return ok }),
	boolFlagSpec("debug", "debug"),
	u64FlagSpec("debugSamples", "debug-samples"),
	enumFlagSpec("order", "order", func(s string) bool { _, ok := core.ParseSortOrder(s); return ok }),
	boolFlagSpec("breakdown", "breakdown"),
	boolFlagSpec("offline", "offline"),
	boolFlagSpec("noOffline", "no-offline"),
	boolFlagSpec("color", "color"),
	boolFlagSpec("noColor", "no-color"),
	stringFlagSpec("timezone", "timezone"),
	stringFlagSpec("jq", "jq"),
	boolFlagSpec("compact", "compact"),
	boolFlagSpec("singleThread", "single-thread"),
	boolFlagSpec("noCost", "no-cost"),

	boolFlagSpec("instances", "instances"),
	stringFlagSpec("project", "project"),
	stringFlagSpec("projectAliases", "project-aliases"),

	enumFlagSpec("startOfWeek", "start-of-week", func(s string) bool {
		_, ok := core.ParseWeekDay(s)
		return ok
	}),

	boolFlagSpec("active", "active"),
	boolFlagSpec("recent", "recent"),
	stringFlagSpec("tokenLimit", "token-limit"),
	f64FlagSpec("sessionLength", "session-length"),

	boolFlagSpec("cache", "cache"),
	boolFlagSpec("noCache", "no-cache"),
	u64FlagSpec("refreshInterval", "refresh-interval"),
	thresholdFlagSpec("contextLowThreshold", "context-low-threshold"),
	thresholdFlagSpec("contextMediumThreshold", "context-medium-threshold"),
	enumFlagSpec("visualBurnRate", "visual-burn-rate", func(s string) bool {
		_, ok := ParseVisualBurnRate(s)
		return ok
	}),
	enumFlagSpec("costSource", "cost-source", func(s string) bool {
		_, ok := ParseCostSource(s)
		return ok
	}),

	enumFlagSpec("speed", "speed", func(s string) bool {
		_, ok := ParseCodexSpeed(s)
		return ok
	}),
	stringFlagSpec("piPath", "pi-path"),
	stringFlagSpec("openClawPath", "open-claw-path"),
}

// applyMapToFlags pushes one option map through the flag set. cliSet holds the
// flags the CLI set explicitly (captured before any config Set, since Set also
// marks flags changed); flags absent from the command are skipped.
func applyMapToFlags(m map[string]any, flags *pflag.FlagSet, cliSet map[string]bool) {
	for _, spec := range flagSpecs {
		if cliSet[spec.flag] {
			continue
		}
		value, ok := spec.get(m)
		if !ok {
			continue
		}
		if flags.Lookup(spec.flag) == nil {
			continue
		}
		_ = flags.Set(spec.flag, value)
	}
}
