package cli

import (
	"strings"
	"testing"
	"time"
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
