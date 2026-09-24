package server

import (
	"context"
	"sort"
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

// cell returns the row's Usage Cell: the five counters keyed by tool+model.
func (r *rawRow) cell() UsageCell {
	return UsageCell{Tool: r.Tool, Model: r.Model, Input: r.Input, Output: r.Output,
		CacheRead: r.CacheRead, Cache5m: r.Cache5m, Cache1h: r.Cache1h}
}

// hourRow returns the row's Hourly Usage cell (the counters plus the hour).
func (r *rawRow) hourRow() HourRow {
	c := r.cell()
	return HourRow{Hour: r.Hour, Tool: c.Tool, Model: c.Model, Input: c.Input,
		Output: c.Output, CacheRead: c.CacheRead, Cache5m: c.Cache5m, Cache1h: c.Cache1h}
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
		cell := r.cell()
		hr := r.hourRow()
		a.tokens += cell.TokensIn(f.IncludeCache)
		a.cost += pricing.CostForHourRow(&hr)
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
	Daily30      []DayPoint      // 近 30 天用量、成本与命中率
	ByTool       []NameStat
	ByModel      []NameStat
	ToolModels   []ToolModelRow // 工具×模型交叉明细(全历史,token 降序)
	Heatmap      []HeatCell     // 星期×小时工作节律(全历史,7×24 全格)
	Composition  Composition    // 全历史五类构成(饼图)
	Devices      []DeviceView
	TodayTokens  uint64
	TodayCost    float64
	TotalTokens  uint64
	CacheHitRate float64    // 当日输入命中率:读/(读+写+未缓存输入);HasRate 为真时有效
	CacheSplit   CacheSplit // 当日输入侧三比例分解,和为 1
	HasRate      bool
	CacheSavings float64 // 当日缓存净节省(USD)
	ReadPerWrite float64 // 当日回本次数:读÷写;零写时为 0
	ActiveDays   int
	Streak       int
	FlaggedToday bool
}

// CacheSplit is the input-side three-way split: cache read / cache write /
// fresh (uncached) input. Proportions sum to 1 whenever any input flowed.
type CacheSplit struct {
	Read, Write, Fresh float64
}

// NameStat is one tool's or model's aggregate with cost and hit rate.
type NameStat struct {
	Name    string
	Tokens  uint64
	Cost    float64
	HitRate float64
	HasRate bool
}

// nameAgg accumulates one dimension's counters behind a NameStat/ToolModelRow.
type nameAgg struct {
	tokens, read, write, fresh uint64
	cost                       float64
}

// ToolModelRow is one (tool, model) cross of the breakdown table.
type ToolModelRow struct {
	Tool    string
	Model   string
	Tokens  uint64
	Cost    float64
	HitRate float64
	HasRate bool
}

// HeatCell is one weekday-hour aggregate (Weekday 0 = Monday).
type HeatCell struct {
	Weekday int
	Hour    int
	Tokens  uint64
}

// HourToolPoint is one hour's per-tool split.
type HourToolPoint struct {
	Hour   int
	Tokens uint64
	Tools  map[string]uint64
}

// DayPoint is one day's tokens, cost and cache hit rate.
type DayPoint struct {
	Date    string
	Tokens  uint64
	Cost    float64
	HitRate float64
	HasRate bool
}

// cacheHitRate is the B-definition rate: cache reads over the whole input
// side (reads + writes + fresh input). Also reports whether the denominator
// existed at all (a pure-output day has no rate).
func cacheHitRate(read, write, fresh uint64) (float64, bool) {
	total := read + write + fresh
	if total == 0 {
		return 0, false
	}
	return float64(read) / float64(total), true
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

// dayAgg is one date's DayPoint plus the input-side counters behind its rate.
type dayAgg struct {
	point              DayPoint
	read, write, fresh uint64
}

// accName returns m[key], seeding a zero aggregate on first touch.
func accName[K comparable](m map[K]*nameAgg, key K) *nameAgg {
	a, ok := m[key]
	if !ok {
		a = &nameAgg{}
		m[key] = a
	}
	return a
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
	daily := map[string]*dayAgg{}
	acc := accName[string]
	tools := map[string]*nameAgg{}
	models := map[string]*nameAgg{}
	type tmKey struct{ tool, model string }
	tms := map[tmKey]*nameAgg{}
	tmOrder := []tmKey{}
	var heat [7][24]uint64
	dates := map[string]bool{}
	var firstDate string
	var todayRead, todayWrite, todayFresh uint64
	for i := range rows {
		r := &rows[i]
		cell := r.cell()
		total := cell.TokensIn(true)
		hr := r.hourRow()
		cost := pricing.CostForHourRow(&hr)
		data.TotalTokens += total
		data.Composition.Input += r.Input
		data.Composition.Output += r.Output
		data.Composition.CacheRead += r.CacheRead
		data.Composition.Cache5m += r.Cache5m
		data.Composition.Cache1h += r.Cache1h
		tmk := tmKey{r.Tool, r.Model}
		if _, seen := tms[tmk]; !seen {
			tmOrder = append(tmOrder, tmk)
		}
		for _, a := range []*nameAgg{acc(tools, r.Tool), acc(models, r.Model), accName(tms, tmk)} {
			a.tokens += total
			a.cost += cost
			a.read += r.CacheRead
			a.write += r.Cache5m + r.Cache1h
			a.fresh += r.Input
		}
		if t, perr := time.Parse("2006-01-02", r.Date); perr == nil {
			heat[int(t.Weekday()+6)%7][r.Hour] += total
		}
		dates[r.Date] = true
		day, ok := daily[r.Date]
		if !ok {
			day = &dayAgg{point: DayPoint{Date: r.Date}}
			daily[r.Date] = day
			if firstDate == "" || r.Date < firstDate {
				firstDate = r.Date
			}
		}
		day.point.Tokens += total
		day.point.Cost += cost
		day.read += r.CacheRead
		day.write += r.Cache5m + r.Cache1h
		day.fresh += r.Input
		if r.Date == today {
			data.TodayTokens += total
			data.TodayCost += cost
			data.CacheSavings += pricing.CacheSavingsForHourRow(&hr)
			todayRead += r.CacheRead
			todayWrite += r.Cache5m + r.Cache1h
			todayFresh += r.Input
			if r.Flagged {
				data.FlaggedToday = true
			}
			if hourly[r.Hour] == nil {
				hourly[r.Hour] = map[string]uint64{}
			}
			hourly[r.Hour][r.Tool] += total
		}
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
		if agg, ok := daily[date]; ok {
			point = agg.point
			point.HitRate, point.HasRate = cacheHitRate(agg.read, agg.write, agg.fresh)
		}
		data.Daily30 = append(data.Daily30, point)
	}
	data.ByTool = sortedNameStats(tools)
	data.ByModel = sortedNameStats(models)
	for _, key := range tmOrder {
		a := tms[key]
		rate, has := cacheHitRate(a.read, a.write, a.fresh)
		data.ToolModels = append(data.ToolModels, ToolModelRow{Tool: key.tool, Model: key.model, Tokens: a.tokens, Cost: a.cost, HitRate: rate, HasRate: has})
	}
	sort.SliceStable(data.ToolModels, func(i, j int) bool {
		if data.ToolModels[i].Tokens != data.ToolModels[j].Tokens {
			return data.ToolModels[i].Tokens > data.ToolModels[j].Tokens
		}
		if data.ToolModels[i].Tool != data.ToolModels[j].Tool {
			return data.ToolModels[i].Tool < data.ToolModels[j].Tool
		}
		return data.ToolModels[i].Model < data.ToolModels[j].Model
	})
	if len(data.ToolModels) > 50 {
		data.ToolModels = data.ToolModels[:50]
	}
	for wd := 0; wd < 7; wd++ {
		for hour := 0; hour < 24; hour++ {
			data.Heatmap = append(data.Heatmap, HeatCell{Weekday: wd, Hour: hour, Tokens: heat[wd][hour]})
		}
	}
	data.ActiveDays = len(dates)
	data.Streak = dayStreak(daily, today)
	data.CacheHitRate, data.HasRate = cacheHitRate(todayRead, todayWrite, todayFresh)
	if total := todayRead + todayWrite + todayFresh; total > 0 {
		data.CacheSplit = CacheSplit{
			Read:  float64(todayRead) / float64(total),
			Write: float64(todayWrite) / float64(total),
			Fresh: float64(todayFresh) / float64(total),
		}
	}
	if todayWrite > 0 {
		data.ReadPerWrite = float64(todayRead) / float64(todayWrite)
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

func sortedNameStats(m map[string]*nameAgg) []NameStat {
	out := make([]NameStat, 0, len(m))
	for name, a := range m {
		rate, has := cacheHitRate(a.read, a.write, a.fresh)
		out = append(out, NameStat{Name: name, Tokens: a.tokens, Cost: a.cost, HitRate: rate, HasRate: has})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tokens > out[j].Tokens })
	return out
}

// dayStreak counts consecutive days with data ending at today (or yesterday,
// so the streak survives until the day's first report).
func dayStreak(daily map[string]*dayAgg, today string) int {
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
		if p, ok := daily[cursor.Format(layout)]; ok && p.point.Tokens > 0 {
			streak++
			cursor = cursor.AddDate(0, 0, -1)
			continue
		}
		return streak
	}
}
