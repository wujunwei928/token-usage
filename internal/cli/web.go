package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/core"
	"github.com/wujunwei928/token-usage/internal/report"
	"github.com/wujunwei928/token-usage/internal/server"
)

// newWebCommand builds `token-usage web`: the loopback-bound local dashboard
// (ADR 0012). One command, no login, no report token — the browser opens
// straight into /me. The listen address is structural, not configurable:
// without an --addr there is no flag combination that can put the
// implicit-identity pages on a network interface.
func newWebCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Browse your own token usage in a browser (local, loopback-only)",
		Long: "Serve the personal usage dashboard on 127.0.0.1 and ingest local\n" +
			"agent logs in-process — nothing leaves this machine. First run seeds a\n" +
			"local user and backfills recent history; browser opens straight into /me.",
		RunE:         runWeb,
		SilenceUsage: true,
	}
	flags := cmd.Flags()
	flags.Int("port", 8787, "loopback listen port (address is always 127.0.0.1)")
	flags.String("db", "", "SQLite database path (default <user config dir>/token-usage/web.db)")
	flags.String("name", "", "local user name (default: OS user name)")
	flags.String("pricing", "", "model-prices.json override path")
	flags.String("since", "", "first-run backfill start date YYYY-MM-DD (default: 29 days back)")
	flags.Duration("refresh", 15*time.Minute, "re-aggregate today's local usage every interval")
	return cmd
}

func runWeb(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	port, _ := flags.GetInt("port")
	if port <= 0 || port > 65535 {
		return fmt.Errorf("--port must be 1-65535 (got %d)", port)
	}
	dbPath, _ := flags.GetString("db")
	if dbPath == "" {
		var err error
		dbPath, err = defaultWebDBPath()
		if err != nil {
			return err
		}
	}
	name, _ := flags.GetString("name")
	if name == "" {
		name = defaultLocalUserName()
	}
	pricingPath, _ := flags.GetString("pricing")
	refreshEvery, _ := flags.GetDuration("refresh")
	if refreshEvery <= 0 {
		return fmt.Errorf("--refresh must be a positive duration (got %s)", refreshEvery)
	}

	store, err := server.OpenStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	pricing, err := server.LoadPricing(pricingPath)
	if err != nil {
		return err
	}

	localUser, err := ensureLocalUser(cmd.Context(), store, name)
	if err != nil {
		return err
	}

	// In-process ingest of today's local usage (same pipeline the report
	// command runs) — no HTTP hop, no token. First run also lands recent
	// history so the 30-day charts start full.
	shared := &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}
	if err := ingestToday(cmd.Context(), store, localUser.ID, shared); err != nil {
		return err
	}
	since, _ := flags.GetString("since")
	days, err := backfillOnFirstRun(cmd.Context(), store, localUser.ID, shared, since, time.Now())
	if err != nil {
		return err
	}
	if days > 0 {
		fmt.Fprintf(os.Stderr, "首次运行:已回溯 %d 天历史数据\n", days)
	}

	// One ingest path shared by the interval ticker and the /me refresh
	// button, serialized so they never interleave mid-scan.
	var refreshMu sync.Mutex
	refreshNow := func() error {
		refreshMu.Lock()
		defer refreshMu.Unlock()
		return ingestToday(context.Background(), store, localUser.ID, shared)
	}
	go func() {
		ticker := time.NewTicker(refreshEvery)
		defer ticker.Stop()
		for range ticker.C {
			if err := refreshNow(); err != nil {
				fmt.Fprintf(os.Stderr, "刷新本机用量失败: %v\n", err)
			}
		}
	}()

	mux := http.NewServeMux()
	web, err := server.NewWeb(store, pricing,
		server.WithLoopbackUser(localUser.ID),
		server.WithRefresh(refreshNow),
		server.WithLocalRoot(),
	)
	if err != nil {
		return err
	}
	web.Register(mux)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Fprintf(os.Stderr, "token-usage web listening on http://%s (db: %s)\n", addr, filepath.Base(dbPath))
	bestEffortOpen("http://" + addr)
	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}

// defaultWebDBPath resolves the web command's persistent database under the
// token-usage config namespace (ADR 0007), so history survives restarts
// without colliding with a deployment's leaderboard.db in the working
// directory.
func defaultWebDBPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "token-usage", "web.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// bestEffortOpen launches the platform browser when the command runs on an
// interactive terminal; headless boxes fail silently and the printed URL
// remains the source of truth.
func bestEffortOpen(url string) {
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return
	}
	var open *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		open = exec.Command("open", url)
	case "windows":
		open = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		open = exec.Command("xdg-open", url)
	}
	_ = open.Start()
}

// defaultLocalUserName labels the implicit local user.
func defaultLocalUserName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "local"
}

// ensureLocalUser finds (or seeds on first run) the web command's user. No
// password: the loopback origin is the credential (ADR 0012).
func ensureLocalUser(ctx context.Context, store *server.Store, name string) (*server.User, error) {
	existing, err := store.UserByName(ctx, name)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err := store.CreateUser(ctx, name, "", "", ""); err != nil {
		return nil, err
	}
	return store.UserByName(ctx, name)
}

// loadLocalDevice resolves the persisted device identity shared with the
// report command.
func loadLocalDevice() (*report.DeviceFile, error) {
	devicePath, err := report.DefaultDevicePath()
	if err != nil {
		return nil, err
	}
	return report.LoadDevice(devicePath)
}

// ingestSnapshots writes snapshots through the store's Latest-wins units —
// one ReplaceDay per (device, date) — the same seam the HTTP ingest path
// drives, minus the wire hop. Snapshots without cells are skipped: an absent
// day must never clear what the server already stores.
func ingestSnapshots(ctx context.Context, store *server.Store, userID int64, snapshots []*report.Snapshot) error {
	for _, snap := range snapshots {
		if snap == nil || len(snap.Hours) == 0 {
			continue
		}
		rows := make([]server.HourRow, len(snap.Hours))
		for i := range snap.Hours {
			c := &snap.Hours[i]
			rows[i] = server.HourRow{
				Hour: c.Hour, Tool: c.Tool, Model: c.Model,
				Input: c.Input, Output: c.Output,
				CacheRead: c.CacheRead, Cache5m: c.Cache5m, Cache1h: c.Cache1h,
			}
		}
		if _, err := store.ReplaceDay(ctx, userID, snap.DeviceID, snap.DeviceLabel, snap.Date, rows); err != nil {
			return fmt.Errorf("ingest %s for %s: %w", snap.Date, snap.DeviceID, err)
		}
	}
	return nil
}

// defaultBackfillDays is the first-run history window: today plus the
// previous 29 days.
const defaultBackfillDays = 29

// backfillOnFirstRun lands local history for a user the first time they have
// none (any row dated before today suppresses it). Empty days never enter
// the snapshot set, so re-running is a per-day Latest-wins no-op. Returns
// the number of days ingested.
func backfillOnFirstRun(ctx context.Context, store *server.Store, userID int64, shared *core.SharedArgs, since string, now time.Time) (int, error) {
	today := now.Format("2006-01-02")
	if since == "" {
		since = now.AddDate(0, 0, -defaultBackfillDays).Format("2006-01-02")
	} else {
		parsed, err := time.Parse("2006-01-02", since)
		if err != nil {
			return 0, fmt.Errorf("--since must be YYYY-MM-DD (got %q)", since)
		}
		if days := int(now.Sub(parsed).Hours()/24) + 1; days > server.MaxBackfillDays {
			return 0, fmt.Errorf("too many days in backfill (max %d)", server.MaxBackfillDays)
		}
	}

	has, err := store.HasUsageBefore(ctx, userID, today)
	if err != nil {
		return 0, err
	}
	if has {
		return 0, nil
	}

	device, err := loadLocalDevice()
	if err != nil {
		return 0, err
	}
	snapshots := report.BuildBackfill(shared, device.DeviceID, device.Label, since, now)
	// The in-process path skips the HTTP validation, so the same backfill
	// caps apply here (day count on real data, rows summed across days).
	totalRows := 0
	for _, snap := range snapshots {
		totalRows += len(snap.Hours)
	}
	if len(snapshots) > server.MaxBackfillDays {
		return 0, fmt.Errorf("too many days in backfill (max %d)", server.MaxBackfillDays)
	}
	if totalRows > server.MaxBackfillRows {
		return 0, fmt.Errorf("backfill exceeds the %d total row cap", server.MaxBackfillRows)
	}
	if err := ingestSnapshots(ctx, store, userID, snapshots); err != nil {
		return 0, err
	}
	return len(snapshots), nil
}

// ingestToday runs the same aggregation pipeline the report command uses
// (all agents, identical dedup semantics, local timezone) for the current
// day and lands it in the local store.
func ingestToday(ctx context.Context, store *server.Store, userID int64, shared *core.SharedArgs) error {
	device, err := loadLocalDevice()
	if err != nil {
		return err
	}
	snap := report.BuildSnapshot(shared, device.DeviceID, device.Label, time.Now())
	if len(snap.Hours) == 0 {
		return nil
	}
	return ingestSnapshots(ctx, store, userID, []*report.Snapshot{snap})
}
