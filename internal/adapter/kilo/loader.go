package kilo

import (
	"database/sql"
	"sort"
	"time"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadEntries reads every discovered Kilo database and returns deduplicated,
// time-ordered entries. The dedupe key is the embedded message id, so the
// first copy of a message wins across data directories.
func LoadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	tz := core.ParseTZ(shared.Timezone)
	var dbPaths []string
	for _, dir := range DataDirs() {
		if dbPath := DBPath(dir); dbPath != "" {
			dbPaths = append(dbPaths, dbPath)
		}
	}
	loaded := common.ReadFilesParallel(dbPaths, shared.SingleThread, func(dbPath string) []core.LoadedEntry {
		return loadEntriesFromDatabase(dbPath, tz, shared, pricing)
	})
	var entries []core.LoadedEntry
	seen := map[string]struct{}{}
	for _, dbEntries := range loaded {
		for i := range dbEntries {
			if dbEntries[i].Data.Message.ID == nil {
				entries = append(entries, dbEntries[i])
				continue
			}
			id := *dbEntries[i].Data.Message.ID
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			entries = append(entries, dbEntries[i])
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

func loadEntriesFromDatabase(dbPath string, tz *time.Location, shared *core.SharedArgs, pricing *core.PricingMap) []core.LoadedEntry {
	db, err := openReadOnly(dbPath)
	if err != nil {
		core.DebugLog(shared, "Failed to open Kilo database: "+dbPath)
		return nil
	}
	defer db.Close()
	rows, err := db.Query("SELECT id, session_id, data FROM message")
	if err != nil {
		core.DebugLog(shared, "Failed to read Kilo database: "+dbPath)
		return nil
	}
	defer rows.Close()

	var entries []core.LoadedEntry
	for rows.Next() {
		var id, sessionID, data sql.NullString
		if err := rows.Scan(&id, &sessionID, &data); err != nil {
			core.DebugLog(shared, "Failed to query Kilo database: "+dbPath)
			return entries
		}
		if !id.Valid || !sessionID.Valid || !data.Valid {
			continue
		}
		value, ok := parseKiloMessage(data.String)
		if !ok {
			continue
		}
		if entry := MessageValueToEntry(value, id.String, sessionID.String, dbPath, tz, shared.Mode, pricing); entry != nil {
			entries = append(entries, *entry)
		}
	}
	if err := rows.Err(); err != nil {
		core.DebugLog(shared, "Failed to query Kilo database: "+dbPath)
	}
	return entries
}

// FilterEntriesByDate keeps entries whose local date falls in the inclusive
// since/until window.
func FilterEntriesByDate(entries []core.LoadedEntry, shared *core.SharedArgs) []core.LoadedEntry {
	if shared.Since == nil && shared.Until == nil {
		return entries
	}
	out := make([]core.LoadedEntry, 0, len(entries))
	for i := range entries {
		if core.DateWithinRange(entries[i].Date, shared.Since, shared.Until) {
			out = append(out, entries[i])
		}
	}
	return out
}
