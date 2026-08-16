package opencode

import (
	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// Profile is OpenCode's report profile: Monday weekly buckets, activity-
// bounded sessions filtered before summarizing (the loader narrows by the
// date window as it reads, so entries are windowed at load time).
var Profile = common.ReportProfile{
	WeekStart:         core.Monday,
	SessionByActivity: true,
}

// adapter exposes the OpenCode loader through the unified Agent Adapter
// interface (ADR 0009). The factory captures the run's shared args with JSON
// forced on (mirroring the pre-refactor spec; the loader narrows reads by
// the --since/--until date window, not by that flag); the captured clone
// replaces the request's args.
type adapter struct {
	loaderShared *core.SharedArgs
}

func (adapter) Agent() string { return "opencode" }

func (adapter) HasData() bool { return HasData() }

func (a adapter) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	entries, err := LoadEntries(a.loaderShared)
	if err != nil {
		return common.LoadResult{}, err
	}
	return common.LoadResult{
		Entries:  entries,
		Detected: len(entries) > 0 || HasData(),
	}, nil
}

func init() {
	common.RegisterAgent("opencode", func(shared *core.SharedArgs) common.Adapter {
		loaderShared := *shared
		loaderShared.JSON = true
		return adapter{loaderShared: &loaderShared}
	})
}
