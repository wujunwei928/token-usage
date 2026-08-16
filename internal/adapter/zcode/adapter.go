package zcode

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is zcode's report profile for the shared pipeline: Monday weeks,
// activity-bounded sessions filtered after summarizing.
var Profile = common.ReportProfile{
	WeekStart:          core.Monday,
	SessionByActivity:  true,
	SessionFilterAfter: true,
}

// adapter exposes the zcode SQLite loader through the unified Agent Adapter
// interface (ADR 0009). zcode prices internally (no pricing source: cost
// stays 0 with MissingPricingModel markers), so the factory injects no
// pricing table.
type adapter struct{}

func (adapter) Agent() string { return "zcode" }

func (adapter) HasData() bool { return HasData() }

func (adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(req.Shared)
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{
		Entries:  entries,
		Detected: len(entries) > 0 || HasData(),
	}, nil
}

func init() {
	common.RegisterAgent("zcode", func(shared *core.SharedArgs) common.Adapter {
		return adapter{}
	})
}
