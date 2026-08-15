package all

import (
	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/adapter/goose"
	"github.com/wujunwei/ccusage-go/internal/adapter/kilo"
	"github.com/wujunwei/ccusage-go/internal/adapter/pi"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// pi participates in the unified report at roster index 7 (includeProjectPath
// metadata like the reference's load_pi_format_agent_rows).
func init() {
	RegisterSpec(7, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 7,
			Agent: "pi",
			Load: func(kind ReportKind) (AgentRows, error) {
				pricing := loadPricingForAdapters(shared)
				entries, err := pi.LoadEntries(pi.LoadOptions{Shared: shared, Pricing: pricing})
				if err != nil {
					return AgentRows{}, err
				}
				detected := len(entries) > 0
				entries = pi.FilterEntriesByDate(entries, shared)
				summaries := pi.SummarizeEntries(entries, toPiKind(kind))
				return AgentRows{
					Rows:     SummaryRows("pi", summaries, true),
					Detected: detected,
				}, nil
			},
		}
	})
}

func toPiKind(kind ReportKind) pi.ReportKind {
	switch kind {
	case KindWeekly:
		return pi.ReportWeekly
	case KindMonthly:
		return pi.ReportMonthly
	case KindSession:
		return pi.ReportSession
	default:
		return pi.ReportDaily
	}
}

// goose participates at roster index 8.
func init() {
	RegisterSpec(8, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 8,
			Agent: "goose",
			Load: func(kind ReportKind) (AgentRows, error) {
				pricing := loadPricingForAdapters(shared)
				entries, err := goose.LoadEntries(shared, pricing)
				if err != nil {
					return AgentRows{}, err
				}
				detected := len(entries) > 0
				entries = common.FilterLoadedEntriesByDate(entries, shared)
				summaries := goose.SummarizeEntries(entries, toGooseKind(kind))
				return AgentRows{
					Rows:     SummaryRows("goose", summaries, false),
					Detected: detected,
				}, nil
			},
		}
	})
}

func toGooseKind(kind ReportKind) goose.ReportKind {
	switch kind {
	case KindWeekly:
		return goose.ReportWeekly
	case KindMonthly:
		return goose.ReportMonthly
	case KindSession:
		return goose.ReportSession
	default:
		return goose.ReportDaily
	}
}

// kilo participates at roster index 10.
func init() {
	RegisterSpec(10, func(shared *core.SharedArgs) Spec {
		return Spec{
			Index: 10,
			Agent: "kilo",
			Load: func(kind ReportKind) (AgentRows, error) {
				pricing := loadPricingForAdapters(shared)
				entries, err := kilo.LoadEntries(shared, pricing)
				if err != nil {
					return AgentRows{}, err
				}
				detected := len(entries) > 0
				entries = kilo.FilterEntriesByDate(entries, shared)
				summaries := kilo.SummarizeEntries(entries, toKiloKind(kind))
				return AgentRows{
					Rows:     SummaryRows("kilo", summaries, false),
					Detected: detected,
				}, nil
			},
		}
	})
}

func toKiloKind(kind ReportKind) kilo.ReportKind {
	switch kind {
	case KindWeekly:
		return kilo.ReportWeekly
	case KindMonthly:
		return kilo.ReportMonthly
	case KindSession:
		return kilo.ReportSession
	default:
		return kilo.ReportDaily
	}
}

// loadPricingForAdapters loads the pricing map the same way the other
// adapters' specs do (display mode skips pricing).
func loadPricingForAdapters(shared *core.SharedArgs) *core.PricingMap {
	if shared.Mode == core.ModeDisplay {
		return nil
	}
	return core.LoadWithOverrides(shared.OfflineEffective(), core.LogLevel() != nil && *core.LogLevel() == 0, shared.PricingOverrides)
}
