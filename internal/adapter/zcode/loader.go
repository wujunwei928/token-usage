package zcode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/wujunwei928/token-usage/internal/core"
)

// sessionRow carries the attribution fields of one zcode session.
type sessionRow struct {
	id        string
	parentID  string
	directory string
}

// LoadEntries reads every Attempt row from the analytics database (one
// model_usage row per model API call attempt) and normalizes it into
// LoadedEntry rows. Retries, failed attempts, and auxiliary calls (session
// titles and friends) are real consumption and all count; reasoning tokens
// fold into output; subagent sessions attribute to their top-level parent,
// mirroring the Claude adapter's sidechain semantics. Sessions created before
// the model_usage table existed fall back to the message table's per-message
// tokens, so one session is never counted from both sources. Pricing is only
// loaded outside display mode.
func LoadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	return core.TrackUsageLoad("zcode", shared, func() ([]core.LoadedEntry, error) {
		return loadEntries(shared)
	})
}

func loadEntries(shared *core.SharedArgs) ([]core.LoadedEntry, error) {
	var pricing *core.PricingMap
	if shared.Mode != core.ModeDisplay {
		refreshLog := true
		if level := core.LogLevel(); level != nil && *level == 0 {
			refreshLog = false
		}
		pricing = core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
	}
	tz := core.ParseTZ(shared.Timezone)
	var entries []core.LoadedEntry
	seen := map[string]bool{}
	for _, path := range DBPaths() {
		loaded, err := loadDB(path, tz, shared.Mode, pricing)
		if err != nil {
			return nil, fmt.Errorf("zcode: %w", err)
		}
		for i := range loaded {
			key := ""
			if loaded[i].Data.RequestID != nil {
				key = *loaded[i].Data.RequestID
			}
			// Only real ids dedup across databases; an empty key must
			// never collapse distinct entries.
			if key != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			entries = append(entries, loaded[i])
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp < entries[j].Timestamp })
	return entries, nil
}

func loadDB(path string, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := requireSchema(db); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sessions, err := loadSessions(db)
	if err != nil {
		return nil, err
	}
	hasModelUsage, err := hasTable(db, "model_usage")
	if err != nil {
		return nil, err
	}
	var entries []core.LoadedEntry
	if hasModelUsage {
		attempts, err := loadAttempts(db, sessions, tz, mode, pricing)
		if err != nil {
			return nil, err
		}
		entries = append(entries, attempts...)
	}
	fallback, err := loadMessageFallback(db, hasModelUsage, sessions, tz, mode, pricing)
	if err != nil {
		return nil, err
	}
	return append(entries, fallback...), nil
}

// loadAttempts reads model_usage: one row per API call attempt, in start
// order. Zero-token attempts (e.g. cancelled before the first token) carry no
// usage and are dropped.
func loadAttempts(db *sql.DB, sessions map[string]sessionRow, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	rows, err := db.Query(`
		SELECT id, session_id, model_id, started_at,
		       input_tokens, output_tokens, reasoning_tokens,
		       cache_creation_input_tokens, cache_read_input_tokens
		FROM model_usage
		ORDER BY started_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []core.LoadedEntry
	for rows.Next() {
		var id, sessionID, model string
		var startedAt int64
		var input, output, reasoning, cacheCreation, cacheRead sql.NullInt64
		if err := rows.Scan(&id, &sessionID, &model, &startedAt,
			&input, &output, &reasoning, &cacheCreation, &cacheRead); err != nil {
			return nil, err
		}
		usage := core.TokenUsageRaw{
			InputTokens:              clampU64(input),
			OutputTokens:             clampU64(output) + clampU64(reasoning),
			CacheCreationInputTokens: clampU64(cacheCreation),
			CacheReadInputTokens:     clampU64(cacheRead),
		}
		if core.TotalUsageTokens(usage) == 0 {
			continue
		}
		entries = append(entries, newEntry(id, sessions, sessionID, model, startedAt, usage, tz, mode, pricing))
	}
	return entries, rows.Err()
}

// messageTokens is the token-bearing subset of a message row's data JSON.
type messageTokens struct {
	Input     *uint64       `json:"input"`
	Output    *uint64       `json:"output"`
	Reasoning *uint64       `json:"reasoning"`
	Cache     *messageCache `json:"cache"`
}

type messageCache struct {
	Read  *uint64 `json:"read"`
	Write *uint64 `json:"write"`
}

type messageData struct {
	ModelID string         `json:"modelID"`
	Tokens  *messageTokens `json:"tokens"`
}

// loadMessageFallback restores history for sessions with no model_usage rows
// (created before the CLI introduced that table): their tokens survive only
// in the message table's per-message data JSON. The NOT EXISTS guard keeps
// every session on exactly one source, so fallback and attempt rows never
// double count.
func loadMessageFallback(db *sql.DB, hasModelUsage bool, sessions map[string]sessionRow, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) ([]core.LoadedEntry, error) {
	query := `SELECT id, session_id, time_created, data FROM message ORDER BY time_created, id`
	if hasModelUsage {
		query = `
			SELECT m.id, m.session_id, m.time_created, m.data
			FROM message m
			WHERE NOT EXISTS (SELECT 1 FROM model_usage u WHERE u.session_id = m.session_id)
			ORDER BY m.time_created, m.id`
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []core.LoadedEntry
	for rows.Next() {
		var id, sessionID, data string
		var created int64
		if err := rows.Scan(&id, &sessionID, &created, &data); err != nil {
			return nil, err
		}
		var parsed messageData
		if err := json.Unmarshal([]byte(data), &parsed); err != nil || parsed.Tokens == nil {
			continue
		}
		var cacheRead, cacheWrite uint64
		if parsed.Tokens.Cache != nil {
			cacheRead = valueOf(parsed.Tokens.Cache.Read)
			cacheWrite = valueOf(parsed.Tokens.Cache.Write)
		}
		usage := core.TokenUsageRaw{
			InputTokens:              valueOf(parsed.Tokens.Input),
			OutputTokens:             valueOf(parsed.Tokens.Output) + valueOf(parsed.Tokens.Reasoning),
			CacheCreationInputTokens: cacheWrite,
			CacheReadInputTokens:     cacheRead,
		}
		if core.TotalUsageTokens(usage) == 0 {
			continue
		}
		model := parsed.ModelID
		if model == "" {
			model = "unknown"
		}
		entries = append(entries, newEntry("message:"+id, sessions, sessionID, model, created, usage, tz, mode, pricing))
	}
	return entries, rows.Err()
}

func newEntry(id string, sessions map[string]sessionRow, sessionID, model string, ts int64, usage core.TokenUsageRaw, tz *time.Location, mode core.CostMode, pricing *core.PricingMap) core.LoadedEntry {
	top := topSession(sessions, sessionID)
	directory := top.directory
	if directory == "" {
		directory = "unknown"
	}
	modelPtr := model
	sessionPtr := top.id
	idPtr := id
	return core.LoadedEntry{
		Data: core.UsageEntry{
			SessionID: &sessionPtr,
			Timestamp: core.FormatRFC3339Millis(ts),
			RequestID: &idPtr,
			Message: core.UsageMessage{
				Usage: usage,
				Model: &modelPtr,
			},
		},
		Timestamp:           ts,
		Date:                core.FormatDateTZ(ts, tz),
		Project:             "zcode",
		SessionID:           top.id,
		ProjectPath:         directory,
		Cost:                zcodeCalculateCost(model, usage, mode, pricing),
		Model:               &modelPtr,
		MissingPricingModel: zcodeMissingPricing(model, usage, mode, pricing),
	}
}

func loadSessions(db *sql.DB) (map[string]sessionRow, error) {
	rows, err := db.Query(`SELECT id, COALESCE(parent_id, ''), COALESCE(directory, '') FROM session`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]sessionRow{}
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.id, &s.parentID, &s.directory); err != nil {
			return nil, err
		}
		out[s.id] = s
	}
	return out, rows.Err()
}

// topSession resolves the top-level ancestor of a session, guarding against
// parent cycles and dangling references.
func topSession(sessions map[string]sessionRow, id string) sessionRow {
	current, ok := sessions[id]
	if !ok {
		return sessionRow{id: id}
	}
	seen := map[string]bool{id: true}
	for current.parentID != "" && !seen[current.parentID] {
		seen[current.parentID] = true
		parent, ok := sessions[current.parentID]
		if !ok {
			break
		}
		current = parent
	}
	return current
}

// requireSchema fails with a clear error when the analytics database lacks
// the tables this adapter reads (a zcode CLI schema change).
func requireSchema(db *sql.DB) error {
	for _, table := range []string{"session", "message"} {
		ok, err := hasTable(db, table)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("unsupported schema: table %q not found", table)
		}
	}
	return nil
}

func hasTable(db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// zcodeCalculateCost prices the entry when the model resolves in the pricing
// map; unknown models (GLM under a coding-plan subscription today) price as
// zero.
func zcodeCalculateCost(model string, usage core.TokenUsageRaw, mode core.CostMode, pricing *core.PricingMap) float64 {
	if mode == core.ModeDisplay || pricing == nil || pricing.Find(model) == nil {
		return 0
	}
	name := model
	return core.CalculateCostForUsage(&name, usage, nil, mode, pricing)
}

func zcodeMissingPricing(model string, usage core.TokenUsageRaw, mode core.CostMode, pricing *core.PricingMap) *string {
	if mode == core.ModeDisplay || pricing == nil {
		return nil
	}
	if core.TotalUsageTokens(usage) == 0 || pricing.Find(model) != nil {
		return nil
	}
	resolved := core.ResolveModelName(model)
	return &resolved
}

func clampU64(v sql.NullInt64) uint64 {
	if !v.Valid || v.Int64 < 0 {
		return 0
	}
	return uint64(v.Int64)
}

func valueOf(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}
