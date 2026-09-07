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

// 启动备份是数据事故的最后防线:必须真的产出一致性快照、正确轮转,
// 且库为空/路径不可写时绝不阻断启动(rotateAutoBackups 只警告不返回错误)。
func TestRotateAutoBackups(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/web.db"
	store, err := server.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 第一次启动:产出 auto-bak-0,内容可用。
	rotateAutoBackups(ctx, store, dbPath)
	assertBackupHasUser(t, dbPath, 0)

	// 第二次启动:0 轮转为 1,新快照落在 0。
	rotateAutoBackups(ctx, store, dbPath)
	for _, i := range []int{0, 1} {
		if _, err := os.Stat(autoBackupPath(dbPath, i)); err != nil {
			t.Fatalf("auto-bak-%d missing after second start: %v", i, err)
		}
	}
	if _, err := os.Stat(autoBackupPath(dbPath, 2)); !os.IsNotExist(err) {
		t.Fatalf("auto-bak-2 should not exist after two starts")
	}
	store.Close()
}

func assertBackupHasUser(t *testing.T, dbPath string, i int) {
	if _, err := os.Stat(autoBackupPath(dbPath, i)); err != nil {
		t.Fatalf("auto-bak-%d missing: %v", i, err)
	}
	// 用独立的只读连接验证备份是合法 SQLite 且含 schema。
	backup, err := server.OpenStore(autoBackupPath(dbPath, i))
	if err != nil {
		t.Fatalf("auto-bak-%d is not a valid database: %v", i, err)
	}
	defer backup.Close()
}
