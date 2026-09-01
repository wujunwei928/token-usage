package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/report"
	"github.com/wujunwei928/token-usage/internal/server"
)

// mustDevicePath resolves the device identity file under the isolated HOME.
func mustDevicePath(t *testing.T) string {
	t.Helper()
	path, err := report.DefaultDevicePath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// seedHistory lands one claude row for date, so a store can simulate a
// non-first run.
func seedHistory(t *testing.T, store *server.Store, userID int64, device, date string, input uint64) {
	t.Helper()
	_, err := store.ReplaceDay(context.Background(), userID, device, "box", date,
		[]server.HourRow{{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: input}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFirstRunBackfillFillsHistory(t *testing.T) {
	now := time.Now()
	dayAt := func(offset, hour int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location()).AddDate(0, 0, offset)
	}
	sh := time.FixedZone("CST", 8*3600)
	isolateHome(t, t.TempDir())
	t.Setenv("TZ", "Asia/Shanghai")
	t.Setenv("CLAUDE_CONFIG_DIR", webFixture(t,
		webUsageLine("f0", "claude-sonnet-4-5", "rf0", dayAt(0, 9), 300, 0, 0, 0, 0),
		webUsageLine("f1", "claude-sonnet-4-5", "rf1", dayAt(-1, 10), 2000, 100, 0, 0, 0),
		webUsageLine("f2", "claude-sonnet-4-5", "rf2", dayAt(-2, 11), 1000, 100, 0, 0, 0),
	))

	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/bf.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	userID, err := store.CreateUser(ctx, "bf-user", "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	shared := &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}
	days, err := backfillHistory(ctx, store, userID, shared, "", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if days != 3 {
		t.Fatalf("backfilled days = %d, want 3", days)
	}

	// Worked example: one row per date, hand-computed counters. The device
	// id is the persisted identity from the isolated HOME.
	device, err := report.LoadDevice(mustDevicePath(t))
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(t, store)
	want := []string{
		fmt.Sprintf("%s|%s|11|claude|claude-sonnet-4-5|1000|100|0|0|0|0", device.DeviceID, dayAt(-2, 11).In(sh).Format("2006-01-02")),
		fmt.Sprintf("%s|%s|10|claude|claude-sonnet-4-5|2000|100|0|0|0|0", device.DeviceID, dayAt(-1, 10).In(sh).Format("2006-01-02")),
		fmt.Sprintf("%s|%s|9|claude|claude-sonnet-4-5|300|0|0|0|0|0", device.DeviceID, dayAt(0, 9).In(sh).Format("2006-01-02")),
	}
	if strings.Join(rows, ";") != strings.Join(want, ";") {
		t.Fatalf("backfill rows != worked example:\n got %v\nwant %v", rows, want)
	}

	// Second launch is not a first run: the backfill must not fire again.
	days, err = backfillHistory(ctx, store, userID, shared, "", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Fatalf("non-first run backfilled %d days, want 0", days)
	}
	if rows2 := rowsOf(t, store); len(rows2) != 3 {
		t.Fatalf("second run changed history: %v", rows2)
	}
}

// Pre-existing history (any row before today) suppresses the implicit
// backfill.
func TestBackfillSkipsWhenHistoryExists(t *testing.T) {
	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/hist.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	userID, _ := store.CreateUser(ctx, "hist-user", "", "", "")
	seedHistory(t, store, userID, "old-dev", time.Now().AddDate(0, 0, -7).Format("2006-01-02"), 500)

	days, err := backfillHistory(ctx, store, userID, &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}, "", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Fatalf("history present but backfilled %d days", days)
	}
	// An explicitly empty --since is indistinguishable from not passing it:
	// it must not turn into a forced replay of the default range.
	days, err = backfillHistory(ctx, store, userID, &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}, "", true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Fatalf("explicit empty --since replayed %d days, want 0", days)
	}
}

// An explicit --since opts into a replay even when history exists: the local
// device's days are rebuilt Latest-wins, while other devices' rows and days
// outside the replay keep whatever is stored.
func TestBackfillExplicitSinceReplaysDespiteHistory(t *testing.T) {
	now := time.Now()
	dayAt := func(offset, hour int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location()).AddDate(0, 0, offset)
	}
	today, yesterday := dayAt(0, 9), dayAt(-1, 10)
	sh := time.FixedZone("CST", 8*3600)
	isolateHome(t, t.TempDir())
	t.Setenv("TZ", "Asia/Shanghai")
	t.Setenv("CLAUDE_CONFIG_DIR", webFixture(t,
		webUsageLine("r0", "claude-sonnet-4-5", "rr0", today, 300, 0, 0, 0, 0),
		webUsageLine("r1", "claude-sonnet-4-5", "rr1", yesterday, 2000, 100, 0, 0, 0),
	))

	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/replay.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	userID, _ := store.CreateUser(ctx, "replay-user", "", "", "")
	// History on both a replayed day (yesterday) and a day outside the
	// replay (7 days back) — planted under a different device id, whose rows
	// Latest-wins never touches.
	seedHistory(t, store, userID, "old-dev", yesterday.Format("2006-01-02"), 500)
	seedHistory(t, store, userID, "old-dev", now.AddDate(0, 0, -7).Format("2006-01-02"), 700)

	shared := &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}
	days, err := backfillHistory(ctx, store, userID, shared, yesterday.Format("2006-01-02"), true, now)
	if err != nil {
		t.Fatal(err)
	}
	if days != 2 {
		t.Fatalf("explicit --since backfilled %d days, want 2 (yesterday+today)", days)
	}

	device, err := report.LoadDevice(mustDevicePath(t))
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(t, store)
	if len(rows) != 4 {
		t.Fatalf("replay rows = %v, want 4", rows)
	}
	// Contains-match instead of ordered join: the two devices' ids sort
	// relative to each other unpredictably.
	got := strings.Join(rows, "\n")
	want := []string{
		fmt.Sprintf("old-dev|%s|9|claude|claude-sonnet-4-5|700|0|0|0|0|0", now.AddDate(0, 0, -7).Format("2006-01-02")),
		fmt.Sprintf("old-dev|%s|9|claude|claude-sonnet-4-5|500|0|0|0|0|0", yesterday.Format("2006-01-02")),
		fmt.Sprintf("%s|%s|10|claude|claude-sonnet-4-5|2000|100|0|0|0|0", device.DeviceID, yesterday.In(sh).Format("2006-01-02")),
		fmt.Sprintf("%s|%s|9|claude|claude-sonnet-4-5|300|0|0|0|0|0", device.DeviceID, today.In(sh).Format("2006-01-02")),
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("replay rows missing %q:\n got %v", w, rows)
		}
	}
}

func TestBackfillSinceValidation(t *testing.T) {
	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/val.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	userID, _ := store.CreateUser(ctx, "val-user", "", "", "")
	shared := &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}
	now := time.Now()

	if _, err := backfillHistory(ctx, store, userID, shared, "2026-13-99", true, now); err == nil {
		t.Fatal("malformed --since accepted")
	}
	far := now.AddDate(-3, 0, 0).Format("2006-01-02")
	if _, err := backfillHistory(ctx, store, userID, shared, far, true, now); err == nil {
		t.Fatal("--since beyond the 550-day cap accepted")
	}
}

// BuildBackfill skips days without data by construction; the ingest keeps
// the same promise (covered for empty single snapshots elsewhere).
func TestBackfillEmptyLogsIsNoop(t *testing.T) {
	isolateHome(t, t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", webFixture(t)) // no lines

	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/noop.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	userID, _ := store.CreateUser(ctx, "noop-user", "", "", "")

	days, err := backfillHistory(ctx, store, userID,
		&core.SharedArgs{Mode: core.ModeDisplay, Offline: true}, "", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Fatalf("empty logs backfilled %d days, want 0", days)
	}
	if rows := rowsOf(t, store); len(rows) != 0 {
		t.Fatalf("empty logs wrote rows: %v", rows)
	}
}

// A replayed backfill replaces stored days (Latest-wins per (device, date))
// instead of accumulating onto them.
func TestBackfillReplayReplacesDays(t *testing.T) {
	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/replay.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	userID, _ := store.CreateUser(ctx, "replay-user", "", "", "")

	cell := func(input uint64) []report.HourCell {
		return []report.HourCell{{Hour: 9, Tool: "claude", Model: "claude-sonnet-4-5", Input: input}}
	}
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	for _, input := range []uint64{100, 250} {
		if err := ingestSnapshots(ctx, store, userID, []*report.Snapshot{{
			DeviceID: "dev-web", DeviceLabel: "box", Date: date, Hours: cell(input),
		}}); err != nil {
			t.Fatal(err)
		}
	}
	rows := rowsOf(t, store)
	if len(rows) != 1 || !strings.Contains(rows[0], "|250|") {
		t.Fatalf("replay did not replace the day: %v", rows)
	}
}
