package cline

import (
	"sort"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries discovers Cline session files, parses them (in parallel unless
// disabled), dedups by message id, and sorts entries by timestamp.
func LoadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	return core.TrackUsageLoad("Cline", shared, func() ([]core.LoadedEntry, error) {
		return loadEntries(shared, pricing)
	})
}

func loadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	tz := core.ParseTZ(shared.Timezone)
	files := DiscoverMessageFiles()
	loaded := common.ReadFilesParallel(files, shared.SingleThread, readFilePair)
	var entries []core.LoadedEntry
	seen := map[string]bool{}
	for _, fileEntries := range loaded {
		for _, entry := range fileEntries {
			if entry.ID != "" {
				if seen[entry.ID] {
					continue
				}
				seen[entry.ID] = true
			}
			entries = append(entries, entryToLoaded(entry, tz, shared.Mode, pricing))
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

// entryToLoaded prices one entry and stamps the fixed Cline identity.
func entryToLoaded(entry clineEntry, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) core.LoadedEntry {
	model := entry.Model
	sessionID := entry.SessionID
	id := entry.ID
	projectPath := entry.CWD
	if projectPath == "" {
		projectPath = "cline"
	}
	cost := 0.0
	if mode != core.ModeDisplay {
		cost = clineCost(entry, pricing, mode)
	}
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionID,
			Timestamp: core.FormatRFC3339Millis(entry.Timestamp),
			Message: core.UsageMessage{
				Usage: entry.Usage,
				Model: &model,
				ID:    &id,
			},
			CostUSD: entry.CostUSD,
		},
		Timestamp:           entry.Timestamp,
		Date:                core.FormatDateTZ(entry.Timestamp, tz),
		Project:             "cline",
		SessionID:           sessionID,
		ProjectPath:         projectPath,
		Cost:                cost,
		Model:               &model,
		MissingPricingModel: clineMissingPricing(entry, mode, pricing),
	}
}

// clineCost prices one entry: a provider-reported metrics.cost wins in auto
// and display modes (the unified Cost Mode), otherwise tokens price from
// the first model candidate the pricing table knows.
func clineCost(entry clineEntry, pricing *core.PricingMap, mode core.CostMode) float64 {
	switch mode {
	case core.ModeDisplay:
		if entry.CostUSD != nil {
			return *entry.CostUSD
		}
		return 0
	case core.ModeAuto:
		if entry.CostUSD != nil {
			return *entry.CostUSD
		}
	}
	if name, ok := findPricedModel(entry, pricing); ok {
		return core.CalculateCostForUsage(&name, entry.Usage, nil, mode, pricing)
	}
	return 0
}

// clineMissingPricing flags models priced from tokens with no price; display
// mode and a provider-reported cost (auto mode) never consult pricing.
func clineMissingPricing(entry clineEntry, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay || (mode == core.ModeAuto && entry.CostUSD != nil) {
		return nil
	}
	if core.TotalUsageTokens(entry.Usage) == 0 || pricing == nil {
		return nil
	}
	if _, ok := findPricedModel(entry, pricing); ok {
		return nil
	}
	resolved := core.ResolveModelName(entry.Model)
	return &resolved
}

// findPricedModel returns the first model candidate the pricing table
// knows, so cost and missing-pricing share one resolution pass.
func findPricedModel(entry clineEntry, pricing *core.PricingMap) (string, bool) {
	for _, candidate := range clineModelCandidates(entry) {
		if pricing.Find(candidate) != nil {
			return candidate, true
		}
	}
	return "", false
}

// clineModelCandidates tries the bare model id before the provider-qualified
// name, so pricingOverrides can target either spelling.
func clineModelCandidates(entry clineEntry) []string {
	if entry.Provider == "" {
		return []string{entry.Model}
	}
	return []string{entry.Model, entry.Provider + "/" + entry.Model}
}
