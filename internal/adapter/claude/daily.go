// daily.go ports the dedicated daily-summary pipeline of the reference
// (rust/adapters/claude/src/daily.rs). It intentionally differs from the
// generic loader in loader.go: agent-progress lines are parsed, dedup
// tiebreaks on cost before speed, and replacement does not refresh the index
// buckets. Both divergences are observable in report output.
package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadDailySummaries scans Claude data and aggregates per-day (or per day and
// project) usage summaries in one pass, like load_daily_summaries_inner.
func LoadDailySummaries(shared *core.SharedArgs, projectFilter *string, groupByProject bool) ([]core.UsageSummary, error) {
	return core.TrackUsageLoad("Claude", shared, func() ([]core.UsageSummary, error) {
		return loadDailySummaries(shared, projectFilter, groupByProject)
	})
}

func loadDailySummaries(shared *core.SharedArgs, projectFilter *string, groupByProject bool) ([]core.UsageSummary, error) {
	deduped, err := loadDailyDeduped(shared, projectFilter)
	if err != nil {
		return nil, err
	}

	type groupKey struct {
		date    string
		project string
	}
	groups := map[groupKey]*dailyAccumulator{}
	var keys []groupKey
	for i := range deduped {
		key := groupKey{date: deduped[i].date, project: deduped[i].project}
		if !groupByProject {
			key.project = ""
		}
		acc, ok := groups[key]
		if !ok {
			acc = &dailyAccumulator{}
			groups[key] = acc
			keys = append(keys, key)
		}
		acc.addEntry(&deduped[i])
	}
	// BTreeMap iteration order: date first, then project.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].date != keys[j].date {
			return keys[i].date < keys[j].date
		}
		return keys[i].project < keys[j].project
	})
	rows := make([]core.UsageSummary, 0, len(keys))
	for _, key := range keys {
		summary := groups[key].intoSummary()
		date := key.date
		summary.Date = &date
		if groupByProject {
			project := key.project
			summary.Project = &project
		}
		rows = append(rows, summary)
	}
	return rows, nil
}

// loadDailyDeduped runs the daily pipeline's scan and dedup once, returning
// the winning entries in push order.
func loadDailyDeduped(shared *core.SharedArgs, projectFilter *string) ([]dailyLoadedEntry, error) {
	paths, err := ClaudePaths()
	if err != nil {
		return nil, err
	}
	files := UsageFiles(paths, projectFilter)
	if len(files) == 0 {
		return []dailyLoadedEntry{}, nil
	}
	pricing := loadDailyPricing(shared)
	tz := core.ParseTZ(shared.Timezone)
	mode := shared.Mode
	loadedFiles := common.ReadFilesParallel(files, shared.SingleThread, func(file string) dailyLoadedFile {
		return readDailyUsageFile(file, tz, mode, pricing)
	})

	dedup := newDailyDeduper()
	for _, lf := range loadedFiles {
		for _, entry := range lf.entries {
			if projectFilter != nil && entry.project != *projectFilter {
				continue
			}
			dedup.Push(entry)
		}
	}
	return dedup.entries, nil
}

// DailyDetailEntry is one deduped daily-pipeline entry with its timestamp.
type DailyDetailEntry struct {
	Timestamp int64
	Date      string
	Model     *string
	Usage     core.TokenUsageRaw
}

// LoadDailyDetailEntries runs the daily pipeline (the ccusage daily and
// all-report code path) and returns its deduped entries with timestamps, so
// hour-level aggregation reconciles exactly with daily-report totals. The
// generic LoadEntries loader intentionally differs (no agent-progress lines,
// different dedup tiebreaks) and must not be used for parity-sensitive
// aggregation.
func LoadDailyDetailEntries(shared *core.SharedArgs) ([]DailyDetailEntry, error) {
	deduped, err := loadDailyDeduped(shared, nil)
	if err != nil {
		return nil, err
	}
	out := make([]DailyDetailEntry, 0, len(deduped))
	for i := range deduped {
		e := &deduped[i]
		out = append(out, DailyDetailEntry{
			Timestamp: e.timestamp,
			Date:      e.date,
			Model:     e.model,
			Usage:     e.usage,
		})
	}
	return out, nil
}

type dailyLoadedFile struct {
	entries []dailyLoadedEntry
}

// loadDailyPricing mirrors the loader's PricingMap::load_with_overrides call.
func loadDailyPricing(shared *core.SharedArgs) *core.PricingMap {
	if shared.Mode == core.ModeDisplay {
		return nil
	}
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	// Ticket 10: ccusage.json pricingOverrides ride on SharedArgs into the
	// pricing map load.
	return core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
}

// dailyLoadedEntry is the slim per-line record the daily pipeline aggregates.
type dailyLoadedEntry struct {
	timestamp           int64
	date                string
	project             string
	usage               core.TokenUsageRaw
	cost                float64
	model               *string
	missingPricingModel *string
	messageID           *string
	requestID           *string
	isSidechain         *bool
}

// dailyUsageMessage mirrors DailyUsageMessage: usage is a required object.
type dailyUsageMessage struct {
	Usage *rawTokenUsage `json:"usage"`
	Model *string        `json:"model"`
	ID    *string        `json:"id"`
}

func (m *dailyUsageMessage) valid() bool {
	return m != nil && m.Usage != nil && m.Usage.valid()
}

// dailyUsageEntry mirrors DailyUsageEntry (the "direct" line variant).
type dailyUsageEntry struct {
	Timestamp   *string            `json:"timestamp"`
	Message     *dailyUsageMessage `json:"message"`
	Version     *string            `json:"version"`
	SessionID   *string            `json:"sessionId"`
	CostUSD     *float64           `json:"costUSD"`
	RequestID   *string            `json:"requestId"`
	IsSidechain *bool              `json:"isSidechain"`
}

// dailyAgentProgressMessage mirrors the nested agent-progress envelope.
type dailyAgentProgressMessage struct {
	Timestamp   *string            `json:"timestamp"`
	Message     *dailyUsageMessage `json:"message"`
	CostUSD     *float64           `json:"costUSD"`
	RequestID   *string            `json:"requestId"`
	IsSidechain *bool              `json:"isSidechain"`
}

type dailyAgentProgressData struct {
	Message *dailyAgentProgressMessage `json:"message"`
}

type dailyAgentProgressEntry struct {
	Data *dailyAgentProgressData `json:"data"`
}

// decodeDailyLine mirrors the untagged DailyUsageLine enum: try the direct
// entry first, then the agent-progress envelope; both fail rejects the line.
func decodeDailyLine(line []byte) (dailyUsageEntry, bool) {
	var direct dailyUsageEntry
	if err := json.Unmarshal(line, &direct); err == nil && direct.valid() {
		return direct, true
	}
	var progress dailyAgentProgressEntry
	if err := json.Unmarshal(line, &progress); err == nil && progress.valid() {
		inner := progress.Data.Message
		return dailyUsageEntry{
			Timestamp:   inner.Timestamp,
			Message:     inner.Message,
			CostUSD:     inner.CostUSD,
			RequestID:   inner.RequestID,
			IsSidechain: inner.IsSidechain,
		}, true
	}
	return dailyUsageEntry{}, false
}

func (e *dailyUsageEntry) valid() bool {
	return e.Timestamp != nil && e.Message.valid()
}

func (p *dailyAgentProgressEntry) valid() bool {
	return p.Data != nil && p.Data.Message != nil && p.Data.Message.Timestamp != nil && p.Data.Message.Message.valid()
}

// isValidDailyUsageEntry mirrors is_valid_daily_usage_entry.
func isValidDailyUsageEntry(data *dailyUsageEntry) bool {
	if data.Version != nil && !isSemverPrefix(*data.Version) {
		return false
	}
	if data.SessionID != nil && *data.SessionID == "" {
		return false
	}
	if data.RequestID != nil && *data.RequestID == "" {
		return false
	}
	if data.Message.ID != nil && *data.Message.ID == "" {
		return false
	}
	if data.Message.Model != nil && *data.Message.Model == "" {
		return false
	}
	return true
}

func readDailyUsageFile(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) dailyLoadedFile {
	project := ExtractProject(path)
	lf := dailyLoadedFile{}
	content, err := os.ReadFile(path)
	if err != nil {
		return lf
	}

	usageMarker := []byte(`"usage":{`)
	for _, line := range common.SplitBytesLines(content) {
		if !bytes.Contains(line, usageMarker) {
			continue
		}
		if hasUnsupportedNullField(line) {
			continue
		}
		data, ok := decodeDailyLine(line)
		if !ok {
			continue
		}
		timestamp, ok := core.ParseTSTimestamp(*data.Timestamp)
		if !ok {
			continue
		}
		if !isValidDailyUsageEntry(&data) {
			continue
		}
		usage := data.Message.Usage.toCore()
		cost := core.CalculateCostForUsage(data.Message.Model, usage, data.CostUSD, mode, pricing)
		missingPricingModel := core.MissingPricingModelForUsage(data.Message.Model, usage, data.CostUSD, mode, pricing)
		var model *string
		if data.Message.Model != nil {
			if *data.Message.Model == "<synthetic>" {
				model = nil
			} else if usage.Speed != nil && *usage.Speed == core.SpeedFast {
				suffixed := *data.Message.Model + "-fast"
				model = &suffixed
			} else {
				dup := *data.Message.Model
				model = &dup
			}
		}
		date := core.FormatDateTZ(timestamp, tz)
		lf.entries = append(lf.entries, dailyLoadedEntry{
			timestamp:           timestamp,
			date:                date,
			project:             project,
			usage:               usage,
			cost:                cost,
			model:               model,
			missingPricingModel: missingPricingModel,
			messageID:           data.Message.ID,
			requestID:           data.RequestID,
			isSidechain:         data.IsSidechain,
		})
		for advisorIndex, advisor := range advisorUsagesFromLine(line) {
			advisorMissing := core.MissingPricingModelForUsage(&advisor.Model, advisor.Usage, nil, mode, pricing)
			var advisorMessageID *string
			if data.Message.ID != nil {
				suffixed := *data.Message.ID + ":advisor:" + strconv.Itoa(advisorIndex)
				advisorMessageID = &suffixed
			}
			lf.entries = append(lf.entries, dailyLoadedEntry{
				timestamp:           timestamp,
				date:                date,
				project:             project,
				usage:               advisor.Usage,
				cost:                core.CalculateCostForUsage(&advisor.Model, advisor.Usage, nil, mode, pricing),
				model:               &advisor.Model,
				missingPricingModel: advisorMissing,
				messageID:           advisorMessageID,
				requestID:           data.RequestID,
				isSidechain:         data.IsSidechain,
			})
		}
	}
	return lf
}

// dailyDedupKey identifies the exact (message, request) bucket.
type dailyDedupKey struct {
	messageID string
	requestID string
}

type dailyDeduper struct {
	exact   map[dailyDedupKey][]int
	message map[string][]int
	entries []dailyLoadedEntry
}

func newDailyDeduper() *dailyDeduper {
	return &dailyDeduper{
		exact:   map[dailyDedupKey][]int{},
		message: map[string][]int{},
	}
}

// Push inserts one entry; on a duplicate the candidate wins when it is
// non-sidechain, has more tokens, costs more, or carries speed (in that
// order). Unlike the generic loader, replacement does not refresh buckets.
func (d *dailyDeduper) Push(entry dailyLoadedEntry) {
	if entry.messageID == nil {
		d.entries = append(d.entries, entry)
		return
	}
	messageID := *entry.messageID
	requestID := ""
	if entry.requestID != nil {
		requestID = *entry.requestID
	}
	exactKey := dailyDedupKey{messageID: messageID, requestID: requestID}

	if index, ok := d.findExact(exactKey, entry.requestID); ok {
		if shouldReplaceDedupedDailyEntry(&entry, &d.entries[index]) {
			d.entries[index] = entry
		}
		return
	}
	// /btw sidechain logs can replay parent messages with new request IDs.
	candidateIsSidechain := isSidechainDailyEntry(&entry)
	if index, ok := d.findMessageSidechain(messageID, entry.requestID, candidateIsSidechain); ok {
		if shouldReplaceDedupedDailyEntry(&entry, &d.entries[index]) {
			d.entries[index] = entry
		}
		return
	}

	index := len(d.entries)
	d.entries = append(d.entries, entry)
	d.pushIndex(exactKey, index)
	d.pushMessageIndex(messageID, index)
}

func (d *dailyDeduper) findExact(key dailyDedupKey, requestID *string) (int, bool) {
	for _, index := range d.exact[key] {
		existing := &d.entries[index]
		if existing.messageID == nil || *existing.messageID != key.messageID {
			continue
		}
		if requestID == nil {
			if existing.requestID == nil {
				return index, true
			}
		} else if existing.requestID != nil && *existing.requestID == *requestID {
			return index, true
		}
	}
	return 0, false
}

func (d *dailyDeduper) findMessageSidechain(messageID string, requestID *string, candidateIsSidechain bool) (int, bool) {
	for _, index := range d.message[messageID] {
		existing := &d.entries[index]
		if existing.messageID == nil || *existing.messageID != messageID {
			continue
		}
		if candidateIsSidechain || isSidechainDailyEntry(existing) {
			return index, true
		}
	}
	return 0, false
}

func (d *dailyDeduper) pushIndex(key dailyDedupKey, index int) {
	for _, existing := range d.exact[key] {
		if existing == index {
			return
		}
	}
	d.exact[key] = append(d.exact[key], index)
}

func (d *dailyDeduper) pushMessageIndex(messageID string, index int) {
	for _, existing := range d.message[messageID] {
		if existing == index {
			return
		}
	}
	d.message[messageID] = append(d.message[messageID], index)
}

func dailyUsageTokenTotal(entry *dailyLoadedEntry) uint64 {
	return entry.usage.InputTokens + entry.usage.OutputTokens +
		entry.usage.CacheCreationTokenCount() + entry.usage.CacheReadInputTokens
}

func shouldReplaceDedupedDailyEntry(candidate, existing *dailyLoadedEntry) bool {
	candidateSidechain := isSidechainDailyEntry(candidate)
	existingSidechain := isSidechainDailyEntry(existing)
	if candidateSidechain != existingSidechain {
		return existingSidechain
	}
	candidateTotal := dailyUsageTokenTotal(candidate)
	existingTotal := dailyUsageTokenTotal(existing)
	if candidateTotal != existingTotal {
		return candidateTotal > existingTotal
	}
	if candidate.cost != existing.cost {
		return candidate.cost > existing.cost
	}
	return candidate.usage.Speed != nil && existing.usage.Speed == nil
}

func isSidechainDailyEntry(entry *dailyLoadedEntry) bool {
	return entry.isSidechain != nil && *entry.isSidechain
}

// dailyAccumulator mirrors DailyAccumulator: models in first-seen order,
// breakdowns sorted by descending cost when the summary is produced.
type dailyAccumulator struct {
	counts         core.TokenCounts
	cost           float64
	models         []string
	breakdowns     []core.ModelBreakdown
	breakdownIndex map[string]int
}

func (a *dailyAccumulator) addEntry(entry *dailyLoadedEntry) {
	a.counts.AddUsage(entry.usage)
	a.cost += entry.cost
	if entry.model != nil {
		model := core.ResolveModelName(*entry.model)
		if a.breakdownIndex == nil {
			a.breakdownIndex = map[string]int{}
		}
		index, ok := a.breakdownIndex[model]
		if !ok {
			index = len(a.breakdowns)
			a.breakdownIndex[model] = index
			a.models = append(a.models, model)
			a.breakdowns = append(a.breakdowns, core.ModelBreakdown{ModelName: model})
		}
		b := &a.breakdowns[index]
		b.InputTokens += entry.usage.InputTokens
		b.OutputTokens += entry.usage.OutputTokens
		b.CacheCreationTokens += entry.usage.CacheCreationTokenCount()
		b.CacheReadTokens += entry.usage.CacheReadInputTokens
		b.Cost += entry.cost
		if entry.missingPricingModel != nil {
			b.MissingPricing = true
		}
	}
}

func (a *dailyAccumulator) intoSummary() core.UsageSummary {
	breakdowns := append([]core.ModelBreakdown(nil), a.breakdowns...)
	sort.SliceStable(breakdowns, func(i, j int) bool {
		return breakdowns[i].Cost > breakdowns[j].Cost
	})
	return core.UsageSummary{
		InputTokens:         a.counts.InputTokens,
		OutputTokens:        a.counts.OutputTokens,
		CacheCreationTokens: a.counts.CacheCreationTokens,
		CacheReadTokens:     a.counts.CacheReadTokens,
		TotalCost:           a.cost,
		ModelsUsed:          a.models,
		ModelBreakdowns:     breakdowns,
	}
}
