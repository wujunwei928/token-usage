package server

import (
	"strings"
	"testing"
)

// Theme infrastructure (web-restyle ticket 01): every rendered page carries
// the anti-FOUC bootstrap (resolves stored/auto preference to data-theme
// before the stylesheet paints), the three-state toggle control, and a
// change event charts can subscribe to.
func TestThemeInfrastructureOnEveryPage(t *testing.T) {
	_, mux := newLocalWeb(t)

	pages := map[string]string{
		"/me":      "dashboard",
		"/pricing": "pricing",
		"/about":   "about",
		"/login":   "login",
	}
	for path, name := range pages {
		resp := getFrom(t, mux, "127.0.0.1:53812", path)
		if resp.Code != 200 {
			t.Fatalf("%s: status %d", path, resp.Code)
		}
		body := resp.Body.String()
		if !strings.Contains(body, `document.documentElement.dataset.theme`) {
			t.Errorf("%s: missing anti-FOUC bootstrap script", name)
		}
		if !strings.Contains(body, "prefers-color-scheme") {
			t.Errorf("%s: bootstrap must consult prefers-color-scheme for auto mode", name)
		}
		if !strings.Contains(body, `id="theme-toggle"`) {
			t.Errorf("%s: missing three-state theme toggle button", name)
		}
	}

	// The toggle controller lives in app.js: it must dispatch the change
	// event charts subscribe to, and keep auto mode live against the OS.
	resp := getFrom(t, mux, "127.0.0.1:53812", "/static/app.js")
	if resp.Code != 200 {
		t.Fatalf("app.js status %d", resp.Code)
	}
	js := resp.Body.String()
	if !strings.Contains(js, "tu-themechange") {
		t.Error("app.js missing tu-themechange event dispatch")
	}
	if !strings.Contains(js, "prefers-color-scheme") || !strings.Contains(js, "addEventListener('change'") {
		t.Error("app.js must re-resolve auto mode when the OS theme changes")
	}
}

// The stylesheet must be token-driven dual-theme: a [data-theme="dark"]
// override block and no raw hex colors outside :root token definitions.
func TestStylesheetDualTheme(t *testing.T) {
	_, mux := newLocalWeb(t)
	resp := getFrom(t, mux, "127.0.0.1:53812", "/static/style.css")
	if resp.Code != 200 {
		t.Fatalf("style.css status %d", resp.Code)
	}
	css := resp.Body.String()
	if !strings.Contains(css, `[data-theme="dark"]`) {
		t.Error("stylesheet has no dark-theme token override block")
	}
	if !strings.Contains(css, "--accent:") || !strings.Contains(css, "--bg:") {
		t.Error("stylesheet missing core design tokens (--bg/--accent)")
	}
}
