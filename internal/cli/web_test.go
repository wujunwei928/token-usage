package cli

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wujunwei928/token-usage/internal/server"
)

// The web command's surface is the loopback invariant itself (ADR 0012): it
// offers a port but no address, so nothing can lift the implicit identity
// off loopback by flag combination.
func TestWebCommandSurface(t *testing.T) {
	cmd := newWebCommand()
	if cmd.Use != "web" {
		t.Fatalf("Use = %q, want web", cmd.Use)
	}
	flags := cmd.Flags()
	for _, name := range []string{"port", "db", "name", "pricing", "since", "refresh"} {
		if flags.Lookup(name) == nil {
			t.Fatalf("flag --%s missing from web command", name)
		}
	}
	if flags.Lookup("addr") != nil {
		t.Fatal("web command must not expose --addr (loopback is structural)")
	}
	if port, _ := flags.GetInt("port"); port != 8787 {
		t.Fatalf("default port = %d, want 8787", port)
	}
	if d, _ := flags.GetDuration("refresh"); d != 15*time.Minute {
		t.Fatalf("default refresh = %s, want 15m", d)
	}
}

// Non-positive refresh intervals must fail fast with a readable error, not
// spin a zero ticker.
func TestWebRefreshFlagValidation(t *testing.T) {
	for _, bad := range []string{"0", "-5s", "0s"} {
		cmd := newWebCommand()
		cmd.SetArgs([]string{"--refresh", bad, "--db", t.TempDir() + "/unused.db"})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--refresh") {
			t.Fatalf("--refresh %s: err = %v, want --refresh error", bad, err)
		}
	}
}

// 启动备份是数据事故的最后防线:必须真的产出一致性快照、落到当天的
// backups/<日期>/ 目录、重复启动覆盖不报错,且绝不阻断启动。
func TestStartupBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/web.db"
	store, err := server.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	startupBackup(ctx, store, dbPath)
	target := dailyBackupTarget(dbPath, time.Now())
	assertValidBackup(t, target)

	// 当天重复启动:覆盖同一路径,不报错不留垃圾。
	startupBackup(ctx, store, dbPath)
	assertValidBackup(t, target)

	entries, err := os.ReadDir(dir + "/backups")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("backups dir has %d entries, want only today's", len(entries))
	}
}

func assertValidBackup(t *testing.T, path string) {
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("backup %s missing: %v", path, err)
	}
	// 用独立的连接验证备份是合法 SQLite 且含 schema。
	backup, err := server.OpenStore(path)
	if err != nil {
		t.Fatalf("backup %s is not a valid database: %v", path, err)
	}
	backup.Close()
}
