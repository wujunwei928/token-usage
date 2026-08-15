package core

import (
	"os"
	"testing"
)

// resetAliasesEnvCacheForTest drops the memoized alias table so the next
// resolve re-reads the environment.
func resetAliasesEnvCacheForTest() {
	aliasesMu.Lock()
	aliasesMap = nil
	aliasesReady = false
	aliasesMu.Unlock()
}

// TOKEN_USAGE_MODEL_ALIASES wins; CCUSAGE_MODEL_ALIASES stays as the legacy
// fallback (ADR 0008).
func TestModelAliasesEnvDualRead(t *testing.T) {
	both := map[string]string{
		"TOKEN_USAGE_MODEL_ALIASES": `{"new-a":"new-b"}`,
		"CCUSAGE_MODEL_ALIASES":     `{"legacy-a":"legacy-b"}`,
	}
	cases := []struct {
		name string
		set  map[string]string
		in   string
		want string
	}{
		{"both set, TOKEN_USAGE wins", both, "new-a", "new-b"},
		{"both set, legacy ignored", both, "legacy-a", "legacy-a"},
		{"legacy only", map[string]string{"CCUSAGE_MODEL_ALIASES": `{"legacy-a":"legacy-b"}`}, "legacy-a", "legacy-b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k := range both {
				os.Unsetenv(k)
			}
			for k, v := range c.set {
				t.Setenv(k, v)
			}
			resetAliasesEnvCacheForTest()
			if got := ResolveModelName(c.in); got != c.want {
				t.Fatalf("ResolveModelName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
