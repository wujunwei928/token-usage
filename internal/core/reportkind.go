package core

// ReportKind selects the report granularity — the one shared vocabulary for
// every Agent Adapter and every consumer (the Report of CONTEXT.md). Replaces
// the per-adapter local enums (ADR 0009).
type ReportKind int

// Report kinds.
const (
	KindDaily ReportKind = iota
	KindWeekly
	KindMonthly
	KindSession
)

// String returns the CLI name of the kind.
func (k ReportKind) String() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "session"
	}
}

// FirstColumn returns the table's first column header for the kind.
func (k ReportKind) FirstColumn() string {
	switch k {
	case KindDaily:
		return "Date"
	case KindWeekly:
		return "Week"
	case KindMonthly:
		return "Month"
	default:
		return "Session"
	}
}

// RowsKey returns the JSON key holding the rows array for the kind.
func (k ReportKind) RowsKey() string {
	switch k {
	case KindDaily:
		return "daily"
	case KindWeekly:
		return "weekly"
	case KindMonthly:
		return "monthly"
	default:
		return "sessions"
	}
}

// PeriodKey returns the per-row period field name for the kind.
func (k ReportKind) PeriodKey() string {
	switch k {
	case KindDaily:
		return "date"
	case KindWeekly:
		return "week"
	case KindMonthly:
		return "month"
	default:
		return "sessionId"
	}
}
