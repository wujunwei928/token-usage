package goose

import (
	"database/sql"
	"sort"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// gooseSessionQuery mirrors the reference query: rows without a model config
// are excluded up front.
const gooseSessionQuery = `
SELECT
    id,
    model_config_json,
    provider_name,
    created_at,
    total_tokens,
    input_tokens,
    output_tokens,
    accumulated_total_tokens,
    accumulated_input_tokens,
    accumulated_output_tokens
FROM sessions
WHERE model_config_json IS NOT NULL
    AND TRIM(model_config_json) != ''
`

// LoadEntries reads every discovered Goose database and returns deduplicated,
// time-ordered entries.
func LoadEntries(shared *core.SharedArgs, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	tz := core.ParseTZ(shared.Timezone)
	dbPaths := DBPaths()
	// Load each database in parallel (a fresh read-only connection per DB),
	// then run the sequential per-db dedup over the original path order.
	loaded := common.ReadFilesParallel(dbPaths, shared.SingleThread, func(dbPath string) []core.LoadedEntry {
		entries, err := loadEntriesFromDB(dbPath, tz, pricing)
		if err != nil {
			core.DebugLog(shared, "Failed to load Goose database "+dbPath+": "+err.Error())
			return nil
		}
		return entries
	})
	var entries []core.LoadedEntry
	seen := map[string]struct{}{}
	for i, dbEntries := range loaded {
		for j := range dbEntries {
			key := dbPaths[i] + ":" + dbEntries[j].SessionID
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				entries = append(entries, dbEntries[j])
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

func loadEntriesFromDB(dbPath string, tz *time.Location, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(gooseSessionQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []core.LoadedEntry
	for rows.Next() {
		var id, modelConfig, createdAt sql.NullString
		var provider sql.NullString
		var total, input, output, accTotal, accInput, accOutput sql.RawBytes
		if err := rows.Scan(&id, &modelConfig, &provider, &createdAt, &total, &input, &output, &accTotal, &accInput, &accOutput); err != nil {
			return nil, err
		}
		// id/model_config/created_at must be non-NULL (the reference's
		// read::<String> failures skip the row); provider may be NULL.
		if !id.Valid || !modelConfig.Valid {
			continue
		}
		if !createdAt.Valid {
			continue
		}
		row := &gooseRow{
			ID:           id.String,
			ModelConfig:  modelConfig.String,
			ProviderName: nullStringPtr(provider),
			CreatedAt:    createdAt.String,
			TotalTokens:  tokenFromRaw(total),
			InputTokens:  tokenFromRaw(input),
			OutputTokens: tokenFromRaw(output),
			AccumTotal:   tokenFromRaw(accTotal),
			AccumInput:   tokenFromRaw(accInput),
			AccumOutput:  tokenFromRaw(accOutput),
		}
		if entry := rowToEntry(row, tz, pricing); entry != nil {
			entries = append(entries, *entry)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// tokenFromRaw coerces a column value to i64 the way sqlite3_column_int64
// does: integers stay, text/real coerce, NULL and unparseable text become
// absent.
func tokenFromRaw(raw sql.RawBytes) tokenValue {
	if raw == nil {
		return tokenValue{}
	}
	text := string(raw)
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return tokenValue{value: value, ok: true}
	}
	if value, err := strconv.ParseFloat(text, 64); err == nil {
		return tokenValue{value: int64(value), ok: true}
	}
	return tokenValue{value: 0, ok: false}
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
