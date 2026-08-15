package server

import (
	"context"
	"time"
)

// Filters select the Hourly Usage rows a view aggregates over. Zero values
// mean "no restriction"; dates are inclusive YYYY-MM-DD bounds (client-local
// Report Dates, mixed timezones accepted by design).
type Filters struct {
	From  string // "" = unbounded
	To    string // "" = unbounded
	Tool  string
	Model string
	City  string
	// IncludeCache selects the Cache 口径: true counts all five classes,
	// false counts input+output only.
	IncludeCache bool
	// IncludeFlagged keeps Anomaly-Flagged rows (owner views only).
	IncludeFlagged bool
	// User restricts to one user's rows (dashboard).
	User *User
}

// UsageCell is one aggregated (tool, model) slice with the five counters.
type UsageCell struct {
	Tool      string
	Model     string
	Input     uint64
	Output    uint64
	CacheRead uint64
	Cache5m   uint64
	Cache1h   uint64
}

// TokensIn returns the cell total under the given Cache 口径.
func (c UsageCell) TokensIn(includeCache bool) uint64 {
	if includeCache {
		return c.Input + c.Output + c.CacheRead + c.Cache5m + c.Cache1h
	}
	return c.Input + c.Output
}

// rawRow is one flat database row pulled into Go for aggregation.
type rawRow struct {
	DeviceID  string
	Date      string
	Hour      int
	Tool      string
	Model     string
	Input     uint64
	Output    uint64
	CacheRead uint64
	Cache5m   uint64
	Cache1h   uint64
	UserID    int64
	UserName  string
	City      string
	Avatar    string
	Flagged   bool
}

// fetchRows loads the filtered Hourly Usage rows with user identity joined.
func (s *Store) fetchRows(ctx context.Context, f Filters) ([]rawRow, error) {
	where := "h.flagged = 0"
	args := []any{}
	if f.IncludeFlagged {
		where = "1=1"
	}
	if f.From != "" {
		where += " AND h.date >= ?"
		args = append(args, f.From)
	}
	if f.To != "" {
		where += " AND h.date <= ?"
		args = append(args, f.To)
	}
	if f.Tool != "" {
		where += " AND h.tool = ?"
		args = append(args, f.Tool)
	}
	if f.Model != "" {
		where += " AND h.model = ?"
		args = append(args, f.Model)
	}
	if f.City != "" {
		where += " AND u.city = ?"
		args = append(args, f.City)
	}
	if f.User != nil {
		where += " AND u.id = ?"
		args = append(args, f.User.ID)
	}
	query := `SELECT h.device_id, h.date, h.hour, h.tool, h.model,
			h.input, h.output, h.cache_read, h.cache_5m, h.cache_1h, h.flagged,
			u.id, u.name, u.city, u.avatar
		FROM hourly_usage h
		JOIN devices d ON d.device_id = h.device_id
		JOIN users u ON u.id = d.user_id
		WHERE ` + where + `
		ORDER BY u.id, h.date, h.hour`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rawRow
	for rows.Next() {
		var r rawRow
		var flagged int
		if err := rows.Scan(&r.DeviceID, &r.Date, &r.Hour, &r.Tool, &r.Model,
			&r.Input, &r.Output, &r.CacheRead, &r.Cache5m, &r.Cache1h, &flagged,
			&r.UserID, &r.UserName, &r.City, &r.Avatar); err != nil {
			return nil, err
		}
		r.Flagged = flagged != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// LeaderRow is one ranked user on the Leaderboard.
type LeaderRow struct {
	Rank    int
	UserID  int64
	Name    string
	City    string
	Tokens  uint64
	Cost    float64
	Devices int
	Models  []ModelSlice
	Flagged bool
}

// ModelSlice is one model's contribution to a row's card badge list.
type ModelSlice struct {
	Model  string
	Tokens uint64
}

// Leaderboard aggregates rows into ranked users under the filters.
func (s *Store) Leaderboard(ctx context.Context, f Filters, pricing *PricingTable) []LeaderRow {
	rows, err := s.fetchRows(ctx, f)
	if err != nil {
		return nil
	}
	type acc struct {
		userID  int64
		name    string
		city    string
		tokens  uint64
		cost    float64
		devices map[string]bool
		models  map[string]uint64
		flagged bool
	}
	order := []int64{}
	accs := map[int64]*acc{}
	for i := range rows {
		r := &rows[i]
		a, ok := accs[r.UserID]
		if !ok {
			a = &acc{userID: r.UserID, name: r.UserName, city: r.City, devices: map[string]bool{}, models: map[string]uint64{}}
			accs[r.UserID] = a
			order = append(order, r.UserID)
		}
		cell := UsageCell{Tool: r.Tool, Model: r.Model, Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, Cache5m: r.Cache5m, Cache1h: r.Cache1h}
		a.tokens += cell.TokensIn(f.IncludeCache)
		a.cost += pricing.CostForHourRow(&HourRow{Hour: r.Hour, Tool: r.Tool, Model: r.Model, Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, Cache5m: r.Cache5m, Cache1h: r.Cache1h})
		a.devices[r.DeviceID] = true
		a.models[r.Model] += cell.TokensIn(f.IncludeCache)
		if r.Flagged {
			a.flagged = true
		}
	}
	out := make([]LeaderRow, 0, len(accs))
	for _, id := range order {
		a := accs[id]
		row := LeaderRow{UserID: a.userID, Name: a.name, City: a.city, Tokens: a.tokens, Cost: a.cost, Flagged: a.flagged}
		row.Devices = len(a.devices)
		for model, tokens := range a.models {
			if tokens > 0 {
				row.Models = append(row.Models, ModelSlice{Model: model, Tokens: tokens})
			}
		}
		sortModelSlices(row.Models)
		// Card badges show the top four models; the rest are noise.
		if len(row.Models) > 4 {
			row.Models = row.Models[:4]
		}
		out = append(out, row)
	}
	// Rank by tokens desc, name asc for determinism.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if out[j].Tokens > out[j-1].Tokens ||
				(out[j].Tokens == out[j-1].Tokens && out[j].Name < out[j-1].Name) {
				out[j], out[j-1] = out[j-1], out[j]
				continue
			}
			break
		}
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func sortModelSlices(models []ModelSlice) {
	for i := 1; i < len(models); i++ {
		for j := i; j > 0; j-- {
			if models[j].Tokens > models[j-1].Tokens {
				models[j], models[j-1] = models[j-1], models[j]
				continue
			}
			break
		}
	}
}

// CommunityTotals powers the leaderboard summary card.
type CommunityTotals struct {
	Tokens  uint64
	Cost    float64
	Users   int
	Devices int
}

// CommunityTotals aggregates the same filtered rows into community sums.
func (s *Store) CommunityTotals(ctx context.Context, f Filters, pricing *PricingTable) CommunityTotals {
	leaders := s.Leaderboard(ctx, f, pricing)
	var t CommunityTotals
	for _, l := range leaders {
		t.Tokens += l.Tokens
		t.Cost += l.Cost
		t.Users++
		t.Devices += l.Devices
	}
	return t
}

// DashboardData is everything /me renders.
type DashboardData struct {
	Today        string
	HourlyToday  []HourToolPoint // 当日 hour×tool 时间线
	Daily30      []DayPoint      // 近 30 天用量与成本
	ByTool       []ModelSlice
	ByModel      []ModelSlice
	Composition  Composition
	Devices      []DeviceView
	TodayTokens  uint64
	TodayCost    float64
	TotalTokens  uint64
	CacheHitRate float64
	ActiveDays   int
	Streak       int
	FlaggedToday bool
}

// HourToolPoint is one hour's per-tool split.
type HourToolPoint struct {
	Hour   int
	Tokens uint64
	Tools  map[string]uint64
}

// DayPoint is one day's tokens and cost.
type DayPoint struct {
	Date   string
	Tokens uint64
	Cost   float64
}

// Composition is the five-class token split.
type Composition struct {
	Input, Output, CacheRead, Cache5m, Cache1h uint64
}

// DeviceView is a device row with last-sync info.
type DeviceView struct {
	ID       string
	Label    string
	LastSync string
	Flagged  bool
}

// Dashboard builds the personal page data for one user.
func (s *Store) Dashboard(ctx context.Context, user *User, pricing *PricingTable) DashboardData {
	rows, err := s.fetchRows(ctx, Filters{User: user, IncludeFlagged: true})
	if err != nil {
		return DashboardData{}
	}
	today := time.Now().Format("2006-01-02")
	data := DashboardData{Today: today}
	hourly := map[int]map[string]uint64{}
	daily := map[string]*DayPoint{}
	tools := map[string]uint64{}
	models := map[string]uint64{}
	dates := map[string]bool{}
	var firstDate string
	for i := range rows {
		r := &rows[i]
		cell := UsageCell{Tool: r.Tool, Model: r.Model, Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, Cache5m: r.Cache5m, Cache1h: r.Cache1h}
		total := cell.TokensIn(true)
		cost := pricing.CostForHourRow(&HourRow{Hour: r.Hour, Tool: r.Tool, Model: r.Model, Input: r.Input, Output: r.Output, CacheRead: r.CacheRead, Cache5m: r.Cache5m, Cache1h: r.Cache1h})
		data.TotalTokens += total
		data.Composition.Input += r.Input
		data.Composition.Output += r.Output
		data.Composition.CacheRead += r.CacheRead
		data.Composition.Cache5m += r.Cache5m
		data.Composition.Cache1h += r.Cache1h
		tools[r.Tool] += total
		models[r.Model] += total
		dates[r.Date] = true
		if r.Date == today {
			data.TodayTokens += total
			data.TodayCost += cost
			if r.Flagged {
				data.FlaggedToday = true
			}
			if hourly[r.Hour] == nil {
				hourly[r.Hour] = map[string]uint64{}
			}
			hourly[r.Hour][r.Tool] += total
		}
		day, ok := daily[r.Date]
		if !ok {
			day = &DayPoint{Date: r.Date}
			daily[r.Date] = day
			if firstDate == "" || r.Date < firstDate {
				firstDate = r.Date
			}
		}
		day.Tokens += total
		day.Cost += cost
	}
	for hour := 0; hour < 24; hour++ {
		point := HourToolPoint{Hour: hour, Tools: map[string]uint64{}}
		for tool, tokens := range hourly[hour] {
			point.Tokens += tokens
			point.Tools[tool] = tokens
		}
		data.HourlyToday = append(data.HourlyToday, point)
	}
	// Fixed 30-day window ending today, empty days rendered as zero.
	start := time.Now().AddDate(0, 0, -29).Format("2006-01-02")
	for d, err := time.Parse("2006-01-02", start); err == nil && !d.After(time.Now()); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		point := DayPoint{Date: date}
		if p, ok := daily[date]; ok {
			point = *p
		}
		data.Daily30 = append(data.Daily30, point)
	}
	data.ByTool = sortedSlices(tools)
	data.ByModel = sortedSlices(models)
	data.ActiveDays = len(dates)
	data.Streak = dayStreak(daily, today)
	var readPlusWrite uint64
	readPlusWrite = data.Composition.CacheRead + data.Composition.Cache5m + data.Composition.Cache1h
	if readPlusWrite > 0 {
		data.CacheHitRate = float64(data.Composition.CacheRead) / float64(readPlusWrite)
	}
	devices, _ := s.UserDevices(ctx, user.ID)
	for _, d := range devices {
		last := s.lastReportAt(ctx, d.ID)
		sync := "从未"
		if last > 0 {
			sync = time.Unix(last, 0).Format("01-02 15:04")
		}
		data.Devices = append(data.Devices, DeviceView{ID: d.ID, Label: d.Label, LastSync: sync})
	}
	return data
}

func sortedSlices(m map[string]uint64) []ModelSlice {
	out := make([]ModelSlice, 0, len(m))
	for name, tokens := range m {
		out = append(out, ModelSlice{Model: name, Tokens: tokens})
	}
	sortModelSlices(out)
	return out
}

// dayStreak counts consecutive days with data ending at today (or yesterday,
// so the streak survives until the day's first report).
func dayStreak(daily map[string]*DayPoint, today string) int {
	layout := "2006-01-02"
	cursor, err := time.Parse(layout, today)
	if err != nil {
		return 0
	}
	if _, ok := daily[today]; !ok {
		cursor = cursor.AddDate(0, 0, -1)
	}
	streak := 0
	for {
		if p, ok := daily[cursor.Format(layout)]; ok && p.Tokens > 0 {
			streak++
			cursor = cursor.AddDate(0, 0, -1)
			continue
		}
		return streak
	}
}
