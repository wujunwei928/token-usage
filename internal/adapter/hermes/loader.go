package hermes

import (
	"sort"
	"strings"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/core"
)

// LoadEntries reads every Hermes state database, keeping the first record
// per session id across databases.
func LoadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	pricing := loadPricing(shared)
	tz := core.ParseTZ(shared.Timezone)
	dbPaths, err := StateDBPaths()
	if err != nil {
		return nil, err
	}
	loaded := common.ReadFilesParallel(dbPaths, shared.SingleThread, func(dbPath string) []hermesEntry {
		return loadStateDBEntries(dbPath, shared)
	})
	var entries []core.LoadedEntry
	seenSessions := map[string]bool{}
	for _, dbEntries := range loaded {
		for i := range dbEntries {
			entry := &dbEntries[i]
			if seenSessions[entry.sessionID] {
				continue
			}
			seenSessions[entry.sessionID] = true
			entries = append(entries, toLoadedEntry(entry, tz, pricing))
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

// loadStateDBEntries reads the sessions table of one state database.
func loadStateDBEntries(dbPath string, shared *core.SharedArgs) []hermesEntry {
	db, err := openSQLiteDatabase(dbPath)
	if err != nil {
		core.DebugLog(shared, "Failed to open Hermes state database: "+dbPath)
		return nil
	}
	root, columns, ok := db.tableInfo("sessions")
	if !ok {
		core.DebugLog(shared, "Failed to read Hermes state database: "+dbPath)
		return nil
	}
	indexes, ok := columnIndexes(columns)
	if !ok {
		core.DebugLog(shared, "Failed to read Hermes state database: "+dbPath)
		return nil
	}
	var entries []hermesEntry
	_ = db.scanTable(root, func(rowid int64, record []sqliteValue) bool {
		// WHERE model IS NOT NULL AND TRIM(model) != ''
		modelIndex := indexes[1]
		if modelIndex >= len(record) || record[modelIndex].isNull || !record[modelIndex].isText {
			return true
		}
		if strings.TrimSpace(string(record[modelIndex].text)) == "" {
			return true
		}
		if entry, ok := readSessionRow(record, indexes); ok {
			entries = append(entries, *entry)
		}
		return true
	})
	return entries
}

// loadPricing builds the pricing table like the reference adapters.
func loadPricing(shared *core.SharedArgs) *core.PricingMap {
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
}
