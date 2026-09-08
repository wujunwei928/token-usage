package cline

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is Cline's report profile for the shared pipeline: Sunday weeks,
// activity-bounded sessions filtered after summarizing (session files carry
// per-message timestamps and the workspace cwd).
var Profile = common.ReportProfile{
	SessionByActivity:  true,
	SessionFilterAfter: true,
}

// adapter exposes the Cline loader through the unified Agent Adapter
// interface (ADR 0009).
type adapter struct {
	pricing *core.PricingMap
}

func (adapter) Agent() string { return "cline" }

func (adapter) HasData() bool { return HasData() }

func (a adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(req.Shared, a.pricing)
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{
		Entries:  entries,
		Detected: len(entries) > 0 || HasData(),
	}, nil
}

func init() {
	common.RegisterAgent("cline", func(shared *core.SharedArgs) common.Adapter {
		return adapter{pricing: common.LoadPricingRawOffline(shared)}
	})
}
