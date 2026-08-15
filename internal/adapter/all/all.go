// Package all builds the unified cross-agent report (the All-Report from
// CONTEXT.md): every agent adapter contributes rows that are merged per period.
package all

import (
	"sort"
	"strings"
	"sync"

	"github.com/wujunwei/ccusage-go/internal/adapter/claude"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// ReportKind selects the report granularity (also the --sections vocabulary).
type ReportKind int

// Report kinds.
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// String returns the JSON key and CLI name of the kind.
func (k ReportKind) String() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "session"
	}
}

// ParseReportKind maps a --sections token; ok=false for unknown names.
func ParseReportKind(value string) (ReportKind, bool) {
	switch value {
	case "daily":
		return KindDaily, true
	case "weekly":
		return KindWeekly, true
	case "monthly":
		return KindMonthly, true
	case "session":
		return KindSession, true
	}
	return KindDaily, false
}

// Row is one merged (or per-agent) report row.
type Row struct {
	Period          string
	Agent           string
	ModelsUsed      []string
	InputTokens     uint64
	OutputTokens    uint64
	CacheCreation   uint64
	CacheRead       uint64
	TotalTokens     uint64
	TotalCost       float64
	Metadata        *core.J
	MetadataAgents  []string
	AgentBreakdowns *[]Row
	ModelBreakdowns []core.ModelBreakdown
}

// AgentRows is one adapter's contribution plus whether it found data.
type AgentRows struct {
	Rows     []Row
	Detected bool
}

// Spec describes one adapter's participation in the unified load.
type Spec struct {
	Index int
	Agent string
	Load  func(kind ReportKind) (AgentRows, error)
}

// registeredSpecs collects adapters that plugged themselves in via
// RegisterSpec; each adapter owns its own spec_<agent>.go file.
var registeredSpecs = map[int]func(shared *core.SharedArgs) Spec{}

// RegisterSpec installs an adapter's unified loader factory at its roster index.
func RegisterSpec(index int, factory func(shared *core.SharedArgs) Spec) {
	registeredSpecs[index] = factory
}

func notImplemented(kind ReportKind) (AgentRows, error) {
	return AgentRows{}, nil
}

// builtInAgentNames is the roster order the reference uses for specs.
var builtInAgentNames = []string{
	"claude", "codex", "opencode", "amp", "droid", "codebuff", "hermes",
	"pi", "goose", "openclaw", "kilo", "copilot", "gemini", "kimi", "qwen", "grok",
}

// BuiltInSpecs returns the adapter roster in reference order; adapters that
// have not landed yet contribute empty loads.
func BuiltInSpecs(shared *core.SharedArgs) []Spec {
	specs := make([]Spec, len(builtInAgentNames))
	for index, agent := range builtInAgentNames {
		if factory, ok := registeredSpecs[index]; ok {
			specs[index] = factory(shared)
			continue
		}
		if agent == "claude" {
			specs[index] = Spec{index, agent, func(kind ReportKind) (AgentRows, error) {
				return loadClaudeRows(kind, shared)
			}}
		} else {
			specs[index] = Spec{index, agent, notImplemented}
		}
	}
	return specs
}

// LoadResult carries merged base rows and the detected agent labels.
type LoadResult struct {
	Rows          []Row
	DetectedAgents []string
}

// LoadBaseRows runs every adapter in parallel and concatenates rows in roster
// order. loadKind is Daily for the daily family and Session for sessions.
func LoadBaseRows(loadKind ReportKind, shared *core.SharedArgs, specs []Spec) (*LoadResult, error) {
	type outcome struct {
		index int
		agent string
		rows  AgentRows
		err   error
	}
	outcomes := make([]outcome, len(specs))
	var wg sync.WaitGroup
	for i := range specs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rows, err := specs[i].Load(loadKind)
			outcomes[i] = outcome{specs[i].Index, specs[i].Agent, rows, err}
		}(i)
	}
	wg.Wait()
	for _, o := range outcomes {
		if o.err != nil {
			return nil, o.err
		}
	}
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].index < outcomes[j].index })
	result := &LoadResult{}
	for _, o := range outcomes {
		if o.rows.Detected {
			result.DetectedAgents = append(result.DetectedAgents, o.agent)
		}
		result.Rows = append(result.Rows, o.rows.Rows...)
	}
	return result, nil
}

// FinishRows turns base rows into the final report rows: sessions pass through
// (metadata agents dropped), the daily family aggregates per period.
func FinishRows(kind ReportKind, rows []Row, shared *core.SharedArgs) []Row {
	if kind == KindSession {
		for i := range rows {
			rows[i].MetadataAgents = nil
		}
		SortRows(rows, shared.Order)
		return rows
	}
	aggregated := AggregateRows(rows, kind)
	SortRows(aggregated, shared.Order)
	return aggregated
}

// SortRows orders by period then agent, reversing for descending order.
func SortRows(rows []Row, order core.SortOrder) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Period != rows[j].Period {
			return rows[i].Period < rows[j].Period
		}
		return rows[i].Agent < rows[j].Agent
	})
	if order == core.OrderDesc {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
}

// AggregateRows merges rows per period; weekly buckets always start Monday.
func AggregateRows(rows []Row, kind ReportKind) []Row {
	groups := map[string]*accumulator{}
	var periods []string
	for _, row := range rows {
		period := row.Period
		switch kind {
		case KindMonthly:
			if len(period) >= 7 {
				period = period[:7]
			}
		case KindWeekly:
			if week := core.WeekStart(period, core.Monday); week != "" {
				period = week
			}
		}
		row.Period = period
		acc, ok := groups[period]
		if !ok {
			acc = &accumulator{}
			groups[period] = acc
			periods = append(periods, period)
		}
		acc.add(row)
	}
	sort.Strings(periods)
	out := make([]Row, 0, len(periods))
	for _, period := range periods {
		out = append(out, groups[period].intoRow(period))
	}
	return out
}

func loadClaudeRows(kind ReportKind, shared *core.SharedArgs) (AgentRows, error) {
	if kind == KindSession {
		entries, err := claude.LoadEntries(claude.LoadOptions{Shared: shared})
		if err != nil {
			return AgentRows{}, err
		}
		detected := len(entries) > 0
		summaries := summarizeEntrySessions(entries)
		summaries = filterSessionSummaries(summaries, shared)
		return AgentRows{Rows: SummaryRows("claude", summaries, false), Detected: detected}, nil
	}
	summaries, err := claude.LoadDailySummaries(shared, nil, false)
	if err != nil {
		return AgentRows{}, err
	}
	detected := len(summaries) > 0
	summaries = filterDailySummariesByDate(summaries, shared)
	return AgentRows{Rows: SummaryRows("claude", summaries, false), Detected: detected}, nil
}

func summarizeEntrySessions(entries []core.LoadedEntry) []core.UsageSummary {
	type key struct{ projectPath, sessionID string }
	groups := map[key]*core.SessionAccumulator{}
	var order []key
	for i := range entries {
		k := key{entries[i].ProjectPath, entries[i].SessionID}
		if _, ok := groups[k]; !ok {
			groups[k] = &core.SessionAccumulator{}
			order = append(order, k)
		}
		groups[k].AddEntry(&entries[i])
	}
	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].projectPath != keys[j].projectPath {
			return keys[i].projectPath < keys[j].projectPath
		}
		return keys[i].sessionID < keys[j].sessionID
	})
	out := make([]core.UsageSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, groups[k].IntoSummary())
	}
	return out
}

func filterSessionSummaries(rows []core.UsageSummary, shared *core.SharedArgs) []core.UsageSummary {
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

func filterDailySummariesByDate(rows []core.UsageSummary, shared *core.SharedArgs) []core.UsageSummary {
	if shared.Since == nil && shared.Until == nil {
		return rows
	}
	out := make([]core.UsageSummary, 0, len(rows))
	for i := range rows {
		date := ""
		if rows[i].Date != nil {
			date = strings.ReplaceAll(*rows[i].Date, "-", "")
		}
		if core.DateWithinRange(date, shared.Since, shared.Until) {
			out = append(out, rows[i])
		}
	}
	return out
}

// SummaryRows converts agent summaries to unified rows, dropping zero-token
// rows and attaching per-agent metadata.
func SummaryRows(agent string, summaries []core.UsageSummary, includeProjectPath bool) []Row {
	rows := make([]Row, 0, len(summaries))
	for i := range summaries {
		summary := &summaries[i]
		period := ""
		switch {
		case summary.Date != nil:
			period = *summary.Date
		case summary.Week != nil:
			period = *summary.Week
		case summary.Month != nil:
			period = *summary.Month
		case summary.SessionID != nil:
			period = *summary.SessionID
		default:
			continue
		}
		totalTokens := summary.TotalTokens()
		if totalTokens == 0 {
			continue
		}
		rows = append(rows, Row{
			Period:          period,
			Agent:           agent,
			ModelsUsed:      summary.ModelsUsed,
			InputTokens:     summary.InputTokens,
			OutputTokens:    summary.OutputTokens,
			CacheCreation:   summary.CacheCreationTokens,
			CacheRead:       summary.CacheReadTokens,
			TotalTokens:     totalTokens,
			TotalCost:       summary.TotalCost,
			Metadata:        summaryMetadata(summary, includeProjectPath),
			MetadataAgents:  []string{agent},
			ModelBreakdowns: summary.ModelBreakdowns,
		})
	}
	return rows
}

func summaryMetadata(summary *core.UsageSummary, includeProjectPath bool) *core.J {
	var fields []any
	if summary.Credits != nil {
		fields = append(fields, "credits", jsonFloatJ(*summary.Credits))
	}
	if summary.SessionID != nil {
		if summary.LastActivity != nil {
			fields = append(fields, "lastActivity", core.JStrV(*summary.LastActivity))
		}
		if includeProjectPath && summary.ProjectPath != nil {
			fields = append(fields, "projectPath", core.JStrV(*summary.ProjectPath))
		}
	}
	if len(fields) == 0 {
		return nil
	}
	ordered := core.JOrdObjV(fields...)
	return &ordered
}

// jsonFloatJ mirrors the reference json_float: whole floats render as
// integers.
func jsonFloatJ(v float64) core.J {
	if !isInfNaN(v) && v == trunc(v) && v >= -9223372036854775808.0 && v <= 9223372036854775807.0 {
		return core.JIntV(int64(v))
	}
	return core.JFloatV(v)
}
