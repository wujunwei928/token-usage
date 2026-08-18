// Package server implements the token-leaderboard server: report ingest,
// storage, aggregation, pricing, and the SSR web pages.
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database holding the leaderboard data.
type Store struct {
	db *sql.DB
}

// openDialer keeps tests and main on the same settings.
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// modernc sqlite is happiest without true concurrency; one writer
	// connection serializes the rare write path while reads stay parallel.
	db.SetMaxOpenConns(4)
	return db, nil
}

// OpenStore opens (creating if needed) the database at path and applies
// migrations.
func OpenStore(path string) (*Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL DEFAULT '',
			city TEXT NOT NULL DEFAULT '',
			avatar TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tokens (
			id INTEGER PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			user_id INTEGER NOT NULL REFERENCES users(id),
			label TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			last_used_at INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			device_id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id),
			label TEXT NOT NULL DEFAULT '',
			first_seen INTEGER NOT NULL,
			last_seen INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id)`,
		`CREATE TABLE IF NOT EXISTS reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			date TEXT NOT NULL,
			reported_at INTEGER NOT NULL,
			entry_count INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reports_device ON reports(device_id, date)`,
		`CREATE TABLE IF NOT EXISTS hourly_usage (
			device_id TEXT NOT NULL,
			date TEXT NOT NULL,
			hour INTEGER NOT NULL,
			tool TEXT NOT NULL,
			model TEXT NOT NULL,
			input INTEGER NOT NULL,
			output INTEGER NOT NULL,
			cache_read INTEGER NOT NULL,
			cache_5m INTEGER NOT NULL,
			cache_1h INTEGER NOT NULL,
			flagged INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (device_id, date, hour, tool, model)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hourly_date ON hourly_usage(date)`,
		`CREATE INDEX IF NOT EXISTS idx_hourly_tool_model ON hourly_usage(tool, model)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Users and tokens
// ---------------------------------------------------------------------------

// User is one leaderboard account.
type User struct {
	ID           int64
	Name         string
	PasswordHash string
	City         string
	Avatar       string
}

// CreateUser inserts a user; passwordHash may be empty for token-only users.
func (s *Store) CreateUser(ctx context.Context, name, passwordHash, city, avatar string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (name, password_hash, city, avatar, created_at) VALUES (?, ?, ?, ?, ?)`,
		name, passwordHash, city, avatar, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateUserPassword sets the password hash for a user id.
func (s *Store) UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}

// UpdateUserProfile sets the city and avatar for a user id.
func (s *Store) UpdateUserProfile(ctx context.Context, userID int64, city, avatar string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET city = ?, avatar = ? WHERE id = ?`, city, avatar, userID)
	return err
}

// UserByID loads one user.
func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, password_hash, city, avatar FROM users WHERE id = ?`, id)
	var u User
	if err := row.Scan(&u.ID, &u.Name, &u.PasswordHash, &u.City, &u.Avatar); err != nil {
		return nil, err
	}
	return &u, nil
}

// UserByName loads one user by (case-insensitive) name.
func (s *Store) UserByName(ctx context.Context, name string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, password_hash, city, avatar FROM users WHERE name = ? COLLATE NOCASE`, name)
	var u User
	if err := row.Scan(&u.ID, &u.Name, &u.PasswordHash, &u.City, &u.Avatar); err != nil {
		return nil, err
	}
	return &u, nil
}

// GenerateToken mints a new random plaintext token for a user.
func GenerateToken() string {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return "cct_" + hex.EncodeToString(raw)
}

// HashToken derives the stored form of a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateToken stores the hash of a token for a user.
func (s *Store) CreateToken(ctx context.Context, userID int64, label, tokenHash string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tokens (token_hash, user_id, label, created_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, label, time.Now().Unix())
	return err
}

// UserForToken resolves a plaintext token to its user, refreshing last_used.
func (s *Store) UserForToken(ctx context.Context, token string) (*User, error) {
	hash := HashToken(token)
	row := s.db.QueryRowContext(ctx, `SELECT user_id FROM tokens WHERE token_hash = ?`, hash)
	var userID int64
	if err := row.Scan(&userID); err != nil {
		return nil, err
	}
	s.db.ExecContext(ctx, `UPDATE tokens SET last_used_at = ? WHERE token_hash = ?`, time.Now().Unix(), hash)
	return s.UserByID(ctx, userID)
}

// RevokeToken deletes a token id owned by the given user.
func (s *Store) RevokeToken(ctx context.Context, userID, tokenID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tokens WHERE id = ? AND user_id = ?`, tokenID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("token not found")
	}
	return nil
}

// UserToken lists a user's tokens (hashes truncated for display).
type UserToken struct {
	ID         int64
	Label      string
	CreatedAt  int64
	LastUsedAt int64
}

// ListTokens returns a user's tokens, newest first.
func (s *Store) ListTokens(ctx context.Context, userID int64) ([]UserToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, label, created_at, COALESCE(last_used_at, 0) FROM tokens WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserToken
	for rows.Next() {
		var t UserToken
		if err := rows.Scan(&t.ID, &t.Label, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

// ErrTooManyDevices rejects a fourth Device for a user.
var ErrTooManyDevices = errors.New("device limit reached (max 3)")

// MaxDevicesPerUser is the leaderboard cap from the house rules.
const MaxDevicesPerUser = 3

// Device is one reporting client machine.
type Device struct {
	ID        string
	UserID    int64
	Label     string
	FirstSeen int64
	LastSeen  int64
}

// BindDevice finds or binds a device for the user, enforcing the cap. The
// caller must hold the write transaction that inserts usage.
func (s *Store) BindDevice(ctx context.Context, tx *sql.Tx, userID int64, deviceID, label string, now int64) error {
	row := tx.QueryRowContext(ctx, `SELECT user_id FROM devices WHERE device_id = ?`, deviceID)
	var owner int64
	err := row.Scan(&owner)
	if err == nil {
		if owner != userID {
			return fmt.Errorf("device %s is bound to another user", deviceID)
		}
		_, err = tx.ExecContext(ctx, `UPDATE devices SET last_seen = ?, label = ? WHERE device_id = ?`, now, label, deviceID)
		return err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return err
	}
	if count >= MaxDevicesPerUser {
		return ErrTooManyDevices
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO devices (device_id, user_id, label, first_seen, last_seen) VALUES (?, ?, ?, ?, ?)`,
		deviceID, userID, label, now, now)
	return err
}

// UserDevices lists a user's devices by first_seen.
func (s *Store) UserDevices(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_id, user_id, label, first_seen, last_seen FROM devices WHERE user_id = ? ORDER BY first_seen`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Label, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CountDevices returns the number of devices a user has bound.
func (s *Store) CountDevices(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE user_id = ?`, userID).Scan(&count)
	return count, err
}

// DeviceOwner maps device ids to user ids for a set of devices.
func (s *Store) DeviceOwner(ctx context.Context, deviceID string) (int64, error) {
	var userID int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM devices WHERE device_id = ?`, deviceID).Scan(&userID)
	return userID, err
}

// ---------------------------------------------------------------------------
// Report ingest (latest-wins)
// ---------------------------------------------------------------------------

// HourRow is one Hourly Usage cell from a Report Snapshot.
type HourRow struct {
	Hour      int    `json:"hour"`
	Tool      string `json:"tool"`
	Model     string `json:"model"`
	Input     uint64 `json:"input"`
	Output    uint64 `json:"output"`
	CacheRead uint64 `json:"cacheRead"`
	Cache5m   uint64 `json:"cacheWrite5m"`
	Cache1h   uint64 `json:"cacheWrite1h"`
}

// TotalTokens sums the five counters.
func (h HourRow) TotalTokens() uint64 {
	return h.Input + h.Output + h.CacheRead + h.Cache5m + h.Cache1h
}

// ReplaceDay installs a device's Report Snapshot for one date: existing rows
// for (device, date) are deleted, then the new cells inserted (Latest-wins).
func (s *Store) ReplaceDay(ctx context.Context, userID int64, deviceID, deviceLabel, date string, rows []HourRow) (dayTokens uint64, err error) {
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if err = s.BindDevice(ctx, tx, userID, deviceID, deviceLabel, now); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM hourly_usage WHERE device_id = ? AND date = ?`, deviceID, date); err != nil {
		return 0, err
	}
	for i := range rows {
		r := &rows[i]
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO hourly_usage (device_id, date, hour, tool, model, input, output, cache_read, cache_5m, cache_1h, flagged)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			deviceID, date, r.Hour, r.Tool, r.Model, r.Input, r.Output, r.CacheRead, r.Cache5m, r.Cache1h); err != nil {
			return 0, err
		}
		dayTokens += r.TotalTokens()
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO reports (device_id, date, reported_at, entry_count, total_tokens) VALUES (?, ?, ?, ?, ?)`,
		deviceID, date, now, len(rows), dayTokens); err != nil {
		return 0, err
	}
	// Anomaly Flag: a device-day over the threshold is hidden from the
	// leaderboard but stays visible to its owner (spec T08).
	if dayTokens > AnomalyThresholdTokens {
		if _, err = tx.ExecContext(ctx,
			`UPDATE hourly_usage SET flagged = 1 WHERE device_id = ? AND date = ?`, deviceID, date); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return dayTokens, nil
}

// AnomalyThresholdTokens is the per-device-per-day token count past which the
// day's data is flagged off the leaderboard.
const AnomalyThresholdTokens = 1_000_000_000

// HasUsageBefore reports whether a user owns any Hourly Usage row dated
// strictly before date — the web command's first-run backfill trigger.
func (s *Store) HasUsageBefore(ctx context.Context, userID int64, date string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM hourly_usage h
			JOIN devices d ON d.device_id = h.device_id
			WHERE d.user_id = ? AND h.date < ?)`, userID, date).Scan(&exists)
	return exists, err
}

// DumpHourly renders every Hourly Usage row as one canonical comparison line
// (device|date|hour|tool|model|counters|flagged), ordered. It is the
// observable used to verify that the web command's in-process ingest and the
// HTTP ingest stay row-for-row identical.
func (s *Store) DumpHourly(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_id, date, hour, tool, model, input, output, cache_read, cache_5m, cache_1h, flagged
		 FROM hourly_usage ORDER BY device_id, date, hour, tool, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var deviceID, date, tool, model string
		var hour, input, output, cacheRead, cache5m, cache1h, flagged int64
		if err := rows.Scan(&deviceID, &date, &hour, &tool, &model, &input, &output, &cacheRead, &cache5m, &cache1h, &flagged); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%s|%s|%d|%s|%s|%d|%d|%d|%d|%d|%d",
			deviceID, date, hour, tool, model, input, output, cacheRead, cache5m, cache1h, flagged))
	}
	return out, rows.Err()
}

// lastReportAt returns the most recent reported_at for a device (any date).
func (s *Store) lastReportAt(ctx context.Context, deviceID string) int64 {
	var ts int64
	s.db.QueryRowContext(ctx, `SELECT MAX(reported_at) FROM reports WHERE device_id = ?`, deviceID).Scan(&ts)
	return ts
}

// deviceLabel returns the stored label for a device id.
func (s *Store) deviceLabel(ctx context.Context, deviceID string) string {
	var label string
	s.db.QueryRowContext(ctx, `SELECT label FROM devices WHERE device_id = ?`, deviceID).Scan(&label)
	return label
}
