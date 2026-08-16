package common

import (
	"reflect"
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

// base is 2026-08-14T10:00:00.000Z (a Friday). Dates 08-14..08-17 span the
// Sunday-start and Monday-start week boundaries, exercising the profile knob.
const base = int64(1786701600000)

func mkEntry(offsetHours int64, date, session string, in, out uint64) core.LoadedEntry {
	model := "test-model"
	sid := session
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sid,
			Message: core.UsageMessage{
				Usage: core.TokenUsageRaw{InputTokens: in, OutputTokens: out},
				Model: &model,
			},
		},
		Timestamp:   base + offsetHours*3600000,
		Date:        date,
		Project:     "test",
		SessionID:   session,
		ProjectPath: "Test",
		Model:       &model,
	}
}

func rowPeriod(row core.UsageSummary) string {
	switch {
	case row.Date != nil:
		return *row.Date
	case row.Week != nil:
		return *row.Week
	case row.Month != nil:
		return *row.Month
	case row.SessionID != nil:
		return *row.SessionID
	}
	return ""
}

func TestSummarizeReportDaily(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 100, 10),
		mkEntry(24, "2026-08-14", "A", 50, 5),
		mkEntry(48, "2026-08-15", "B", 200, 20),
	}
	rows := SummarizeReport(entries, core.KindDaily, ReportProfile{})
	if len(rows) != 2 {
		t.Fatalf("daily rows = %d, want 2", len(rows))
	}
	if rows[0].Date == nil || *rows[0].Date != "2026-08-14" || rows[0].InputTokens != 150 || rows[0].OutputTokens != 15 {
		t.Errorf("row0 = %+v, want date 2026-08-14 in=150 out=15", rows[0])
	}
	if rows[1].Date == nil || *rows[1].Date != "2026-08-15" || rows[1].InputTokens != 200 {
		t.Errorf("row1 = %+v, want date 2026-08-15 in=200", rows[1])
	}
	if !reflect.DeepEqual(rows[0].ModelsUsed, []string{"test-model"}) {
		t.Errorf("modelsUsed = %v", rows[0].ModelsUsed)
	}
}

func TestSummarizeReportWeeklySundayDefault(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 1, 0),  // Friday
		mkEntry(24, "2026-08-15", "A", 1, 0), // Saturday
		mkEntry(48, "2026-08-16", "B", 1, 0), // Sunday
		mkEntry(72, "2026-08-17", "B", 1, 0), // Monday
	}
	rows := SummarizeReport(entries, core.KindWeekly, ReportProfile{})
	want := []string{"2026-08-09", "2026-08-16"}
	if len(rows) != 2 {
		t.Fatalf("weekly rows = %d, want 2 (buckets %v)", len(rows), want)
	}
	for i, w := range want {
		if rows[i].Week == nil || *rows[i].Week != w || rows[i].InputTokens != 2 {
			t.Errorf("row%d = week %v in=%d, want %s in=2", i, rows[i].Week, rows[i].InputTokens, w)
		}
	}
}

func TestSummarizeReportWeeklyMonday(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 1, 0),  // Friday
		mkEntry(24, "2026-08-15", "A", 1, 0), // Saturday
		mkEntry(48, "2026-08-16", "B", 1, 0), // Sunday
		mkEntry(72, "2026-08-17", "B", 1, 0), // Monday
	}
	rows := SummarizeReport(entries, core.KindWeekly, ReportProfile{WeekStart: core.Monday})
	want := []struct {
		week string
		in   uint64
	}{{"2026-08-10", 3}, {"2026-08-17", 1}}
	if len(rows) != 2 {
		t.Fatalf("weekly rows = %d, want 2", len(rows))
	}
	for i, w := range want {
		if rows[i].Week == nil || *rows[i].Week != w.week || rows[i].InputTokens != w.in {
			t.Errorf("row%d = week %v in=%d, want %s in=%d", i, rows[i].Week, rows[i].InputTokens, w.week, w.in)
		}
	}
}

func TestSummarizeReportMonthly(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-07-31", "A", 10, 0),
		mkEntry(48, "2026-08-01", "B", 20, 0),
	}
	rows := SummarizeReport(entries, core.KindMonthly, ReportProfile{})
	if len(rows) != 2 || rows[0].Month == nil || *rows[0].Month != "2026-07" || rows[1].Month == nil || *rows[1].Month != "2026-08" {
		t.Fatalf("monthly buckets = %v", rows)
	}
}

func TestSummarizeReportSessionKeySwap(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 100, 0),
		mkEntry(24, "2026-08-15", "A", 50, 0),
		mkEntry(25, "2026-08-15", "B", 10, 0),
	}
	rows := SummarizeReport(entries, core.KindSession, ReportProfile{})
	if len(rows) != 2 {
		t.Fatalf("session rows = %d, want 2", len(rows))
	}
	if rows[0].SessionID == nil || *rows[0].SessionID != "A" || rows[0].InputTokens != 150 || rows[0].Date != nil {
		t.Errorf("row0 = %+v, want session A in=150 with date slot cleared", rows[0])
	}
	if rows[0].LastActivity != nil {
		t.Errorf("key-swap session rows carry no activity bounds, got lastActivity %v", *rows[0].LastActivity)
	}
	if rows[1].SessionID == nil || *rows[1].SessionID != "B" || rows[1].InputTokens != 10 {
		t.Errorf("row1 = %+v, want session B in=10", rows[1])
	}
}

func TestSummarizeReportSessionAccumulator(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 100, 0), // 10:00
		mkEntry(24, "2026-08-15", "A", 50, 0), // next day 10:00
		mkEntry(25, "2026-08-15", "B", 10, 0),
	}
	rows := SummarizeReport(entries, core.KindSession, ReportProfile{SessionByActivity: true})
	if len(rows) != 2 {
		t.Fatalf("session rows = %d, want 2", len(rows))
	}
	a := rows[0]
	if a.SessionID == nil || *a.SessionID != "A" || a.InputTokens != 150 {
		t.Errorf("row0 = %+v, want session A in=150", a)
	}
	if a.LastActivity == nil || *a.LastActivity != "2026-08-15T10:00:00.000Z" {
		t.Errorf("lastActivity = %v, want 2026-08-15T10:00:00.000Z", a.LastActivity)
	}
	if a.FirstActivity == nil || *a.FirstActivity != "2026-08-14T10:00:00.000Z" {
		t.Errorf("firstActivity = %v, want 2026-08-14T10:00:00.000Z", a.FirstActivity)
	}
	if a.ProjectPath == nil || *a.ProjectPath != "Test" {
		t.Errorf("projectPath = %v, want Test", a.ProjectPath)
	}
}

func TestReportRowsFiltersEntriesBeforeSummarizing(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 100, 0),
		mkEntry(24, "2026-08-15", "A", 50, 0),
		mkEntry(25, "2026-08-15", "B", 10, 0),
	}
	since := "20260815"
	shared := &core.SharedArgs{Since: &since}
	rows := ReportRows(entries, core.KindSession, shared, ReportProfile{})
	// Filter-first: session A only keeps its in-window entry (50), B keeps 10.
	if len(rows) != 2 || rows[0].InputTokens != 50 || rows[1].InputTokens != 10 {
		t.Fatalf("filter-first session rows = %+v, want A=50 B=10", rows)
	}
	daily := ReportRows(entries, core.KindDaily, shared, ReportProfile{})
	if len(daily) != 1 || daily[0].InputTokens != 60 {
		t.Fatalf("filter-first daily rows = %+v, want one 2026-08-15 row in=60", daily)
	}
}

func TestReportRowsSessionFiltersAfterSummarizing(t *testing.T) {
	entries := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 100, 0),
		mkEntry(24, "2026-08-15", "A", 50, 0),
		mkEntry(25, "2026-08-15", "B", 10, 0),
	}
	since := "20260815"
	shared := &core.SharedArgs{Since: &since}
	profile := ReportProfile{SessionByActivity: true, SessionFilterAfter: true}
	rows := ReportRows(entries, core.KindSession, shared, profile)
	// Filter-after: session A summarized from ALL entries (150); its last
	// activity (08-15) is inside the window so the whole session survives.
	if len(rows) != 2 || rows[0].InputTokens != 150 || rows[1].InputTokens != 10 {
		t.Fatalf("filter-after session rows = %+v, want A=150 B=10", rows)
	}
	// A session whose last activity falls outside the window disappears whole.
	outside := []core.LoadedEntry{
		mkEntry(0, "2026-08-14", "A", 100, 0),
		mkEntry(1, "2026-08-14", "A", 50, 0),
	}
	rows = ReportRows(outside, core.KindSession, shared, profile)
	if len(rows) != 0 {
		t.Fatalf("session outside window survived: %+v", rows)
	}
}
