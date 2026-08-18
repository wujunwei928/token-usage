package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newLocalWeb builds the page server in its token-usage web shape: a seeded
// user plus the loopback implicit-identity option (ADR 0012).
func newLocalWeb(t *testing.T, opts ...WebOption) (*Store, http.Handler) {
	t.Helper()
	store, err := OpenStore(t.TempDir() + "/local.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	userID, err := store.CreateUser(context.Background(), "local-user", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	pricing, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	all := append([]WebOption{WithLoopbackUser(userID)}, opts...)
	web, err := NewWeb(store, pricing, all...)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	web.Register(mux)
	return store, mux
}

// getFrom requests path with a spoofed RemoteAddr through the handler tree.
func getFrom(t *testing.T, h http.Handler, remoteAddr, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	return resp
}

func TestLoopbackImplicitUser(t *testing.T) {
	_, mux := newLocalWeb(t)

	for _, loopback := range []string{"127.0.0.1:53812", "[::1]:53812"} {
		resp := getFrom(t, mux, loopback, "/me")
		if resp.Code != http.StatusOK {
			t.Fatalf("loopback %s: /me = %d, want 200", loopback, resp.Code)
		}
		if !strings.Contains(resp.Body.String(), "我的 Token") {
			t.Fatalf("loopback %s: dashboard did not render", loopback)
		}
	}

	resp := getFrom(t, mux, "192.0.2.9:53812", "/me")
	if resp.Code != http.StatusSeeOther || resp.Header().Get("Location") != "/login" {
		t.Fatalf("non-loopback: /me = %d %q, want redirect to /login", resp.Code, resp.Header().Get("Location"))
	}
}

// The exemption must stay opt-in: the default web server (server serve) keeps
// requiring a session even for loopback requests.
func TestLoopbackExemptionIsOptIn(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/plain.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pricing, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	web, err := NewWeb(store, pricing)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	web.Register(mux)

	resp := getFrom(t, mux, "127.0.0.1:53812", "/me")
	if resp.Code != http.StatusSeeOther || resp.Header().Get("Location") != "/login" {
		t.Fatalf("default server: loopback /me = %d, want redirect to /login", resp.Code)
	}
}

// The web-local manual refresh entry: POST /refresh runs the injected ingest
// and bounces back to /me, whose page carries the refresh form — and only in
// this mode (the deployment server never mounts it).
func TestLocalRefreshEntry(t *testing.T) {
	refreshed := 0
	_, mux := newLocalWeb(t, WithRefresh(func() error {
		refreshed++
		return nil
	}))

	me := getFrom(t, mux, "127.0.0.1:53812", "/me")
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `action="/refresh"`) {
		t.Fatalf("/me missing local refresh entry: %d", me.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.RemoteAddr = "127.0.0.1:53812"
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther || resp.Header().Get("Location") != "/me" {
		t.Fatalf("POST /refresh = %d %q, want redirect to /me", resp.Code, resp.Header().Get("Location"))
	}
	if refreshed != 1 {
		t.Fatalf("refresh callback ran %d times, want 1", refreshed)
	}
}

// Without the option the route must not exist — deployment servers never
// expose an ingest trigger to their pages.
func TestRefreshRouteAbsentWithoutOption(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/norefresh.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pricing, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	web, err := NewWeb(store, pricing)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	web.Register(mux)

	resp := getFrom(t, mux, "192.0.2.9:53812", "/refresh")
	if resp.Code != http.StatusMethodNotAllowed && resp.Code != http.StatusNotFound {
		t.Fatalf("POST /refresh on deployment server = %d, want 404/405", resp.Code)
	}
}

// The web-local root lands on the personal dashboard; the deployment server
// keeps its leaderboard first page.
func TestLocalRootRedirectsToDashboard(t *testing.T) {
	_, mux := newLocalWeb(t, WithLocalRoot())
	resp := getFrom(t, mux, "127.0.0.1:53812", "/")
	if resp.Code != http.StatusSeeOther || resp.Header().Get("Location") != "/me" {
		t.Fatalf("local root: / = %d %q, want redirect to /me", resp.Code, resp.Header().Get("Location"))
	}

	store, err := OpenStore(t.TempDir() + "/board.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pricing, _ := LoadPricing("")
	plain, err := NewWeb(store, pricing)
	if err != nil {
		t.Fatal(err)
	}
	mux2 := http.NewServeMux()
	plain.Register(mux2)
	if resp := getFrom(t, mux2, "127.0.0.1:53812", "/"); resp.Code != http.StatusOK {
		t.Fatalf("deployment root: / = %d, want 200 leaderboard", resp.Code)
	}
}

// The loopback identity is cookie-free, so a page on another website could
// form-POST to 127.0.0.1 from the local browser: state-changing requests
// with a foreign Origin must be rejected, same-origin or absent Origin
// allowed.
func TestLocalModeRejectsCrossOriginPosts(t *testing.T) {
	_, mux := newLocalWeb(t, WithRefresh(func() error { return nil }))

	post := func(origin string) int {
		req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
		req.RemoteAddr = "127.0.0.1:53812"
		req.Host = "127.0.0.1:8787"
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		return resp.Code
	}

	if code := post("https://evil.example"); code != http.StatusForbidden {
		t.Fatalf("foreign Origin: POST /refresh = %d, want 403", code)
	}
	if code := post("http://127.0.0.1:8787"); code != http.StatusSeeOther {
		t.Fatalf("same-origin Origin: POST /refresh = %d, want 303", code)
	}
	if code := post(""); code != http.StatusSeeOther {
		t.Fatalf("absent Origin: POST /refresh = %d, want 303", code)
	}
}
