package report

import (
	"os"
	"testing"
)

// The endpoint chain prefers TOKEN_USAGE_REPORT_* and falls back to the
// legacy CCUSAGE_REPORT_* names (ADR 0008); flags still beat both.
func TestResolveEndpointEnvDualRead(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())

	t.Run("both env set, TOKEN_USAGE wins", func(t *testing.T) {
		t.Setenv("TOKEN_USAGE_REPORT_SERVER", "http://new")
		t.Setenv("TOKEN_USAGE_REPORT_TOKEN", "new-token")
		t.Setenv("CCUSAGE_REPORT_SERVER", "http://legacy")
		t.Setenv("CCUSAGE_REPORT_TOKEN", "legacy-token")
		server, token, err := ResolveEndpoint([]string{"report"}, "", "")
		if err != nil || server != "http://new" || token != "new-token" {
			t.Fatalf("got server=%q token=%q err=%v, want http://new new-token", server, token, err)
		}
	})

	t.Run("legacy names still resolve", func(t *testing.T) {
		os.Unsetenv("TOKEN_USAGE_REPORT_SERVER")
		os.Unsetenv("TOKEN_USAGE_REPORT_TOKEN")
		t.Setenv("CCUSAGE_REPORT_SERVER", "http://legacy")
		t.Setenv("CCUSAGE_REPORT_TOKEN", "legacy-token")
		server, token, err := ResolveEndpoint([]string{"report"}, "", "")
		if err != nil || server != "http://legacy" || token != "legacy-token" {
			t.Fatalf("got server=%q token=%q err=%v, want http://legacy legacy-token", server, token, err)
		}
	})
}
