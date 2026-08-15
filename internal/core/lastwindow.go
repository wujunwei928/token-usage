package core

import (
	"fmt"
	"time"
)

// PeriodUnit is the calendar unit a report groups rows by.
type PeriodUnit int

// Period units.
const (
	PeriodDay PeriodUnit = iota
	PeriodWeek
	PeriodMonth
)

// LastPeriodsSince returns the compact YYYYMMDD start of the window covering
// the most recent count periods, the last of which is the one today falls in.
// today is a YYYY-MM-DD already resolved in the report timezone; weeks start
// on startOfWeek. ok=false when the date cannot be parsed or shifted.
func LastPeriodsSince(unit PeriodUnit, count uint32, today string, startOfWeek WeekDay) (string, bool) {
	if count < 1 {
		count = 1
	}
	earlier := int64(count - 1)
	t, ok := parseISODate(today)
	if !ok {
		return "", false
	}
	var start time.Time
	switch unit {
	case PeriodDay:
		start = t.AddDate(0, 0, -int(earlier))
	case PeriodWeek:
		week, ok := parseISODate(WeekStart(today, startOfWeek))
		if !ok {
			return "", false
		}
		start = week.AddDate(0, 0, -7*int(earlier))
	case PeriodMonth:
		firstOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		start = firstOfMonth.AddDate(0, -int(earlier), 0)
	}
	return fmt.Sprintf("%04d%02d%02d", start.Year(), int(start.Month()), start.Day()), true
}
