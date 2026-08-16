package claude

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is Claude's report profile for the shared entries pipeline:
// Sunday weeks, activity-bounded sessions filtered after summarizing. The
// unified report itself keeps Claude's hand-written spec (the daily pipeline
// and its dedup side-channel are its own — ADR 0009's permanent exception).
var Profile = common.ReportProfile{
	SessionByActivity:  true,
	SessionFilterAfter: true,
}

// adapter exposes the Claude loader through the unified Agent Adapter
// interface (ADR 0009). Claude prices internally; the project filter stays a
// CLI-flag concern (nil for registry loads).
type adapter struct{}

func (adapter) Agent() string { return "claude" }

func (adapter) HasData() bool { return false }

func (adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(LoadOptions{Shared: req.Shared})
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{Entries: entries, Detected: len(entries) > 0}, nil
}

func init() {
	common.RegisterAgent("claude", func(shared *core.SharedArgs) common.Adapter {
		return adapter{}
	})
}
