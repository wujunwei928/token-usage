package core

import (
	"fmt"
	"strings"
	"time"
)

const (
	millisPerSecond = 1000
	millisPerMinute = 60 * millisPerSecond
	millisPerHour   = 60 * millisPerMinute
	millisPerDay    = 24 * millisPerHour
)

// ParseTSTimestamp parses the strict RFC3339-ish timestamp shapes the reference
// accepts: len 20/25 with Z or ±, or len 24/29 with .mmm — anything else fails.
func ParseTSTimestamp(value string) (int64, bool) {
	b := []byte(value)
	var millis int64
	var timezoneStart int
	switch {
	case (len(b) == 20 || len(b) == 25) && (b[19] == 'Z' || b[19] == '+' || b[19] == '-'):
		millis, timezoneStart = 0, 19
	case (len(b) == 24 || len(b) == 29) && b[19] == '.':
		m, ok := parseDigits(b[20:23])
		if !ok {
			return 0, false
		}
		millis, timezoneStart = int64(m), 23
	default:
		return 0, false
	}
	if b[4] != '-' || b[7] != '-' || b[10] != 'T' || b[13] != ':' || b[16] != ':' {
		return 0, false
	}
	year, ok := parseDigits(b[0:4])
	if !ok {
		return 0, false
	}
	month, ok := parseDigits(b[5:7])
	if !ok {
		return 0, false
	}
	day, ok := parseDigits(b[8:10])
	if !ok {
		return 0, false
	}
	hour, ok := parseDigits(b[11:13])
	if !ok {
		return 0, false
	}
	minute, ok := parseDigits(b[14:16])
	if !ok {
		return 0, false
	}
	second, ok := parseDigits(b[17:19])
	if !ok {
		return 0, false
	}
	if hour > 23 || minute > 59 || second > 59 {
		return 0, false
	}
	offsetMinutes, ok := parseTimezoneOffset(b[timezoneStart:])
	if !ok {
		return 0, false
	}
	days, ok := daysFromCivil(int64(year), int64(month), int64(day))
	if !ok {
		return 0, false
	}
	ts := days * millisPerDay
	ts += int64(hour) * millisPerHour
	ts += int64(minute) * millisPerMinute
	ts += int64(second) * millisPerSecond
	ts += millis
	ts -= int64(offsetMinutes) * millisPerMinute
	return ts, true
}

func parseDigits(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	value := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		value = value*10 + int(c-'0')
	}
	return value, true
}

func parseTimezoneOffset(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	if b[0] == 'Z' {
		return 0, true
	}
	if len(b) != 6 || (b[0] != '+' && b[0] != '-') {
		return 0, false
	}
	hours, ok := parseDigits(b[1:3])
	if !ok {
		return 0, false
	}
	minutes, ok := parseDigits(b[4:6])
	if !ok {
		return 0, false
	}
	if b[3] != ':' {
		return 0, false
	}
	offset := hours*60 + minutes
	if b[0] == '-' {
		offset = -offset
	}
	return offset, true
}

// daysFromCivil converts a Gregorian date to days since the Unix epoch,
// validating month and day ranges ("days_from_civil" algorithm).
func daysFromCivil(y, m, d int64) (int64, bool) {
	if m < 1 || m > 12 || d < 1 {
		return 0, false
	}
	daysInMonth := []int64{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if m == 2 && isLeapYear(y) {
		daysInMonth[1] = 29
	}
	if d > daysInMonth[m-1] {
		return 0, false
	}
	// March-based months so leap days land at the end of the cycle.
	var mp int64
	var y2 int64
	if m > 2 {
		mp = m - 3
		y2 = y
	} else {
		mp = m + 9
		y2 = y - 1
	}
	era := floorDiv(y2, 400)                    // floor division for both signs
	yoe := y2 - era*400                         // [0, 399]
	doy := (153*mp+2)/5 + d - 1                 // [0, 365]
	doe := yoe*365 + yoe/4 - yoe/100 + doy      // [0, 146096]
	return era*146097 + doe - 719468, true
}

func isLeapYear(y int64) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// ParseTZ resolves an IANA timezone name; nil means system timezone. Unknown
// names silently fall back to the system timezone, matching the reference.
func ParseTZ(timezone *string) *time.Location {
	if timezone == nil {
		return nil
	}
	loc, err := time.LoadLocation(*timezone)
	if err != nil {
		return nil
	}
	return loc
}

// FormatDateTZ renders the local date of timestamp in tz (nil = system).
func FormatDateTZ(timestamp int64, tz *time.Location) string {
	t := time.UnixMilli(timestamp)
	if tz != nil {
		t = t.In(tz)
	}
	y, m, d := t.Date()
	return fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
}

// FormatRFC3339Millis renders a UTC RFC3339 timestamp with milliseconds.
func FormatRFC3339Millis(timestamp int64) string {
	t := time.UnixMilli(timestamp).UTC()
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d.%03dZ",
		t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1e6)
}

// DateWithinRange reports whether a YYYY-MM-DD date falls in the inclusive
// since/until window, compared as compact YYYYMMDD text so partial bounds such
// as "2026" keep working.
func DateWithinRange(date string, since, until *string) bool {
	if since == nil && until == nil {
		return true
	}
	compact := strings.ReplaceAll(date, "-", "")
	if since != nil && compact < *since {
		return false
	}
	if until != nil && compact > *until {
		return false
	}
	return true
}

// UTCNow returns the current Unix time in milliseconds.
func UTCNow() int64 {
	return time.Now().UnixMilli()
}
