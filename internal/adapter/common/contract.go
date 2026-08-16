package common

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

// AdapterContract wires one adapter's fixture environment into the shared
// contract suite: the interface is the test surface (ADR 0009).
type AdapterContract struct {
	Agent   string
	Profile ReportProfile

	// Build prepares the adapter's environment (temp dirs, env overrides) and
	// returns the ready adapter plus its load request.
	Build func(t *testing.T) (Adapter, LoadRequest)
}

// RunContract verifies the shared adapter contract: deterministic loads, the
// Detected rule, entry shape, unique periods, and token mass balance across
// every report kind under the adapter's profile.
func RunContract(t *testing.T, c AdapterContract) {
	t.Helper()
	adapter, request := c.Build(t)

	if adapter.Agent() != c.Agent {
		t.Errorf("Agent() = %q, want %q", adapter.Agent(), c.Agent)
	}

	first, err := adapter.LoadEntries(request)
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}
	second, err := adapter.LoadEntries(request)
	if err != nil {
		t.Fatalf("second LoadEntries: %v", err)
	}
	if !sameEntryShape(first.Entries, second.Entries) {
		t.Error("two loads disagree: entries are not deterministic")
	}

	for i := range first.Entries {
		if first.Entries[i].Date == "" {
			t.Errorf("entry %d has an empty Date", i)
		}
		if first.Entries[i].SessionID == "" {
			t.Errorf("entry %d has an empty SessionID", i)
		}
	}
	assertTimestampsOrdered(t, first.Entries)
	assertDedupHashUnique(t, first.Entries)

	if want := len(first.Entries) > 0 || adapter.HasData(); first.Detected != want {
		t.Errorf("Detected = %v, want %v (entries %d, HasData %v)",
			first.Detected, want, len(first.Entries), adapter.HasData())
	}

	if len(first.Entries) == 0 {
		t.Fatalf("fixture produced no entries; the contract needs data")
	}

	entryMass := uint64(0)
	weekBuckets := map[string]bool{}
	for i := range first.Entries {
		entry := &first.Entries[i]
		entryMass += core.TotalUsageTokens(entry.Data.Message.Usage) + entry.ExtraTotalTokens
		if week := core.WeekStart(entry.Date, c.Profile.WeekStart); week != "" {
			weekBuckets[week] = true
		} else {
			weekBuckets[entry.Date] = true
		}
	}
	for _, kind := range []core.ReportKind{core.KindDaily, core.KindWeekly, core.KindMonthly, core.KindSession} {
		rows := SummarizeReport(first.Entries, kind, c.Profile)
		if kind == core.KindWeekly {
			for i := range rows {
				if rows[i].Week != nil && !weekBuckets[*rows[i].Week] {
					t.Errorf("weekly bucket %q is not the profile week start of any entry date", *rows[i].Week)
				}
			}
		}
		rowMass := uint64(0)
		seen := map[string]bool{}
		for i := range rows {
			rowMass += rows[i].TotalTokens()
			period := SummaryPeriod(&rows[i])
			if period == "" {
				t.Errorf("%s row %d carries no period", kind, i)
				continue
			}
			if seen[period] {
				t.Errorf("%s has duplicate period %q", kind, period)
			}
			seen[period] = true
		}
		if rowMass != entryMass {
			t.Errorf("%s token mass %d != entry mass %d", kind, rowMass, entryMass)
		}
	}
}

func sameEntryShape(a, b []core.LoadedEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		massA := core.TotalUsageTokens(a[i].Data.Message.Usage) + a[i].ExtraTotalTokens
		massB := core.TotalUsageTokens(b[i].Data.Message.Usage) + b[i].ExtraTotalTokens
		if a[i].Timestamp != b[i].Timestamp ||
			a[i].Date != b[i].Date ||
			a[i].SessionID != b[i].SessionID ||
			massA != massB {
			return false
		}
	}
	return true
}

// assertTimestampsOrdered requires the entry stream to be monotonic by
// timestamp in one direction — adapters pick ascending or newest-first, but
// never interleave.
func assertTimestampsOrdered(t *testing.T, entries []core.LoadedEntry) {
	t.Helper()
	ascending, descending := true, true
	for i := 1; i < len(entries); i++ {
		switch {
		case entries[i].Timestamp > entries[i-1].Timestamp:
			descending = false
		case entries[i].Timestamp < entries[i-1].Timestamp:
			ascending = false
		}
	}
	if !ascending && !descending {
		t.Error("entries are not monotonic by timestamp (neither ascending nor newest-first)")
	}
}

// assertDedupHashUnique requires the (message.id, requestId) pair to be
// unique among entries that carry an identity at all — the Dedup Hash rule
// of CONTEXT.md. Entries without any identifier are exempt.
func assertDedupHashUnique(t *testing.T, entries []core.LoadedEntry) {
	t.Helper()
	seen := map[[2]string]bool{}
	for i := range entries {
		id, request := entries[i].Data.Message.ID, entries[i].Data.RequestID
		if id == nil && request == nil {
			continue
		}
		key := [2]string{deref(id), deref(request)}
		if seen[key] {
			t.Errorf("duplicate dedup hash (%s, %s) at entry %d", key[0], key[1], i)
		}
		seen[key] = true
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
