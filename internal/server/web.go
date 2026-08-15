package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Web serves the SSR pages: leaderboard, dashboard, pricing, about, and the
// lightweight account flow (register/login/settings with User Token minting).
type Web struct {
	store    *Store
	pricing  *PricingTable
	sessions *sessionStore
	tpl      *template.Template
}

// sessionStore is the in-memory cookie-session table (restart logs everyone
// out; acceptable for the v1 community server).
type sessionStore struct {
	mu   sync.Mutex
	sess map[string]int64
}

func newSessionStore() *sessionStore {
	return &sessionStore{sess: map[string]int64{}}
}

func (s *sessionStore) create(userID int64) string {
	raw := make([]byte, 24)
	rand.Read(raw)
	id := hex.EncodeToString(raw)
	s.mu.Lock()
	s.sess[id] = userID
	s.mu.Unlock()
	return id
}

func (s *sessionStore) user(id string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid, ok := s.sess[id]
	return uid, ok
}

func (s *sessionStore) drop(id string) {
	s.mu.Lock()
	delete(s.sess, id)
	s.mu.Unlock()
}

const sessionCookie = "ccsid"

// NewWeb builds the page server.
func NewWeb(store *Store, pricing *PricingTable) (*Web, error) {
	tpl, err := template.New("").Funcs(template.FuncMap{
		"fmtTokens": FormatTokens,
		"fmtCost":   FormatCost,
		"initial":   func(name string) string { return strings.ToUpper(name[:1]) },
		"mul100":    func(v float64) float64 { return v * 100 },
		"ts":        func(v int64) string { return time.Unix(v, 0).Format("2006-01-02 15:04") },
		"json":      func(v any) template.JS { raw, _ := json.Marshal(v); return template.JS(raw) },
	}).ParseFS(templateFS, "web/templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Web{store: store, pricing: pricing, sessions: newSessionStore(), tpl: tpl}, nil
}

// Register mounts the web routes.
func (w *Web) Register(mux *http.ServeMux) {
	staticRoot, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticRoot))))
	mux.HandleFunc("GET /{$}", w.handleLeaderboard)
	mux.HandleFunc("GET /me", w.requireUser(w.handleDashboard))
	mux.HandleFunc("GET /pricing", w.handlePricing)
	mux.HandleFunc("GET /about", w.handleAbout)
	mux.HandleFunc("GET /login", w.handleLoginForm)
	mux.HandleFunc("POST /login", w.handleLogin)
	mux.HandleFunc("GET /register", w.handleRegisterForm)
	mux.HandleFunc("POST /register", w.handleRegister)
	mux.HandleFunc("POST /logout", w.handleLogout)
	mux.HandleFunc("GET /settings", w.requireUser(w.handleSettings))
	mux.HandleFunc("POST /settings/tokens", w.requireUser(w.handleTokenCreate))
	mux.HandleFunc("POST /settings/tokens/revoke", w.requireUser(w.handleTokenRevoke))
	mux.HandleFunc("POST /settings/profile", w.requireUser(w.handleProfile))
}

// currentUser resolves the session cookie to a user, if any.
func (w *Web) currentUser(r *http.Request) *User {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	uid, ok := w.sessions.user(cookie.Value)
	if !ok {
		return nil
	}
	user, err := w.store.UserByID(r.Context(), uid)
	if err != nil {
		return nil
	}
	return user
}

// requireUser wraps a handler that needs a logged-in user.
func (w *Web) requireUser(next func(http.ResponseWriter, *http.Request, *User)) http.HandlerFunc {
	return func(resp http.ResponseWriter, r *http.Request) {
		user := w.currentUser(r)
		if user == nil {
			http.Redirect(resp, r, "/login", http.StatusSeeOther)
			return
		}
		next(resp, r, user)
	}
}

type pageData struct {
	Title   string
	User    *User
	Content any
	Flash   string
}

func (w *Web) render(resp http.ResponseWriter, status int, name string, data pageData) {
	resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	resp.WriteHeader(status)
	if err := w.tpl.ExecuteTemplate(resp, name, data); err != nil {
		fmt.Printf("template %s: %v\n", name, err)
	}
}

// RangeNames is the leaderboard time-range vocabulary.
var RangeNames = []string{"today", "yesterday", "daybefore", "3d", "7d", "30d", "all"}

// rangeLabels maps range keys to Chinese labels.
var rangeLabels = map[string]string{
	"today": "今天", "yesterday": "昨天", "daybefore": "前天",
	"3d": "近 3 天", "7d": "近 7 天", "30d": "近 30 天", "all": "全部",
}

// rangeBounds resolves a range key to inclusive date bounds (server local).
func rangeBounds(key string) (from, to string) {
	today := time.Now().Format("2006-01-02")
	day := func(offset int) string { return time.Now().AddDate(0, 0, offset).Format("2006-01-02") }
	switch key {
	case "yesterday":
		return day(-1), day(-1)
	case "daybefore":
		return day(-2), day(-2)
	case "3d":
		return day(-2), today
	case "7d":
		return day(-6), today
	case "30d":
		return day(-29), today
	case "all":
		return "", ""
	default: // today
		return today, today
	}
}

// leaderboardView carries one page render's data.
type leaderboardView struct {
	Range        string
	RangeLabel   string
	Ranges       []rangeOption
	Tools        []string
	Models       []string
	Cities       []string
	Tool         string
	Model        string
	City         string
	IncludeCache bool
	Rows         []LeaderRow
	Totals       CommunityTotals
}

type rangeOption struct {
	Key    string
	Label  string
	Active bool
}

// filterOptions enumerates distinct tools/models/cities present in stored data.
func (w *Web) filterOptions(ctx context.Context) (tools, models, cities []string) {
	tools = w.store.distinctTools(ctx)
	models = w.store.distinctModels(ctx)
	cities = w.store.distinctCities(ctx)
	return
}

func (w *Web) handleLeaderboard(resp http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	rng := q.Get("range")
	if rng == "" {
		rng = "today"
	}
	if _, ok := rangeLabels[rng]; !ok {
		rng = "today"
	}
	from, to := rangeBounds(rng)
	includeCache := q.Get("cache") != "0"
	filters := Filters{
		From: from, To: to,
		Tool:         q.Get("tool"),
		Model:        q.Get("model"),
		City:         q.Get("city"),
		IncludeCache: includeCache,
	}
	tools, models, cities := w.filterOptions(ctx)
	view := leaderboardView{
		Range:        rng,
		RangeLabel:   rangeLabels[rng],
		Tool:         filters.Tool,
		Model:        filters.Model,
		City:         filters.City,
		IncludeCache: includeCache,
		Rows:         w.store.Leaderboard(ctx, filters, w.pricing),
		Tools:        tools,
		Models:       models,
		Cities:       cities,
	}
	for _, key := range RangeNames {
		view.Ranges = append(view.Ranges, rangeOption{Key: key, Label: rangeLabels[key], Active: key == rng})
	}
	view.Totals = w.store.CommunityTotals(ctx, filters, w.pricing)
	w.render(resp, http.StatusOK, "index.html", pageData{Title: "Token 排行榜", User: w.currentUser(r), Content: view})
}

func (w *Web) handleDashboard(resp http.ResponseWriter, r *http.Request, user *User) {
	data := w.store.Dashboard(r.Context(), user, w.pricing)
	w.render(resp, http.StatusOK, "me.html", pageData{Title: "我的 Token", User: user, Content: data})
}

func (w *Web) handlePricing(resp http.ResponseWriter, r *http.Request) {
	type row struct {
		ModelPrice
		Note string
	}
	var rows []row
	for _, card := range w.pricing.Roster() {
		note := ""
		switch card.Source {
		case SourceOverride:
			note = "override"
		case SourceFamily:
			note = "estimated"
		default:
			note = "official"
		}
		rows = append(rows, row{ModelPrice: card, Note: note})
	}
	w.render(resp, http.StatusOK, "pricing.html", pageData{
		Title: "模型价格表", User: w.currentUser(r), Content: rows,
	})
}

func (w *Web) handleAbout(resp http.ResponseWriter, r *http.Request) {
	w.render(resp, http.StatusOK, "about.html", pageData{Title: "数据说明", User: w.currentUser(r)})
}

// ---------------------------------------------------------------------------
// Accounts
// ---------------------------------------------------------------------------

func (w *Web) setSession(resp http.ResponseWriter, userID int64) {
	http.SetCookie(resp, &http.Cookie{
		Name: sessionCookie, Value: w.sessions.create(userID),
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
	})
}

func (w *Web) handleLoginForm(resp http.ResponseWriter, r *http.Request) {
	w.render(resp, http.StatusOK, "login.html", pageData{Title: "登录"})
}

func (w *Web) handleLogin(resp http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PostFormValue("name"))
	password := r.PostFormValue("password")
	user, err := w.store.UserByName(r.Context(), name)
	if err != nil || user.PasswordHash == "" || !VerifyPassword(password, user.PasswordHash) {
		w.render(resp, http.StatusUnauthorized, "login.html", pageData{Title: "登录", Flash: "用户名或密码错误"})
		return
	}
	w.setSession(resp, user.ID)
	http.Redirect(resp, r, "/me", http.StatusSeeOther)
}

func (w *Web) handleRegisterForm(resp http.ResponseWriter, r *http.Request) {
	w.render(resp, http.StatusOK, "register.html", pageData{Title: "注册"})
}

func (w *Web) handleRegister(resp http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := strings.TrimSpace(r.PostFormValue("name"))
	password := r.PostFormValue("password")
	city := strings.TrimSpace(r.PostFormValue("city"))
	if len(name) < 1 || len(name) > 32 {
		w.render(resp, http.StatusBadRequest, "register.html", pageData{Title: "注册", Flash: "昵称需 1-32 个字符"})
		return
	}
	if len(password) < 6 {
		w.render(resp, http.StatusBadRequest, "register.html", pageData{Title: "注册", Flash: "密码至少 6 位"})
		return
	}
	if len(city) > 32 {
		w.render(resp, http.StatusBadRequest, "register.html", pageData{Title: "注册", Flash: "城市过长"})
		return
	}
	hash, err := HashPassword(password)
	if err == nil {
		_, err = w.store.CreateUser(ctx, name, hash, city, "")
	}
	if err != nil {
		w.render(resp, http.StatusConflict, "register.html", pageData{Title: "注册", Flash: "昵称已被占用"})
		return
	}
	user, _ := w.store.UserByName(ctx, name)
	w.setSession(resp, user.ID)
	http.Redirect(resp, r, "/settings", http.StatusSeeOther)
}

func (w *Web) handleLogout(resp http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		w.sessions.drop(cookie.Value)
	}
	http.SetCookie(resp, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(resp, r, "/", http.StatusSeeOther)
}

// settingsView carries the settings page data.
type settingsView struct {
	ProfileCity string
	Tokens      []UserToken
	NewToken    string
	DeviceCount int
}

func (w *Web) handleSettings(resp http.ResponseWriter, r *http.Request, user *User) {
	view := w.buildSettings(r.Context(), user, "")
	w.render(resp, http.StatusOK, "settings.html", pageData{Title: "设置", User: user, Content: view})
}

func (w *Web) buildSettings(ctx context.Context, user *User, newToken string) settingsView {
	tokens, _ := w.store.ListTokens(ctx, user.ID)
	count, _ := w.store.CountDevices(ctx, user.ID)
	return settingsView{ProfileCity: user.City, Tokens: tokens, NewToken: newToken, DeviceCount: count}
}

func (w *Web) handleTokenCreate(resp http.ResponseWriter, r *http.Request, user *User) {
	label := strings.TrimSpace(r.PostFormValue("label"))
	if label == "" {
		label = "cli"
	}
	token := GenerateToken()
	if err := w.store.CreateToken(r.Context(), user.ID, label, HashToken(token)); err != nil {
		http.Error(resp, "failed to create token", http.StatusInternalServerError)
		return
	}
	view := w.buildSettings(r.Context(), user, token)
	w.render(resp, http.StatusOK, "settings.html", pageData{Title: "设置", User: user, Content: view, Flash: "token 已生成,仅本次显示,请立即复制"})
}

func (w *Web) handleTokenRevoke(resp http.ResponseWriter, r *http.Request, user *User) {
	id, _ := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	if id > 0 {
		w.store.RevokeToken(r.Context(), user.ID, id)
	}
	http.Redirect(resp, r, "/settings", http.StatusSeeOther)
}

func (w *Web) handleProfile(resp http.ResponseWriter, r *http.Request, user *User) {
	city := strings.TrimSpace(r.PostFormValue("city"))
	if len(city) > 32 {
		city = user.City
	}
	if err := w.store.UpdateUserProfile(r.Context(), user.ID, city, ""); err != nil {
		http.Error(resp, "failed to save profile", http.StatusInternalServerError)
		return
	}
	http.Redirect(resp, r, "/settings", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Formatting helpers shared by templates
// ---------------------------------------------------------------------------

// FormatTokens renders a token count with compact units.
func FormatTokens(v uint64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(v)/1e9)
	case v >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(v)/1e6)
	case v >= 1_000:
		return fmt.Sprintf("%.1fK", float64(v)/1e3)
	default:
		return strconv.FormatUint(v, 10)
	}
}

// FormatCost renders a USD cost.
func FormatCost(v float64) string {
	switch {
	case v >= 100:
		return fmt.Sprintf("$%.0f", v)
	case v >= 1:
		return fmt.Sprintf("$%.2f", v)
	case v >= 0.01:
		return fmt.Sprintf("$%.4f", v)
	default:
		return fmt.Sprintf("$%.6f", v)
	}
}

// distinctTools lists tools present in stored usage.
func (s *Store) distinctTools(ctx context.Context) []string {
	return s.distinct(ctx, `SELECT DISTINCT tool FROM hourly_usage ORDER BY tool`)
}

// distinctModels lists models present in stored usage.
func (s *Store) distinctModels(ctx context.Context) []string {
	return s.distinct(ctx, `SELECT DISTINCT model FROM hourly_usage ORDER BY model`)
}

// distinctCities lists user cities present in stored usage.
func (s *Store) distinctCities(ctx context.Context) []string {
	return s.distinct(ctx, `SELECT DISTINCT u.city FROM hourly_usage h
		JOIN devices d ON d.device_id = h.device_id
		JOIN users u ON u.id = d.user_id
		WHERE u.city != '' ORDER BY u.city`)
}

func (s *Store) distinct(ctx context.Context, query string) []string {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err == nil {
			out = append(out, v)
		}
	}
	return out
}
