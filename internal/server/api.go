package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxReportBytes caps one Report Snapshot request body.
const maxReportBytes = 2 << 20 // 2 MiB

// maxRowsPerReport caps the cells for one day (a day has at most
// 24 hours × tools × models; anything beyond is abuse or a bug).
const maxRowsPerReport = 24 * 64

// Backfill caps: one request may carry a run of days (the client's --since
// backfill); half a year is ~180 days, we accept up to 550 with a generous
// total-row budget while staying far below the 2MB body cap in practice.
const (
	maxDaysPerReport     = 550
	maxTotalRowsBackfill = 200_000
)

// reportWindow / reportWindowMax form the per-token rate limit: at most 60
// reports per hour keeps an hourly timer comfortable while blocking floods.
const (
	reportWindow    = time.Hour
	reportWindowMax = 60
)

// DaySnapshot is one date's Hourly Usage inside a backfill report.
type DaySnapshot struct {
	Date  string    `json:"date"`
	Hours []HourRow `json:"hours"`
}

// ReportPayload is the Report Snapshot wire format (see the spec). The
// steady-state shape carries one day (date + hours); the backfill shape
// carries a run of days via the days array. The two are mutually exclusive
// and days wins when both appear.
type ReportPayload struct {
	DeviceID    string        `json:"deviceId"`
	DeviceLabel string        `json:"deviceLabel"`
	Date        string        `json:"date"`
	Timezone    string        `json:"timezone"`
	GeneratedAt string        `json:"generatedAt"`
	Hours       []HourRow     `json:"hours"`
	Days        []DaySnapshot `json:"days"`
}

// days returns the normalized per-date snapshots the payload carries.
func (p *ReportPayload) daySnapshots() ([]DaySnapshot, error) {
	if len(p.Days) > 0 {
		if len(p.Days) > maxDaysPerReport {
			return nil, fmt.Errorf("too many days in backfill (max %d)", maxDaysPerReport)
		}
		total := 0
		for i := range p.Days {
			day := &p.Days[i]
			if !validDateShape(day.Date) {
				return nil, fmt.Errorf("days[%d].date must be YYYY-MM-DD", i)
			}
			if len(day.Hours) > maxRowsPerReport {
				return nil, fmt.Errorf("days[%d] has too many hour rows (max %d)", i, maxRowsPerReport)
			}
			total += len(day.Hours)
			if total > maxTotalRowsBackfill {
				return nil, fmt.Errorf("backfill exceeds the %d total row cap", maxTotalRowsBackfill)
			}
			day.Hours = mergeDuplicateRows(day.Hours)
		}
		return p.Days, nil
	}
	if p.Date == "" || !validDateShape(p.Date) {
		return nil, errors.New("date must be YYYY-MM-DD")
	}
	return []DaySnapshot{{Date: p.Date, Hours: mergeDuplicateRows(p.Hours)}}, nil
}

// tokenLimiter is a fixed-window per-token request counter.
type tokenLimiter struct {
	mu      sync.Mutex
	buckets map[string]*limiterBucket
}

type limiterBucket struct {
	windowStart time.Time
	count       int
}

func newTokenLimiter() *tokenLimiter {
	return &tokenLimiter{buckets: map[string]*limiterBucket{}}
}

func (l *tokenLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok || now.Sub(b.windowStart) >= reportWindow {
		l.buckets[key] = &limiterBucket{windowStart: now, count: 1}
		return true
	}
	if b.count >= reportWindowMax {
		return false
	}
	b.count++
	return true
}

// API serves the ingest endpoints.
type API struct {
	Store   *Store
	Limiter *tokenLimiter
	Logger  *log.Logger
}

// NewAPI builds the ingest API around a store.
func NewAPI(store *Store, logger *log.Logger) *API {
	if logger == nil {
		logger = log.Default()
	}
	return &API{Store: store, Limiter: newTokenLimiter(), Logger: logger}
}

// Register mounts the API routes on mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/report", a.handleReport)
}

// apiError writes a JSON error envelope.
func apiError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"accepted": false,
		"code":     code,
		"message":  message,
	})
}

// bearerToken extracts the Authorization: Bearer credential.
func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if len(value) > 7 && strings.EqualFold(value[:7], "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

// validDateShape checks a strict YYYY-MM-DD with a plausible month/day.
func validDateShape(date string) bool {
	if len(date) != 10 || date[4] != '-' || date[7] != '-' {
		return false
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return false
	}
	return true
}

// validatePayload applies the strict acceptance rules; violations return a
// user-readable message. Day-level rules live in daySnapshots.
func validatePayload(p *ReportPayload) error {
	if p.DeviceID == "" || len(p.DeviceID) > 64 {
		return errors.New("deviceId must be 1-64 characters")
	}
	if len(p.DeviceLabel) > 128 {
		return errors.New("deviceLabel too long (max 128)")
	}
	if len(p.Timezone) > 64 {
		return errors.New("timezone too long (max 64)")
	}
	if len(p.Hours) > 0 && len(p.Hours) > maxRowsPerReport {
		return fmt.Errorf("too many hour rows (max %d)", maxRowsPerReport)
	}
	if _, err := p.daySnapshots(); err != nil {
		return err
	}
	// Row-level rules apply to every day's cells (single-day shape included).
	all := p.allRows()
	for i := range all {
		r := &all[i]
		if r.Hour > 23 {
			return fmt.Errorf("hours[%d].hour must be 0-23", i)
		}
		if r.Tool == "" || len(r.Tool) > 32 {
			return fmt.Errorf("hours[%d].tool must be 1-32 characters", i)
		}
		if r.Model == "" || len(r.Model) > 128 {
			return fmt.Errorf("hours[%d].model must be 1-128 characters", i)
		}
	}
	return nil
}

// allRows returns every cell of the payload (single-day or backfill shape).
func (p *ReportPayload) allRows() []HourRow {
	if len(p.Days) > 0 {
		var out []HourRow
		for i := range p.Days {
			out = append(out, p.Days[i].Hours...)
		}
		return out
	}
	return p.Hours
}

// mergeDuplicateRows folds repeated (hour, tool, model) cells so a sloppy
// client cannot violate the primary key.
func mergeDuplicateRows(rows []HourRow) []HourRow {
	type key struct {
		hour  int
		tool  string
		model string
	}
	seen := map[key]int{}
	out := make([]HourRow, 0, len(rows))
	for _, r := range rows {
		k := key{r.Hour, r.Tool, r.Model}
		if idx, ok := seen[k]; ok {
			out[idx].Input += r.Input
			out[idx].Output += r.Output
			out[idx].CacheRead += r.CacheRead
			out[idx].Cache5m += r.Cache5m
			out[idx].Cache1h += r.Cache1h
			continue
		}
		seen[k] = len(out)
		out = append(out, r)
	}
	return out
}

// handleReport implements POST /v1/report.
func (a *API) handleReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := bearerToken(r)
	if token == "" {
		apiError(w, http.StatusUnauthorized, "missing_token", "Authorization: Bearer <token> required")
		return
	}
	user, err := a.Store.UserForToken(ctx, token)
	if err != nil {
		apiError(w, http.StatusUnauthorized, "invalid_token", "unknown or revoked token")
		return
	}
	if !a.Limiter.allow(token, time.Now()) {
		apiError(w, http.StatusTooManyRequests, "rate_limited",
			fmt.Sprintf("rate limit: %d reports per hour", reportWindowMax))
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReportBytes))
	if err != nil {
		apiError(w, http.StatusRequestEntityTooLarge, "body_too_large",
			fmt.Sprintf("request body exceeds %d bytes", maxReportBytes))
		return
	}
	var payload ReportPayload
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		apiError(w, http.StatusBadRequest, "bad_json", "malformed report payload: "+err.Error())
		return
	}
	if err := validatePayload(&payload); err != nil {
		apiError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	days, err := payload.daySnapshots()
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	// One ReplaceDay per date: each (device, date) stays an independent
	// Latest-wins unit, so a backfill re-run is idempotent per day.
	var totalTokens uint64
	for i := range days {
		dayTokens, err := a.Store.ReplaceDay(ctx, user.ID, payload.DeviceID, payload.DeviceLabel, days[i].Date, days[i].Hours)
		if err != nil {
			if errors.Is(err, ErrTooManyDevices) {
				apiError(w, http.StatusConflict, "device_limit",
					fmt.Sprintf("each user may bind at most %d devices; revoke an old device or reuse one", MaxDevicesPerUser))
				return
			}
			a.Logger.Printf("report ingest error: %v", err)
			apiError(w, http.StatusInternalServerError, "internal", "failed to store report")
			return
		}
		totalTokens += dayTokens
	}
	devices, _ := a.Store.CountDevices(ctx, user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"accepted":    true,
		"deviceCount": devices,
		"dayTokens":   strconv.FormatUint(totalTokens, 10),
		"days":        len(days),
		"dayDate":     days[len(days)-1].Date,
	})
}

// SeedUser is the CLI-facing helper: creates a user and returns the plaintext
// token exactly once.
func SeedUser(ctx context.Context, store *Store, name, passwordHash, city string) (token string, err error) {
	userID, err := store.CreateUser(ctx, name, passwordHash, city, "")
	if err != nil {
		return "", err
	}
	token = GenerateToken()
	if err := store.CreateToken(ctx, userID, "seed", HashToken(token)); err != nil {
		return "", err
	}
	return token, nil
}
