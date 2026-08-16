package pi

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is pi's report profile: Sunday weeks, activity-bounded sessions
// filtered before summarizing.
var Profile = common.ReportProfile{
	SessionByActivity: true,
}

// adapter exposes the pi loader through the unified Agent Adapter interface
// (ADR 0009). The factory captures the run's shared args and loads pricing
// with the display-mode gate the reference spec used (display mode skips
// pricing); the custom path stays nil for registry loads (CLI flag consumers
// construct their own LoadOptions).
type adapter struct{}

func adapterPricing(shared *core.SharedArgs) *core.PricingMap {
	return common.LoadPricingDisplayGated(shared)
}

func (adapter) Agent() string { return "pi" }

func (adapter) HasData() bool { return false }

func (adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(LoadOptions{Shared: req.Shared, Pricing: adapterPricing(req.Shared)})
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{Entries: entries, Detected: len(entries) > 0}, nil
}

func init() {
	common.RegisterAgent("pi", func(shared *core.SharedArgs) common.Adapter {
		return adapter{}
	})
}
