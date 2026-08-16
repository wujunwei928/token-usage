package qwen

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is Qwen's report profile for the shared pipeline: Sunday weeks,
// activity-bounded sessions filtered after summarizing so a session whose
// last activity reaches into the window survives whole.
var Profile = common.ReportProfile{
	SessionByActivity:  true,
	SessionFilterAfter: true,
}

// adapter exposes the Qwen loader through the unified Agent Adapter
// interface (ADR 0009). Qwen prices internally, so the factory injects no
// pricing table.
type adapter struct{}

func (adapter) Agent() string { return "qwen" }

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
	common.RegisterAgent("qwen", func(shared *core.SharedArgs) common.Adapter {
		return adapter{}
	})
}
