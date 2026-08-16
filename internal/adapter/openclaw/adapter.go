package openclaw

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is OpenClaw's report profile: Sunday weeks, activity-bounded
// sessions filtered before summarizing.
var Profile = common.ReportProfile{
	SessionByActivity: true,
}

// adapter exposes the OpenClaw loader through the unified Agent Adapter
// interface (ADR 0009). The unified report passes no custom path (nil), with
// the unified pricing semantics.
type adapter struct {
	pricing *core.PricingMap
}

func (adapter) Agent() string { return "openclaw" }

func (adapter) HasData() bool { return false }

func (a adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(req.Shared, nil, a.pricing)
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{Entries: entries, Detected: len(entries) > 0}, nil
}

func init() {
	common.RegisterAgent("openclaw", func(shared *core.SharedArgs) common.Adapter {
		return adapter{pricing: common.LoadPricingRawOffline(shared)}
	})
}
