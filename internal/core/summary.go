package core

import (
	"sort"
	"time"
)

// SummarizeByKey groups entries by keyFn (sorted by key) and stamps each group
// with the date/project returned by metaFn.
func SummarizeByKey(entries []LoadedEntry, keyFn func(*LoadedEntry) string, metaFn func(string) (string, *string)) []UsageSummary {
	groups := map[string]*usageAccumulator{}
	var keys []string
	for i := range entries {
		key := keyFn(&entries[i])
		acc, ok := groups[key]
		if !ok {
			acc = &usageAccumulator{}
			groups[key] = acc
			keys = append(keys, key)
		}
		acc.addEntry(&entries[i])
	}
	sort.Strings(keys)
	rows := make([]UsageSummary, 0, len(keys))
	for _, key := range keys {
		date, project := metaFn(key)
		summary := groups[key].intoSummary()
		summary.Date = &date
		summary.Project = project
		rows = append(rows, summary)
	}
	return rows
}

type usageAccumulator struct {
	counts         TokenCounts
	cost           float64
	credits        *float64
	messageCount   *uint64
	models         []string
	breakdowns     []ModelBreakdown
	breakdownIndex map[string]int
}

func (a *usageAccumulator) addEntry(entry *LoadedEntry) {
	usage := entry.Data.Message.Usage
	a.counts.AddUsage(usage)
	a.counts.ExtraTotalTokens += entry.ExtraTotalTokens
	a.cost += entry.Cost
	if entry.Credits != nil {
		if a.credits == nil {
			zero := 0.0
			a.credits = &zero
		}
		*a.credits += *entry.Credits
	}
	if entry.MessageCount != nil {
		if a.messageCount == nil {
			zero := uint64(0)
			a.messageCount = &zero
		}
		*a.messageCount += *entry.MessageCount
	}
	if entry.Model != nil {
		model := ResolveModelName(*entry.Model)
		if a.breakdownIndex == nil {
			a.breakdownIndex = map[string]int{}
		}
		index, ok := a.breakdownIndex[model]
		if !ok {
			index = len(a.breakdowns)
			a.breakdownIndex[model] = index
			a.models = append(a.models, model)
			a.breakdowns = append(a.breakdowns, ModelBreakdown{ModelName: model})
		}
		b := &a.breakdowns[index]
		b.InputTokens += usage.InputTokens
		b.OutputTokens += usage.OutputTokens
		b.CacheCreationTokens += usage.CacheCreationTokenCount()
		b.CacheReadTokens += usage.CacheReadInputTokens
		b.ExtraTotalTokens += entry.ExtraTotalTokens
		b.Cost += entry.Cost
		if entry.MissingPricingModel != nil {
			b.MissingPricing = true
		}
	}
}

func (a *usageAccumulator) intoSummary() UsageSummary {
	breakdowns := append([]ModelBreakdown(nil), a.breakdowns...)
	sort.SliceStable(breakdowns, func(i, j int) bool {
		// Descending by cost, matching total_cmp semantics for finite values.
		return breakdowns[i].Cost > breakdowns[j].Cost
	})
	return UsageSummary{
		InputTokens:         a.counts.InputTokens,
		OutputTokens:        a.counts.OutputTokens,
		CacheCreationTokens: a.counts.CacheCreationTokens,
		CacheReadTokens:     a.counts.CacheReadTokens,
		ExtraTotalTokens:    a.counts.ExtraTotalTokens,
		TotalCost:           a.cost,
		Credits:             a.credits,
		MessageCount:        a.messageCount,
		ModelsUsed:          a.models,
		ModelBreakdowns:     breakdowns,
	}
}

// SessionAccumulator groups by (project, session) tracking activity bounds.
type SessionAccumulator struct {
	usage    usageAccumulator
	latest   *struct {
		timestamp   int64
		sessionID   string
		projectPath string
	}
	earliest  *int64
	versions  map[string]struct{}
}

// AddEntry folds one entry into the session group.
func (s *SessionAccumulator) AddEntry(entry *LoadedEntry) {
	s.usage.addEntry(entry)
	if s.latest == nil || entry.Timestamp > s.latest.timestamp {
		s.latest = &struct {
			timestamp   int64
			sessionID   string
			projectPath string
		}{entry.Timestamp, entry.SessionID, entry.ProjectPath}
	}
	if s.earliest == nil || entry.Timestamp < *s.earliest {
		ts := entry.Timestamp
		s.earliest = &ts
	}
	if entry.Data.Version != nil {
		if s.versions == nil {
			s.versions = map[string]struct{}{}
		}
		s.versions[*entry.Data.Version] = struct{}{}
	}
}

// IntoSummary stamps the session identity and activity timestamps.
func (s *SessionAccumulator) IntoSummary() UsageSummary {
	summary := s.usage.intoSummary()
	if s.latest != nil {
		sessionID := s.latest.sessionID
		projectPath := s.latest.projectPath
		lastActivity := FormatRFC3339Millis(s.latest.timestamp)
		summary.SessionID = &sessionID
		summary.ProjectPath = &projectPath
		summary.LastActivity = &lastActivity
	}
	if s.earliest != nil {
		first := FormatRFC3339Millis(*s.earliest)
		summary.FirstActivity = &first
	}
	if s.versions != nil {
		versions := make([]string, 0, len(s.versions))
		for v := range s.versions {
			versions = append(versions, v)
		}
		sort.Strings(versions)
		summary.Versions = versions
	}
	return summary
}

// BucketKind selects the weekly/monthly rollup.
type BucketKind int

// Bucket kinds.
const (
	BucketMonthly BucketKind = iota
	BucketWeekly
)

// SummarizeSummariesByBucket rolls daily rows up into weekly or monthly rows.
func SummarizeSummariesByBucket(rows []UsageSummary, kind BucketKind, start WeekDay) []UsageSummary {
	groups := map[string][]UsageSummary{}
	var buckets []string
	for _, row := range rows {
		if row.Date == nil {
			continue
		}
		var bucket string
		switch kind {
		case BucketMonthly:
			if len(*row.Date) >= 7 {
				bucket = (*row.Date)[:7]
			} else {
				bucket = *row.Date
			}
		case BucketWeekly:
			bucket = WeekStart(*row.Date, start)
			if bucket == "" {
				bucket = *row.Date
			}
		}
		if _, ok := groups[bucket]; !ok {
			buckets = append(buckets, bucket)
		}
		groups[bucket] = append(groups[bucket], row)
	}
	sort.Strings(buckets)
	out := make([]UsageSummary, 0, len(buckets))
	for _, bucket := range buckets {
		summary := aggregateSummaries(groups[bucket])
		b := bucket
		switch kind {
		case BucketMonthly:
			summary.Month = &b
		case BucketWeekly:
			summary.Week = &b
		}
		out = append(out, summary)
	}
	return out
}

func aggregateSummaries(rows []UsageSummary) UsageSummary {
	summary := UsageSummary{ModelsUsed: []string{}}
	seenModels := map[string]struct{}{}
	breakdownIndex := map[string]int{}
	for _, row := range rows {
		summary.InputTokens += row.InputTokens
		summary.OutputTokens += row.OutputTokens
		summary.CacheCreationTokens += row.CacheCreationTokens
		summary.CacheReadTokens += row.CacheReadTokens
		summary.ExtraTotalTokens += row.ExtraTotalTokens
		summary.TotalCost += row.TotalCost
		if row.Credits != nil {
			if summary.Credits == nil {
				zero := 0.0
				summary.Credits = &zero
			}
			*summary.Credits += *row.Credits
		}
		if row.MessageCount != nil {
			if summary.MessageCount == nil {
				zero := uint64(0)
				summary.MessageCount = &zero
			}
			*summary.MessageCount += *row.MessageCount
		}
		for _, model := range row.ModelsUsed {
			if _, ok := seenModels[model]; !ok {
				seenModels[model] = struct{}{}
				summary.ModelsUsed = append(summary.ModelsUsed, model)
			}
		}
		for _, item := range row.ModelBreakdowns {
			index, ok := breakdownIndex[item.ModelName]
			if !ok {
				index = len(summary.ModelBreakdowns)
				breakdownIndex[item.ModelName] = index
				summary.ModelBreakdowns = append(summary.ModelBreakdowns, ModelBreakdown{ModelName: item.ModelName})
			}
			b := &summary.ModelBreakdowns[index]
			b.InputTokens += item.InputTokens
			b.OutputTokens += item.OutputTokens
			b.CacheCreationTokens += item.CacheCreationTokens
			b.CacheReadTokens += item.CacheReadTokens
			b.ExtraTotalTokens += item.ExtraTotalTokens
			b.Cost += item.Cost
			b.MissingPricing = b.MissingPricing || item.MissingPricing
		}
	}
	sort.SliceStable(summary.ModelBreakdowns, func(i, j int) bool {
		return summary.ModelBreakdowns[i].Cost > summary.ModelBreakdowns[j].Cost
	})
	return summary
}

// SortSummaries orders rows by dateFn per the requested order.
func SortSummaries(rows []UsageSummary, order SortOrder, dateFn func(*UsageSummary) string) []UsageSummary {
	sort.SliceStable(rows, func(i, j int) bool {
		if order == OrderDesc {
			return dateFn(&rows[j]) < dateFn(&rows[i])
		}
		return dateFn(&rows[i]) < dateFn(&rows[j])
	})
	return rows
}

// FilterAndSortSummaries applies the since/until window then sorts by dateFn.
func FilterAndSortSummaries(rows []UsageSummary, shared *SharedArgs, dateFn func(*UsageSummary) string) []UsageSummary {
	if shared.Since != nil || shared.Until != nil {
		filtered := make([]UsageSummary, 0, len(rows))
		for i := range rows {
			if DateWithinRange(dateFn(&rows[i]), shared.Since, shared.Until) {
				filtered = append(filtered, rows[i])
			}
		}
		rows = filtered
	}
	sort.SliceStable(rows, func(i, j int) bool {
		switch shared.Order {
		case OrderDesc:
			return dateFn(&rows[j]) < dateFn(&rows[i])
		default:
			return dateFn(&rows[i]) < dateFn(&rows[j])
		}
	})
	return rows
}

// parseISODate parses a strict YYYY-MM-DD; ok=false on malformed input.
func parseISODate(date string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	return t, err == nil
}

// WeekStart computes the YYYY-MM-DD of the week containing date, per start day.
func WeekStart(date string, start WeekDay) string {
	t, ok := parseISODate(date)
	if !ok {
		return ""
	}
	// weekday: Sunday = 0 ... Saturday = 6
	weekday := int(t.Weekday())
	shift := (weekday - int(start) + 7) % 7
	monday := t.AddDate(0, 0, -shift)
	return monday.Format("2006-01-02")
}
