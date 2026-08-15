// Package report builds and ships the client half of the token leaderboard:
// the Report Snapshot (today's Hourly Usage across all agents) and the
// persistent Device ID.
package report

import (
	"sort"
	"sync"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/amp"
	"github.com/wujunwei928/token-usage/internal/adapter/claude"
	"github.com/wujunwei928/token-usage/internal/adapter/codebuff"
	"github.com/wujunwei928/token-usage/internal/adapter/codex"
	"github.com/wujunwei928/token-usage/internal/adapter/copilot"
	"github.com/wujunwei928/token-usage/internal/adapter/droid"
	"github.com/wujunwei928/token-usage/internal/adapter/gemini"
	"github.com/wujunwei928/token-usage/internal/adapter/goose"
	"github.com/wujunwei928/token-usage/internal/adapter/grok"
	"github.com/wujunwei928/token-usage/internal/adapter/hermes"
	"github.com/wujunwei928/token-usage/internal/adapter/kilo"
	"github.com/wujunwei928/token-usage/internal/adapter/kimi"
	"github.com/wujunwei928/token-usage/internal/adapter/openclaw"
	"github.com/wujunwei928/token-usage/internal/adapter/opencode"
	"github.com/wujunwei928/token-usage/internal/adapter/pi"
	"github.com/wujunwei928/token-usage/internal/adapter/qwen"
	"github.com/wujunwei928/token-usage/internal/adapter/zcode"
	"github.com/wujunwei928/token-usage/internal/core"
)

// HourCell is one (hour, tool, model) aggregate in the snapshot.
type HourCell struct {
	Hour      int    `json:"hour"`
	Tool      string `json:"tool"`
	Model     string `json:"model"`
	Input     uint64 `json:"input"`
	Output    uint64 `json:"output"`
	CacheRead uint64 `json:"cacheRead"`
	Cache5m   uint64 `json:"cacheWrite5m"`
	Cache1h   uint64 `json:"cacheWrite1h"`
}

// Snapshot is the Report Snapshot: one device's day of Hourly Usage.
type Snapshot struct {
	DeviceID    string     `json:"deviceId"`
	DeviceLabel string     `json:"deviceLabel"`
	Date        string     `json:"date"`
	Timezone    string     `json:"timezone"`
	GeneratedAt string     `json:"generatedAt"`
	Hours       []HourCell `json:"hours"`
}

// TotalTokens sums every cell.
func (s *Snapshot) TotalTokens() uint64 {
	var total uint64
	for i := range s.Hours {
		h := &s.Hours[i]
		total += h.Input + h.Output + h.CacheRead + h.Cache5m + h.Cache1h
	}
	return total
}

// cellKey groups cells by (date, hour, tool, model).
type cellKey struct {
	date  string
	hour  int
	tool  string
	model string
}

// builder accumulates cells with deterministic output order.
type builder struct {
	cells map[cellKey]*HourCell
	order []cellKey
}

func (b *builder) add(date string, hour int, tool, model string, input, output, cacheRead, cache5m, cache1h uint64) {
	k := cellKey{date, hour, tool, model}
	cell, ok := b.cells[k]
	if !ok {
		cell = &HourCell{Hour: hour, Tool: tool, Model: model}
		b.cells[k] = cell
		b.order = append(b.order, k)
	}
	cell.Input += input
	cell.Output += output
	cell.CacheRead += cacheRead
	cell.Cache5m += cache5m
	cell.Cache1h += cache1h
}

// loadedAdapter describes one agent's entry source for the snapshot: entries
// already deduped by the adapter, each carrying timestamp/model/usage.
type loadedAdapter struct {
	tool    string
	entries []core.LoadedEntry
}

// loadAllAgents gathers entries in [since, today] from every agent adapter
// in parallel. The SharedArgs mirror the all-report defaults: display cost
// mode (the client never prices tokens — the server owns Cost Estimates),
// offline pricing, and the requested timezone.
//
// Claude deliberately uses the daily pipeline (LoadDailyDetailEntries), the
// same code path ccusage daily / all-report take — the generic loader
// intentionally differs (no agent-progress lines, different dedup tiebreaks),
// and snapshot totals must reconcile with daily-report totals.
func loadAllAgents(shared *core.SharedArgs, since, today string) []loadedAdapter {
	pricing := core.LoadEmbedded()
	var mu sync.Mutex
	var wg sync.WaitGroup
	var out []loadedAdapter

	load := func(tool string, fn func() ([]core.LoadedEntry, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entries, err := fn()
			if err != nil {
				return
			}
			var kept []core.LoadedEntry
			for i := range entries {
				date := entries[i].Date
				if date >= since && date <= today {
					kept = append(kept, entries[i])
				}
			}
			if len(kept) == 0 {
				return
			}
			mu.Lock()
			out = append(out, loadedAdapter{tool: tool, entries: kept})
			mu.Unlock()
		}()
	}

	load("claude", func() ([]core.LoadedEntry, error) { return claudeDailyEntries(shared) })
	load("codex", func() ([]core.LoadedEntry, error) {
		return codexEntries(shared)
	})
	load("opencode", func() ([]core.LoadedEntry, error) { return opencode.LoadEntries(shared) })
	load("amp", func() ([]core.LoadedEntry, error) { return amp.LoadEntries(shared, pricing) })
	load("droid", func() ([]core.LoadedEntry, error) { return droid.LoadEntries(shared) })
	load("codebuff", func() ([]core.LoadedEntry, error) { return codebuff.LoadEntries(shared) })
	load("hermes", func() ([]core.LoadedEntry, error) { return hermes.LoadEntries(shared) })
	load("pi", func() ([]core.LoadedEntry, error) {
		return pi.LoadEntries(pi.LoadOptions{Shared: shared, Pricing: pricing})
	})
	load("goose", func() ([]core.LoadedEntry, error) { return goose.LoadEntries(shared, pricing) })
	load("openclaw", func() ([]core.LoadedEntry, error) { return openclaw.LoadEntries(shared, nil, pricing) })
	load("kilo", func() ([]core.LoadedEntry, error) { return kilo.LoadEntries(shared, pricing) })
	load("copilot", func() ([]core.LoadedEntry, error) { return copilot.LoadEntries(shared, pricing) })
	load("gemini", func() ([]core.LoadedEntry, error) { return gemini.LoadEntries(shared, pricing) })
	load("kimi", func() ([]core.LoadedEntry, error) { return kimi.LoadEntries(shared, pricing) })
	load("qwen", func() ([]core.LoadedEntry, error) { return qwen.LoadEntries(shared) })
	load("grok", func() ([]core.LoadedEntry, error) { return grok.LoadEntries(shared) })
	load("zcode", func() ([]core.LoadedEntry, error) { return zcode.LoadEntries(shared) })

	wg.Wait()
	return out
}

// claudeDailyEntries converts the daily pipeline's deduped entries into the
// snapshot's LoadedEntry shape (the daily code path, for parity with
// `ccusage daily` totals — see loadAllAgents).
func claudeDailyEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	detail, err := claude.LoadDailyDetailEntries(shared)
	if err != nil {
		return nil, err
	}
	out := make([]core.LoadedEntry, 0, len(detail))
	for i := range detail {
		d := &detail[i]
		model := d.Model
		out = append(out, core.LoadedEntry{
			Timestamp: d.Timestamp,
			Date:      d.Date,
			Model:     model,
			Data: core.UsageEntry{
				Message: core.UsageMessage{
					Model: model,
					Usage:  d.Usage,
				},
			},
		})
	}
	return out, nil
}

// codexEntries converts Codex token events into LoadedEntry rows (cached
// input maps to cache-read; Codex has no cache-write counters).
func codexEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	events, err := codex.LoadCodexEvents(shared)
	if err != nil {
		return nil, err
	}
	tz := core.ParseTZ(shared.Timezone)
	out := make([]core.LoadedEntry, 0, len(events))
	for i := range events {
		e := &events[i]
		ts, ok := core.ParseTSTimestamp(e.Timestamp)
		if !ok {
			continue
		}
		model := "unknown"
		if e.Model != nil && *e.Model != "" {
			model = *e.Model
		}
		usage := core.TokenUsageRaw{
			InputTokens:          e.InputTokens,
			OutputTokens:         e.OutputTokens,
			CacheReadInputTokens: e.CachedInputTokens,
		}
		out = append(out, core.LoadedEntry{
			Timestamp: ts,
			Date:      core.FormatDateTZ(ts, tz),
			Model:     &model,
			Data: core.UsageEntry{
				Message: core.UsageMessage{
					Model: &model,
					Usage: usage,
				},
			},
		})
	}
	return out, nil
}

// BuildSnapshot aggregates today's usage (client-local timezone) across all
// 16 agent adapters into a Report Snapshot.
func BuildSnapshot(shared *core.SharedArgs, deviceID, deviceLabel string, now time.Time) *Snapshot {
	tz := timezoneOf(shared)
	today := core.FormatDateTZ(now.UnixMilli(), tz)
	return &Snapshot{
		DeviceID:    deviceID,
		DeviceLabel: deviceLabel,
		Date:        today,
		Timezone:    tz.String(),
		GeneratedAt: now.Format(time.RFC3339),
		Hours:       buildCells(shared, today, today)[today],
	}
}

// BuildBackfill aggregates every date in [since, today] with data into one
// snapshot per date. Empty dates are skipped: an absent day leaves whatever
// the server already stores untouched (each (device, date) is an independent
// Latest-wins unit).
func BuildBackfill(shared *core.SharedArgs, deviceID, deviceLabel, since string, now time.Time) []*Snapshot {
	tz := timezoneOf(shared)
	today := core.FormatDateTZ(now.UnixMilli(), tz)
	byDate := buildCells(shared, since, today)
	var dates []string
	for date := range byDate {
		if date >= since && date <= today {
			dates = append(dates, date)
		}
	}
	sort.Strings(dates)
	out := make([]*Snapshot, 0, len(dates))
	for _, date := range dates {
		out = append(out, &Snapshot{
			DeviceID:    deviceID,
			DeviceLabel: deviceLabel,
			Date:        date,
			Timezone:    tz.String(),
			GeneratedAt: now.Format(time.RFC3339),
			Hours:       byDate[date],
		})
	}
	return out
}

func timezoneOf(shared *core.SharedArgs) *time.Location {
	if tz := core.ParseTZ(shared.Timezone); tz != nil {
		return tz
	}
	return time.Local
}

// buildCells loads every agent's entries in [since, today] once and groups
// them into per-date Hourly Usage cells.
func buildCells(shared *core.SharedArgs, since, today string) map[string][]HourCell {
	b := &builder{cells: map[cellKey]*HourCell{}, order: []cellKey{}}
	for _, adapter := range loadAllAgents(shared, since, today) {
		for i := range adapter.entries {
			entry := &adapter.entries[i]
			if entry.Timestamp == 0 || entry.Date < since || entry.Date > today {
				continue
			}
			model := "unknown"
			if entry.Model != nil && *entry.Model != "" {
				model = core.ResolveModelName(*entry.Model)
			}
			usage := entry.Data.Message.Usage
			cache5m := usage.CacheCreationInputTokens
			cache1h := uint64(0)
			if usage.CacheCreation != nil {
				cache5m = usage.CacheCreation.Ephemeral5mInputTokens
				cache1h = usage.CacheCreation.Ephemeral1hInputTokens
			}
			hour := time.UnixMilli(entry.Timestamp).In(tzOf(shared)).Hour()
			b.add(entry.Date, hour, adapter.tool, model,
				usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens,
				cache5m, cache1h)
		}
	}
	byDate := map[string][]HourCell{}
	for _, k := range b.order {
		byDate[k.date] = append(byDate[k.date], *b.cells[k])
	}
	return byDate
}

func tzOf(shared *core.SharedArgs) *time.Location { return timezoneOf(shared) }
