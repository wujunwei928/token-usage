package core

import "testing"

// Worked expectations: each kind's CLI name, table column, JSON rows key, and
// per-row period field, as fixed by the reference CLI vocabulary.
func TestReportKindVocabulary(t *testing.T) {
	cases := []struct {
		kind        ReportKind
		name        string
		firstColumn string
		rowsKey     string
		periodKey   string
	}{
		{KindDaily, "daily", "Date", "daily", "date"},
		{KindWeekly, "weekly", "Week", "weekly", "week"},
		{KindMonthly, "monthly", "Month", "monthly", "month"},
		{KindSession, "session", "Session", "sessions", "sessionId"},
	}
	for _, c := range cases {
		if got := c.kind.String(); got != c.name {
			t.Errorf("%d String() = %q, want %q", c.kind, got, c.name)
		}
		if got := c.kind.FirstColumn(); got != c.firstColumn {
			t.Errorf("%s FirstColumn() = %q, want %q", c.name, got, c.firstColumn)
		}
		if got := c.kind.RowsKey(); got != c.rowsKey {
			t.Errorf("%s RowsKey() = %q, want %q", c.name, got, c.rowsKey)
		}
		if got := c.kind.PeriodKey(); got != c.periodKey {
			t.Errorf("%s PeriodKey() = %q, want %q", c.name, got, c.periodKey)
		}
	}
}
