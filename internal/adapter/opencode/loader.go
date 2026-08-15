package opencode

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// sqliteDriver is registered by the modernc.org/sqlite import above.
const sqliteDriver = "sqlite"

// LoadEntries loads and dedups OpenCode usage entries across every data
// directory, sorted by timestamp.
func LoadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	paths, err := Paths()
	if err != nil {
		return nil, err
	}
	var entries []core.LoadedEntry
	seen := map[string]struct{}{}
	for _, path := range paths {
		dirEntries, err := loadEntriesFromDirectory(path, shared)
		if err != nil {
			return nil, err
		}
		for _, entry := range dirEntries {
			if id := entryID(&entry); id != "" {
				if _, dup := seen[id]; dup {
					continue
				}
				seen[id] = struct{}{}
			}
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})
	return entries, nil
}

// loadPricing builds the pricing table for token-priced runs; display mode
// skips pricing entirely.
func loadPricing(shared *core.SharedArgs) *core.PricingMap {
	if shared.Mode == core.ModeDisplay {
		return nil
	}
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
}

func loadEntriesFromDirectory(dir string, shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	pricing := loadPricing(shared)
	tz := core.ParseTZ(shared.Timezone)
	window := dateWindowFromShared(shared, tz)

	var entries []core.LoadedEntry
	seen := map[string]struct{}{}
	if db := dbPath(dir); db != "" {
		for _, entry := range loadEntriesFromDatabase(db, tz, shared.Mode, pricing, shared, window) {
			if id := entryID(&entry); id != "" {
				if _, dup := seen[id]; dup {
					continue
				}
				seen[id] = struct{}{}
			}
			entries = append(entries, entry)
		}
	}

	messagesDir := filepath.Join(dir, "storage", "message")
	var collected []string
	collectFilesWithExtension(messagesDir, "json", &collected)
	files := collected

	// Skip files the DB pass already covered: message files live at
	// storage/message/<sessionID>/<messageID>.json, so the stem is the
	// dedup id. The id dedup below would drop them anyway — skipping here
	// just avoids the read.
	if len(seen) > 0 {
		kept := files[:0]
		for _, file := range files {
			stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			if _, covered := seen[stem]; covered {
				continue
			}
			kept = append(kept, file)
		}
		files = kept
	}

	// Parallel reads preserve file order, so the sequential id dedup below
	// always sees the same duplicate regardless of thread count.
	loaded := common.ReadFilesParallel(files, shared.SingleThread, func(file string) *core.LoadedEntry {
		return readMessageFile(file, tz, shared.Mode, pricing, shared, window)
	})
	for _, entry := range loaded {
		if entry == nil {
			continue
		}
		if id := entryID(entry); id != "" {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
		}
		entries = append(entries, *entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})
	return entries, nil
}

// HasData reports whether an OpenCode install has usage data at all,
// independent of the requested date window (the loader narrows by
// --since/--until as it reads, so loaded entries cannot answer this).
func HasData() bool {
	paths, err := Paths()
	if err != nil {
		return false
	}
	for _, path := range paths {
		if hasSource(path) {
			return true
		}
	}
	return false
}

// hasSource reports whether the directory holds any usage source: the SQLite
// database, or at least one message file. It stops at the first message file
// rather than collecting them all.
func hasSource(dir string) bool {
	if dbPath(dir) != "" {
		return true
	}
	return hasJSONFile(filepath.Join(dir, "storage", "message"))
}

// hasJSONFile recursively checks for a *.json regular file. Entries are judged
// by lstat (like the reference's file_type()), so symlinks are neither
// followed nor counted.
func hasJSONFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			if hasJSONExtension(entry.Name()) {
				return true
			}
		} else if info.IsDir() {
			if hasJSONFile(filepath.Join(dir, entry.Name())) {
				return true
			}
		}
	}
	return false
}

// hasJSONExtension matches path.extension() == "json": a bare ".json" hidden
// file has no extension.
func hasJSONExtension(name string) bool {
	return strings.HasSuffix(name, ".json") && len(name) > len(".json")
}

// collectFilesWithExtension recursively gathers files whose name ends in
// "."+extension, judging by lstat like the reference.
func collectFilesWithExtension(dir, extension string, files *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if info.Mode().IsRegular() {
			suffix := "." + extension
			if name := entry.Name(); strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
				*files = append(*files, path)
			}
		} else if info.IsDir() {
			collectFilesWithExtension(path, extension, files)
		}
	}
}

// dbPath finds the SQLite database for an OpenCode install: opencode.db when
// present, else the first (sorted) opencode-<channel>.db file.
func dbPath(dir string) string {
	defaultPath := filepath.Join(dir, "opencode.db")
	if isRegularFile(defaultPath) {
		return defaultPath
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var candidates []string
	for _, entry := range entries {
		if !isChannelDBName(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if isRegularFile(path) {
			candidates = append(candidates, path)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

// isChannelDBName matches opencode-<channel>.db where channel is
// alphanumeric, '_' or '-' (possibly empty).
func isChannelDBName(name string) bool {
	const prefix, suffix = "opencode-", ".db"
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	channel, ok := strings.CutSuffix(rest, suffix)
	if !ok {
		return false
	}
	for i := 0; i < len(channel); i++ {
		b := channel[i]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-') {
			return false
		}
	}
	return true
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// loadEntriesFromDatabase reads the message table. Rows are narrowed by the
// (widened) window in SQL when the schema allows it, then checked against the
// exact payload window; any failure degrades to fewer rows, never an error.
func loadEntriesFromDatabase(
	dbPath string,
	tz *time.Location,
	mode core.CostMode,
	pricing *core.PricingMap,
	shared *core.SharedArgs,
	window dateWindow,
) []core.LoadedEntry {
	db, err := sql.Open(sqliteDriver, "file:"+dbPath+"?mode=ro")
	if err != nil {
		core.DebugLog(shared, "Failed to open OpenCode database: "+dbPath)
		return nil
	}
	defer db.Close()

	// Push the window into SQL only while a sample of time_created still
	// looks millisecond-scaled; the payload check in the loop stays
	// authoritative either way.
	var pushdown dateWindow
	if window.isUnbounded() || timeCreatedLooksLikeMillis(db) {
		pushdown = window.widenedForPushdown()
	} else {
		core.DebugLog(shared, "OpenCode time_created is not millisecond-scale; scanning unfiltered: "+dbPath)
		pushdown = dateWindow{}
	}
	query, args := messageQuery(pushdown)
	rows, err := db.QueryContext(context.Background(), query, args...)
	if err != nil {
		// A pre-SQLite-era schema has no time_created column, so the
		// filtered query cannot prepare; scan unfiltered instead.
		core.DebugLog(shared, "Failed to prepare filtered OpenCode query; scanning unfiltered: "+dbPath)
		query, args = messageQuery(dateWindow{})
		rows, err = db.QueryContext(context.Background(), query, args...)
		if err != nil {
			core.DebugLog(shared, "Failed to read OpenCode database: "+dbPath)
			return nil
		}
	}
	defer rows.Close()

	var entries []core.LoadedEntry
	bounded := !window.isUnbounded()
	for rows.Next() {
		var id, sessionID, data string
		if err := rows.Scan(&id, &sessionID, &data); err != nil {
			continue
		}
		if bounded {
			if millis, ok := extractMessageTimestamp(data); ok && !window.contains(millis) {
				continue
			}
		}
		if !utf8.ValidString(data) {
			continue
		}
		value := ParseOpenCodeMessage([]byte(data))
		if value == nil {
			continue
		}
		if entry := MessageValueToEntry(value, &id, &sessionID, tz, mode, pricing); entry != nil {
			entries = append(entries, *entry)
		}
	}
	if err := rows.Err(); err != nil {
		core.DebugLog(shared, "Failed to query OpenCode database: "+dbPath)
	}
	return entries
}

// messageQuery builds the message scan, narrowed to the window where its
// bounds are set. The bounds go through a subquery that selects only id so
// the (session_id, time_created, id) index answers the range without reading
// every data blob.
func messageQuery(window dateWindow) (string, []any) {
	switch {
	case window.start != nil && window.end != nil:
		return "SELECT id, session_id, data FROM message WHERE id IN (SELECT id FROM message WHERE time_created >= ? AND time_created < ?)",
			[]any{*window.start, *window.end}
	case window.start != nil:
		return "SELECT id, session_id, data FROM message WHERE id IN (SELECT id FROM message WHERE time_created >= ?)",
			[]any{*window.start}
	case window.end != nil:
		return "SELECT id, session_id, data FROM message WHERE id IN (SELECT id FROM message WHERE time_created < ?)",
			[]any{*window.end}
	default:
		return "SELECT id, session_id, data FROM message", nil
	}
}

// minMillisScale is the smallest value treated as millisecond scale: any Unix
// timestamp after 1973 needs at least 12 digits, while second-scale values
// stay far below it.
const minMillisScale = 100_000_000_000

// timeCreatedLooksLikeMillis reports whether a sample of message.time_created
// looks like Unix milliseconds, the scale the payload's time.created uses.
func timeCreatedLooksLikeMillis(db *sql.DB) bool {
	rows, err := db.QueryContext(context.Background(),
		"SELECT max(time_created) FROM (SELECT time_created FROM message LIMIT 8)")
	if err != nil {
		return false
	}
	defer rows.Close()
	if !rows.Next() {
		return false
	}
	var max int64
	if err := rows.Scan(&max); err != nil {
		return false
	}
	return max >= minMillisScale
}

func readMessageFile(
	path string,
	tz *time.Location,
	mode core.CostMode,
	pricing *core.PricingMap,
	shared *core.SharedArgs,
	window dateWindow,
) *core.LoadedEntry {
	content, err := os.ReadFile(path)
	if err != nil {
		core.DebugLog(shared, "Failed to read OpenCode message file "+path+": "+err.Error())
		return nil
	}
	if !utf8.Valid(content) {
		// serde rejects non-UTF-8 payloads, and the pre-parse window scan
		// below needs UTF-8 text anyway.
		return nil
	}
	// Skip out-of-range entries before the full parse. Extraction works on
	// the raw text and fails open (missing timestamp -> full parse).
	if !window.isUnbounded() {
		if millis, ok := extractMessageTimestamp(string(content)); ok && !window.contains(millis) {
			return nil
		}
	}
	value := ParseOpenCodeMessage(content)
	if value == nil {
		return nil
	}
	return MessageValueToEntry(value, nil, nil, tz, mode, pricing)
}

// entryID returns the message id used for dedup, or "" when absent.
func entryID(entry *core.LoadedEntry) string {
	if entry.Data.Message.ID == nil {
		return ""
	}
	return *entry.Data.Message.ID
}

// extractMessageTimestamp pulls time.created millis from raw JSON to skip
// rows before a full parse. Only the canonical "time": { ... "created":
// <digits> ... } shape is recognized, and the search for created never leaves
// that first time object, so a time object belonging to something else in the
// payload cannot contribute a number. Everything else returns ok=false and
// the caller falls back to a full parse: a scan that gives up costs time,
// whereas a scan that guesses wrong would silently drop an in-range entry.
func extractMessageTimestamp(data string) (int64, bool) {
	const timeKey = `"time":`
	const createdKey = `"created":`

	timeIndex := strings.Index(data, timeKey)
	if timeIndex < 0 {
		return 0, false
	}
	timeObject := trimStartSpace(data[timeIndex+len(timeKey):])
	if !strings.HasPrefix(timeObject, "{") {
		return 0, false
	}
	timeObject = timeObject[1:]
	objectEnd := strings.Index(timeObject, "}")
	if objectEnd < 0 {
		return 0, false
	}
	timeObject = timeObject[:objectEnd]
	createdIndex := strings.Index(timeObject, createdKey)
	if createdIndex < 0 {
		return 0, false
	}
	afterKey := trimStartSpace(timeObject[createdIndex+len(createdKey):])
	end := len(afterKey)
	for i := 0; i < len(afterKey); i++ {
		if afterKey[i] < '0' || afterKey[i] > '9' {
			end = i
			break
		}
	}
	millis, err := strconv.ParseInt(afterKey[:end], 10, 64)
	if err != nil {
		return 0, false
	}
	return millis, true
}

// trimStartSpace mirrors Rust's str::trim_start (Unicode whitespace).
func trimStartSpace(s string) string {
	return strings.TrimLeftFunc(s, unicode.IsSpace)
}

// dateWindow is the half-open millisecond window derived from --since/--until
// used to skip rows and files before they are parsed. A nil bound means that
// side is not narrowed, either because the option was absent or because it is
// not a full date. It is deliberately equivalent to the authoritative
// date_within_range check applied to loaded entries: both resolve the bounds
// in the reporting timezone.
type dateWindow struct {
	start *int64
	end   *int64
}

func dateWindowFromShared(shared *core.SharedArgs, tz *time.Location) dateWindow {
	start, end := dateRangeBoundsMS(shared.Since, shared.Until, tz)
	return dateWindow{start: start, end: end}
}

func (w dateWindow) isUnbounded() bool {
	return w.start == nil && w.end == nil
}

// widenedForPushdown returns the same window widened by a day on each side
// for the SQL push-down: the time_created column is only a proxy for the
// payload's time.created, so drift up to a day costs a few extra scanned rows
// instead of excluding a row the report wanted.
func (w dateWindow) widenedForPushdown() dateWindow {
	const millisPerDay = 24 * 60 * 60 * 1000
	widened := dateWindow{}
	if w.start != nil {
		value := *w.start - millisPerDay
		widened.start = &value
	}
	if w.end != nil {
		value := *w.end + millisPerDay
		widened.end = &value
	}
	return widened
}

func (w dateWindow) contains(millis int64) bool {
	if w.start != nil && millis < *w.start {
		return false
	}
	if w.end != nil && millis >= *w.end {
		return false
	}
	return true
}

// dateRangeBoundsMS resolves the --since/--until window to half-open Unix
// millisecond bounds [since 00:00, the day after until 00:00) in tz. A nil
// bound is not narrowed: either the option was absent, or it is not a full
// YYYYMMDD date and has no instant to compare against. A nil tz means the
// system timezone.
func dateRangeBoundsMS(since, until *string, tz *time.Location) (*int64, *int64) {
	var start, end *int64
	if since != nil {
		if day, ok := startOfDayMS(*since, tz); ok {
			start = &day
		}
	}
	if until != nil {
		if date, ok := parseCompactDate(*until); ok {
			next := addOneDay(date)
			nextCompact := fmt.Sprintf("%04d%02d%02d", next.year, next.month, next.day)
			if day, ok := startOfDayMS(nextCompact, tz); ok {
				end = &day
			}
		}
	}
	return start, end
}

// startOfDayMS parses a compact YYYYMMDD date and returns the first instant
// of that day in tz as Unix milliseconds.
func startOfDayMS(value string, tz *time.Location) (int64, bool) {
	date, ok := parseCompactDate(value)
	if !ok {
		return 0, false
	}
	if tz == nil {
		tz = time.Local
	}
	// Rebuild the civil date in the reporting timezone so the instant is
	// local midnight, not UTC midnight re-displayed.
	return time.Date(date.year, time.Month(date.month), date.day, 0, 0, 0, 0, tz).UnixMilli(), true
}

// civilDate is a validated calendar date.
type civilDate struct {
	year  int
	month int
	day   int
}

// parseCompactDate parses a full YYYYMMDD date; false for anything else,
// including partial bounds like "202601".
func parseCompactDate(value string) (civilDate, bool) {
	if len(value) != 8 {
		return civilDate{}, false
	}
	for i := 0; i < 8; i++ {
		if value[i] < '0' || value[i] > '9' {
			return civilDate{}, false
		}
	}
	year, _ := strconv.Atoi(value[:4])
	month, _ := strconv.Atoi(value[4:6])
	day, _ := strconv.Atoi(value[6:8])
	daysInMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if month == 2 && isLeapYear(year) {
		daysInMonth[1] = 29
	}
	if month < 1 || month > 12 || day < 1 || day > daysInMonth[month-1] {
		return civilDate{}, false
	}
	return civilDate{year: year, month: month, day: day}, true
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// addOneDay advances a civil date by one day (calendar arithmetic, so month
// and year rollovers land on the right calendar day).
func addOneDay(date civilDate) civilDate {
	daysInMonth := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if date.month == 2 && isLeapYear(date.year) {
		daysInMonth[1] = 29
	}
	if date.day < daysInMonth[date.month-1] {
		return civilDate{date.year, date.month, date.day + 1}
	}
	if date.month < 12 {
		return civilDate{date.year, date.month + 1, 1}
	}
	return civilDate{date.year + 1, 1, 1}
}
