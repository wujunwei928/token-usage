package server

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Local Panel branding (web-restyle ticket 02): in local mode the chrome is
// the single-user「Token 用量」face — no leaderboard, no login/register, no
// logout — while server serve keeps the community「Token 排行榜」branding.
func TestLocalPanelBranding(t *testing.T) {
	_, mux := newLocalWeb(t, WithLocalRoot(), WithRefresh(func() error { return nil }))
	body := getFrom(t, mux, "127.0.0.1:53812", "/me").Body.String()

	if !strings.Contains(body, "Token 用量") {
		t.Error("local mode: missing「Token 用量」brand")
	}
	if !strings.Contains(body, "本机数据 · 不出网") {
		t.Error("local mode: missing「本机数据 · 不出网」annotation")
	}
	for _, leftover := range []string{
		"Token 排行榜",
		`href="/register"`,
		`href="/login"`,
		">排行榜</a>",
		`action="/logout"`,
	} {
		if strings.Contains(body, leftover) {
			t.Errorf("local mode: community chrome leaked: %q", leftover)
		}
	}
}

func TestServeModeBrandingUnchanged(t *testing.T) {
	_, _, srv := newTestServer(t)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page := readAll(t, resp)
	for _, want := range []string{"Token 排行榜", `href="/register"`, `href="/login"`, ">排行榜</a>"} {
		if !strings.Contains(page, want) {
			t.Errorf("serve mode: community branding missing %q", want)
		}
	}
	if strings.Contains(page, "本机数据 · 不出网") {
		t.Error("serve mode: local annotation leaked into community page")
	}
}

// Rebrand guard (ADR 0008): templates must not regress to `ccusage` copy.
func TestTemplatesNoCcusageCopy(t *testing.T) {
	var files []string
	err := filepath.WalkDir("web/templates", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".html") {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("template walk found only %d files", len(files))
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(raw)), "ccusage") {
			t.Errorf("%s: stale ccusage copy", f)
		}
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
