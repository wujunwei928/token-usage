package omp

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is omp's report profile: like pi (whose session format omp shares),
// Sunday weeks and activity-bounded sessions filtered before summarizing.
var Profile = common.ReportProfile{
	SessionByActivity: true,
}

// adapter exposes the omp loader through the unified Agent Adapter interface
// (ADR 0009). Like zcode (ADR 0006's beyond-upstream pattern), the Detected
// verdict counts the sessions directory itself so a fully date-filtered run
// still reports the agent as present on the machine.
type adapter struct{}

func (adapter) Agent() string { return "omp" }

func (adapter) HasData() bool { return HasData() }

func (adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(LoadOptions{Shared: req.Shared, Pricing: common.LoadPricingDisplayGated(req.Shared)})
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{Entries: entries, Detected: len(entries) > 0 || HasData()}, nil
}

func init() {
	common.RegisterAgent("omp", func(shared *core.SharedArgs) common.Adapter {
		return adapter{}
	})
}
