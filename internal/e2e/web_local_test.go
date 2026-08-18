package e2e

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// webProc runs the real binary in web-local mode over isolated fixture
// logs, on a free loopback port.
type webProc struct {
	cmd     *exec.Cmd
	base    string
	logPath string
}

func startWeb(t *testing.T, dbDir, home, claudeDir string, refresh time.Duration) *webProc {
	t.Helper()
	addr := freePort(t)
	port := strings.TrimPrefix(addr, "127.0.0.1:")
	args := []string{"web", "--port", port, "--db", filepath.Join(dbDir, "web.db")}
	if refresh > 0 {
		args = append(args, "--refresh", refresh.String())
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"CLAUDE_CONFIG_DIR="+claudeDir, "HOME="+home, "TZ=Asia/Shanghai")
	logPath := filepath.Join(dbDir, "web.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	proc := &webProc{cmd: cmd, base: "http://" + addr, logPath: logPath}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(proc.base + "/about")
		if err == nil {
			resp.Body.Close()
			return proc
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("web server did not come up: %s", proc.logDump(t))
	return nil
}

func (p *webProc) logDump(t *testing.T) string {
	t.Helper()
	raw, _ := os.ReadFile(p.logPath)
	return string(raw)
}

func (p *webProc) getPage(t *testing.T, path string) string {
	t.Helper()
	return fetchPage(t, p.base, path)
}

// The web-local chain: one command → no login → /me shows today's real
// aggregated local usage, first-run backfill fills history, and the running
// server picks up new log lines via the refresh interval.
func TestWebLocalEndToEnd(t *testing.T) {
	work := t.TempDir()
	home := filepath.Join(work, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	at := func(offset, hour int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), hour, 5, 0, 0, now.Location()).AddDate(0, 0, offset)
	}
	// History for the first-run backfill (yesterday 1100) plus today's
	// baseline (120).
	logPath := filepath.Join(work, "logs")
	claudeDir := filepath.Join(logPath, "claude")
	project := filepath.Join(claudeDir, "projects", "p")
	os.MkdirAll(project, 0o755)
	lines := []string{
		usageLine("e1", "claude-sonnet-4-5", "re1", at(-1, 9), 1000, 100, 0, 0, 0),
		usageLine("e2", "claude-sonnet-4-5", "re2", at(0, 8), 100, 20, 0, 0, 0),
	}
	writeLines := func() {
		os.WriteFile(filepath.Join(project, "session-e.jsonl"),
			[]byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
	writeLines()

	proc := startWeb(t, work, home, claudeDir, 500*time.Millisecond)

	// No login: /me answers directly (no redirect to /login), and the root
	// lands on the dashboard too.
	page := proc.getPage(t, "/me")
	if !strings.Contains(page, "我的 Token") || !strings.Contains(page, "刷新数据") {
		t.Fatalf("/me did not render the local dashboard with its refresh entry")
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	root, err := noRedirect.Get(proc.base + "/")
	if err != nil {
		t.Fatal(err)
	}
	root.Body.Close()
	if root.StatusCode != http.StatusSeeOther || root.Header.Get("Location") != "/me" {
		t.Fatalf("local root did not redirect to /me: %d %q", root.StatusCode, root.Header.Get("Location"))
	}
	// First-run backfill: yesterday's 1100 lands with today's 120 in the
	// cumulative card (1100 + 120 = 1220 → 1.2K).
	page = proc.getPage(t, "/me")
	if !strings.Contains(page, "1.2K") {
		t.Fatalf("cumulative total missing backfilled history:\n%s", proc.logDump(t))
	}

	// New local usage appears without a restart: append a line, wait for the
	// refresh interval, and the dashboard total grows (120 → 7080).
	lines = append(lines, usageLine("e3", "claude-opus-4-1", "re3", at(0, 21), 6000, 900, 10, 40, 10))
	writeLines()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		page := proc.getPage(t, "/me")
		if strings.Contains(page, "7.1K") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.Contains(proc.getPage(t, "/me"), "7.1K") {
		t.Fatalf("refreshed total 7.1K missing after interval:\n%s", proc.logDump(t))
	}
}

// The loopback invariant in the shipped binary: the banner names the
// loopback address, and there is no flag that can change it.
func TestWebLocalBindsLoopbackOnly(t *testing.T) {
	work := t.TempDir()
	empty := filepath.Join(work, "empty-claude")
	os.MkdirAll(filepath.Join(empty, "projects"), 0o755)
	proc := startWeb(t, work, filepath.Join(work, "h"), empty, 0)
	if !strings.Contains(proc.logDump(t), "127.0.0.1") {
		t.Fatalf("web banner did not name the loopback address:\n%s", proc.logDump(t))
	}
	out, err := exec.Command(bin, "web", "--addr", "0.0.0.0:8787").CombinedOutput()
	if err == nil {
		t.Fatal("web accepted --addr (loopback must be structural)")
	}
	if !strings.Contains(string(out), "--addr") {
		t.Fatalf("--addr rejection did not name the flag: %s", out)
	}
}
