package gemini

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is Gemini's report profile for the shared pipeline: Sunday weeks,
// key-swap sessions, filter before summarize — the zero value.
var Profile = common.ReportProfile{}

// adapter exposes the Gemini loader through the unified Agent Adapter
// interface (ADR 0009), with the unified pricing semantics (always a full
// map, never suppressed by display mode).
type adapter struct {
	pricing *core.PricingMap
}

func (adapter) Agent() string { return "gemini" }

func (adapter) HasData() bool { return false }

func (a adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(req.Shared, a.pricing)
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{Entries: entries, Detected: len(entries) > 0}, nil
}

func init() {
	common.RegisterAgent("gemini", func(shared *core.SharedArgs) common.Adapter {
		return adapter{pricing: common.LoadPricingRawOffline(shared)}
	})
}
