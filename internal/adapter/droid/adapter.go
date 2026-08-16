package droid

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is Droid's report profile for the shared pipeline: Sunday weeks,
// key-swap sessions, filter before summarize — the zero value.
var Profile = common.ReportProfile{}

// adapter exposes the Droid loader through the unified Agent Adapter
// interface (ADR 0009). Droid prices internally, so the factory injects no
// pricing table.
type adapter struct{}

func (adapter) Agent() string { return "droid" }

func (adapter) HasData() bool { return false }

func (adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(req.Shared)
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{Entries: entries, Detected: len(entries) > 0}, nil
}

func init() {
	common.RegisterAgent("droid", func(shared *core.SharedArgs) common.Adapter {
		return adapter{}
	})
}
