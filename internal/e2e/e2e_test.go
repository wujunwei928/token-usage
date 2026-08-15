// Package e2e runs the single full-stack seam: real server binary + real
// ccusage report command over fixture agent logs, asserting observable
// behavior end to end (the ingest API, Latest-wins, and the rendered pages).
package e2e

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	ccusageBin string
	serverBin  string
	repoRoot   string
)

func TestMain(m *testing.M) {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repoRoot = filepath.Dir(filepath.Dir(dir))
	tmp, err := os.MkdirTemp("", "ccusage-e2e")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	ccusageBin = filepath.Join(tmp, "ccusage")
	serverBin = filepath.Join(tmp, "server")
	for _, target := range []struct{ bin, pkg string }{{ccusageBin, "./cmd/ccusage"}, {serverBin, "./cmd/server"}} {
		build := exec.Command("go", "build", "-o", target.bin, target.pkg)
		build.Dir = repoRoot
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}

// freePort asks the kernel for an unused TCP port.
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// serverProc starts the real server binary and waits until it answers.
type serverProc struct {
	cmd  *exec.Cmd
	base string
	db   string
}

func startServer(t *testing.T, dbDir string) *serverProc {
	t.Helper()
	addr := freePort(t)
	db := filepath.Join(dbDir, "lb.db")
	cmd := exec.Command(serverBin, "serve", "-addr", addr, "-db", db)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	proc := &serverProc{cmd: cmd, base: "http://" + addr, db: db}
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
	t.Fatal("server did not come up")
	return nil
}

// addUser seeds a user through the server CLI and returns the report token.
func (p *serverProc) addUser(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command(serverBin, "add-user", "-db", p.db, "-name", name).Output()
	if err != nil {
		t.Fatalf("add-user: %v (%s)", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "report token (shown once): ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "report token (shown once): "))
		}
	}
	t.Fatalf("no token in add-user output: %q", out)
	return ""
}

func (p *serverProc) getPage(t *testing.T, path string) string {
	t.Helper()
	resp, err := http.Get(p.base + path)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(raw)
}

// usageLine builds one Claude-style JSONL usage entry stamped at when.
func usageLine(id, model, requestID string, when time.Time, input, output, cacheRead, cache5m, cache1h uint64) string {
	cacheBlock := "null"
	if cache5m > 0 || cache1h > 0 {
		cacheBlock = fmt.Sprintf(`{"ephemeral_5m_input_tokens":%d,"ephemeral_1h_input_tokens":%d}`, cache5m, cache1h)
	}
	return fmt.Sprintf(`{"sessionId":"s-%s","timestamp":"%s","version":"2.0.0","requestId":"%s",`+
		`"message":{"id":"msg_%s","model":"%s","usage":{"input_tokens":%d,"output_tokens":%d,`+
		`"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"cache_creation":%s}},`+
		`"costUSD":0.01}`,
		id, when.UTC().Format("2006-01-02T15:04:05.000Z"), requestID, id, model,
		input, output, cacheRead, cache5m+cache1h, cacheBlock)
}

// claudeFixture writes a Claude config dir carrying today's log lines.
func claudeFixture(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	project := filepath.Join(dir, "projects", "proj-one")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "session-a.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runReport executes the real client binary with a hermetic environment.
func runReport(t *testing.T, configDir, deviceFile string, extraArgs ...string) (string, string, int) {
	t.Helper()
	args := append([]string{"report", "--device-file", deviceFile}, extraArgs...)
	cmd := exec.Command(ccusageBin, args...)
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir, "HOME="+t.TempDir(), "TZ=Asia/Shanghai")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run report: %v", err)
	}
	return stdout.String(), stderr.String(), code
}

func TestReportEndToEnd(t *testing.T) {
	work := t.TempDir()
	proc := startServer(t, work)
	token := proc.addUser(t, "e2e-user")

	// Today 09:xx and 21:xx local (Asia/Shanghai), plus a yesterday line that
	// must not enter the snapshot, and a 5m/1h cache-write split.
	now := time.Now()
	today9 := time.Date(now.Year(), now.Month(), now.Day(), 9, 5, 0, 0, now.Location())
	today21 := time.Date(now.Year(), now.Month(), now.Day(), 21, 5, 0, 0, now.Location())
	yesterday := today9.AddDate(0, 0, -1)
	config := claudeFixture(t,
		usageLine("a1", "claude-sonnet-4-5", "r1", today9, 1000, 200, 5000, 300, 100),
		usageLine("a2", "claude-opus-4-1", "r2", today21, 400, 80, 0, 0, 0),
		usageLine("a3", "claude-sonnet-4-5", "r3", yesterday, 99999, 1, 0, 0, 0),
	)
	device := filepath.Join(work, "device.json")

	stdout, stderr, code := runReport(t, config, device, "--server", proc.base, "--token", token)
	if code != 0 {
		t.Fatalf("report failed (%d): %s %s", code, stdout, stderr)
	}
	// Snapshot total: (1000+200+5000+300+100) + (400+80) = 7080.
	if !strings.Contains(stdout, "7080") {
		t.Fatalf("day tokens 7080 missing from output: %s", stdout)
	}
	// Device identity persisted.
	if _, err := os.Stat(device); err != nil {
		t.Fatal("device.json not created")
	}

	// The leaderboard page shows the aggregated numbers for today.
	page := proc.getPage(t, "/?range=today")
	for _, want := range []string{"e2e-user", "7.1K", "claude-sonnet-4-5"} {
		if !strings.Contains(page, want) {
			t.Fatalf("leaderboard missing %q", want)
		}
	}
	// Yesterday's line was excluded (yesterday board is empty of this user's
	// 100000-token line).
	if page2 := proc.getPage(t, "/?range=yesterday"); strings.Contains(page2, "100.0K") {
		t.Fatalf("yesterday line leaked into snapshot: %s", page2)
	}

	// Latest-wins: rewrite the log to a single smaller line, rerun.
	config2 := claudeFixture(t,
		usageLine("a1", "claude-sonnet-4-5", "r1", today9, 100, 20, 0, 0, 0),
	)
	stdout, _, code = runReport(t, config2, device, "--server", proc.base, "--token", token)
	if code != 0 {
		t.Fatalf("rerun failed: %s", stdout)
	}
	page = proc.getPage(t, "/?range=today")
	if strings.Contains(page, "7.1K") {
		t.Fatal("latest-wins did not replace the day's data")
	}
	if !strings.Contains(page, "120") {
		t.Fatal("replacement total 120 not visible")
	}

	// Failure UX: revoked token → exit 4 with guidance.
	badToken := proc.addUser(t, "other")
	_, stderr, code = runReport(t, config2, device, "--server", proc.base, "--token", badToken+"x")
	if code != 4 || !strings.Contains(stderr, "设置") {
		t.Fatalf("invalid token UX: code=%d stderr=%q", code, stderr)
	}
	// Unreachable server → exit 3.
	_, _, code = runReport(t, config2, device, "--server", "http://127.0.0.1:1", "--token", badToken)
	if code != 3 {
		t.Fatalf("network failure exit code = %d, want 3", code)
	}
	// Missing config → exit 2 with guidance.
	_, stderr, code = runReport(t, config2, device)
	if code != 2 || !strings.Contains(stderr, "reportServer") {
		t.Fatalf("missing config UX: code=%d stderr=%q", code, stderr)
	}
}

func TestReportDryRunTodayOnly(t *testing.T) {
	now := time.Now()
	today9 := time.Date(now.Year(), now.Month(), now.Day(), 9, 5, 0, 0, now.Location())
	tomorrow := today9.AddDate(0, 0, 1)
	config := claudeFixture(t,
		usageLine("b1", "claude-sonnet-4-5", "r1", today9, 10, 5, 0, 50, 20),
		usageLine("b2", "claude-sonnet-4-5", "r2", tomorrow, 77, 7, 0, 0, 0),
	)
	stdout, _, code := runReport(t, config, filepath.Join(t.TempDir(), "d.json"), "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run failed: %s", stdout)
	}
	if !strings.Contains(stdout, `"date"`) || !strings.Contains(stdout, `"cacheWrite5m": 50`) {
		t.Fatalf("dry-run payload malformed:\n%s", stdout)
	}
	if strings.Contains(stdout, "77") {
		t.Fatalf("tomorrow's entry leaked into today's snapshot:\n%s", stdout)
	}
}

// codexLine writes one Codex token_count event line stamped at when.
func codexLine(model string, when time.Time, input, cached, output uint64) string {
	return fmt.Sprintf(`{"timestamp":"%s","type":"event_msg","payload":{"type":"token_count",`+
		`"info":{"model":"%s","last_token_usage":{"input_tokens":%d,"cached_input_tokens":%d,`+
		`"output_tokens":%d,"reasoning_output_tokens":0,"total_tokens":%d},`+
		`"total_token_usage":{"input_tokens":%d,"cached_input_tokens":%d,"output_tokens":%d,`+
		`"reasoning_output_tokens":0,"total_tokens":%d}}}}`,
		when.UTC().Format("2006-01-02T15:04:05.000Z"), model,
		input, cached, output, input+cached+output,
		input, cached, output, input+cached+output)
}

func TestMultiAgentEndToEnd(t *testing.T) {
	work := t.TempDir()
	proc := startServer(t, work)
	token := proc.addUser(t, "multi-user")

	now := time.Now()
	today10 := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())

	// Claude side: the same message replayed in two files must dedup to one.
	claudeDir := t.TempDir()
	project := filepath.Join(claudeDir, "projects", "proj-multi")
	os.MkdirAll(project, 0o755)
	line := usageLine("m1", "claude-sonnet-4-5", "req-m1", today10, 1000, 100, 0, 0, 0)
	os.WriteFile(filepath.Join(project, "one.jsonl"), []byte(line+"\n"), 0o644)
	os.WriteFile(filepath.Join(project, "two.jsonl"), []byte(line+"\n"), 0o644)

	// Codex side under CODEX_HOME.
	codexDir := t.TempDir()
	sessions := filepath.Join(codexDir, "sessions", "rollout-1")
	os.MkdirAll(sessions, 0o755)
	os.WriteFile(filepath.Join(sessions, "session-c.jsonl"),
		[]byte(codexLine("gpt-5.3-codex", today10, 2000, 500, 300)+"\n"), 0o644)

	device := filepath.Join(work, "device.json")
	args := []string{"report", "--device-file", device, "--server", proc.base, "--token", token}
	cmd := exec.Command(ccusageBin, args...)
	cmd.Env = append(os.Environ(),
		"CLAUDE_CONFIG_DIR="+claudeDir, "CODEX_HOME="+codexDir,
		"HOME="+t.TempDir(), "TZ=Asia/Shanghai")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "上报成功") {
		t.Fatalf("multi-agent report failed: %s", out)
	}

	// Totals: claude 1100 (replay deduped to one) + codex 2800
	// (input 2000 + cache-read 500 + output 300) = 3900.
	page := proc.getPage(t, "/?range=today")
	if !strings.Contains(page, "3.9K") {
		t.Fatalf("multi-agent total missing (want 3.9K): %s", page)
	}
	// Tool split: codex filter shows codex only (2800 = 2.8K).
	codexPage := proc.getPage(t, "/?range=today&tool=codex")
	if !strings.Contains(codexPage, "2.8K") {
		t.Fatalf("codex tool slice wrong: %s", codexPage)
	}
	if strings.Contains(proc.getPage(t, "/?range=today&tool=codex"), "1.1K") {
		t.Fatal("claude usage leaked into codex filter")
	}
}

func TestBackfillEndToEnd(t *testing.T) {
	work := t.TempDir()
	proc := startServer(t, work)
	token := proc.addUser(t, "backfiller")

	now := time.Now()
	days := []time.Time{
		time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location()).AddDate(0, 0, -2),
		time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location()).AddDate(0, 0, -1),
		time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, now.Location()),
	}
	var lines []string
	for i, when := range days {
		lines = append(lines, usageLine(
			fmt.Sprintf("bf%d", i), "claude-sonnet-4-5", fmt.Sprintf("rb%d", i), when,
			uint64(1000*(i+1)), 100, 0, 0, 0))
	}
	config := claudeFixture(t, lines...)
	device := filepath.Join(work, "device.json")

	stdout, _, code := runReport(t, config, device,
		"--server", proc.base, "--token", token,
		"--since", now.AddDate(0, 0, -2).Format("2006-01-02"))
	if code != 0 {
		t.Fatalf("backfill failed: %s", stdout)
	}
	// Totals: 1000+2000+3000 inputs + 3×100 outputs = 6300.
	if !strings.Contains(stdout, "6300") {
		t.Fatalf("backfill total missing: %s", stdout)
	}

	// The 30-day board shows the full backfilled window.
	page := proc.getPage(t, "/?range=30d")
	if !strings.Contains(page, "6.3K") {
		t.Fatalf("30d board missing 6.3K: %s", page)
	}
	// Day 3 only: exactly day-before-yesterday's row (1100 = 1.1K).
	if page := proc.getPage(t, "/?range=daybefore"); !strings.Contains(page, "1.1K") {
		t.Fatalf("daybefore board missing 1.1K: %s", page)
	}

	// Latest-wins per date: rerun backfill with an edited log for day -1 only.
	lines[1] = usageLine("bf1", "claude-sonnet-4-5", "rb1", days[1], 500, 50, 0, 0, 0)
	config2 := claudeFixture(t, lines...)
	stdout, _, code = runReport(t, config2, device,
		"--server", proc.base, "--token", token,
		"--since", now.AddDate(0, 0, -2).Format("2006-01-02"))
	if code != 0 {
		t.Fatalf("backfill rerun failed: %s", stdout)
	}
	if page := proc.getPage(t, "/?range=30d"); !strings.Contains(page, "4.8K") {
		t.Fatalf("rerun total wrong (want 4750): %s", page)
	}
}
