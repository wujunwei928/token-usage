package amp

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is Amp's report profile for the shared pipeline: Sunday weeks,
// key-swap sessions, filter before summarize — the zero value.
var Profile = common.ReportProfile{}

// adapter exposes the Amp loader through the unified Agent Adapter interface
// (ADR 0009). The factory loads the pricing table with the raw --offline flag
// (no --no-offline fold), the semantics Amp's spec carried.
type adapter struct {
	pricing *core.PricingMap
}

func (adapter) Agent() string { return "amp" }

func (adapter) HasData() bool { return false }

func (a adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(req.Shared, a.pricing)
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{Entries: entries, Detected: len(entries) > 0}, nil
}

func init() {
	common.RegisterAgent("amp", func(shared *core.SharedArgs) common.Adapter {
		return adapter{pricing: common.LoadPricingRawOffline(shared)}
	})
}
