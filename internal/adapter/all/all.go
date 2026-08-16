// Package all builds the unified cross-agent report (the All-Report from
// CONTEXT.md): every agent adapter contributes rows that are merged per period.
package all

import (
	"sort"
	"sync"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// ReportKind is the shared report vocabulary (also the --sections
// vocabulary); the unified report's kind is the same type every adapter and
// the CLI use (ADR 0009).
type ReportKind = core.ReportKind

// Report kinds.
const (
	KindDaily   = core.KindDaily
	KindWeekly  = core.KindWeekly
	KindMonthly = core.KindMonthly
	KindSession = core.KindSession
)

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
	Agent string
	Load  func(kind ReportKind) (AgentRows, error)
}

// registeredSpecs collects adapters that plugged themselves in via
// RegisterSpec; each adapter owns its own spec_<agent>.go file.
var registeredSpecs = map[string]func(shared *core.SharedArgs) Spec{}

// RegisterSpec installs an adapter's unified loader factory under its roster
// name. The display order comes from the roster itself, never from
// registration.
func RegisterSpec(agent string, factory func(shared *core.SharedArgs) Spec) {
	registeredSpecs[agent] = factory
}

func notImplemented(kind ReportKind) (AgentRows, error) {
	return AgentRows{}, nil
}

// BuiltInSpecs returns the adapter roster in unified-report order (the common
// roster, a single source); adapters that have not landed yet contribute
// empty loads.
func BuiltInSpecs(shared *core.SharedArgs) []Spec {
	names := common.Roster()
	specs := make([]Spec, len(names))
	for index, agent := range names {
		if factory, ok := registeredSpecs[agent]; ok {
			specs[index] = factory(shared)
			continue
		}
		specs[index] = Spec{agent, notImplemented}
	}
	return specs
}

// LoadResult carries merged base rows and the detected agent labels.
type LoadResult struct {
	Rows           []Row
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
			outcomes[i] = outcome{i, specs[i].Agent, rows, err}
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
