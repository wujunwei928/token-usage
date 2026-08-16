package droid

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

func writeSettings(t *testing.T, dir, name, stamped string, in, out uint64) {
	t.Helper()
	content := `{"model":"Contract-Model","providerLock":"anthropic","providerLockTimestamp":"` +
		stamped + `","tokenUsage":{"inputTokens":` + uintToString(in) +
		`,"outputTokens":` + uintToString(out) + `}}`
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write settings fixture: %v", err)
	}
}

func uintToString(v uint64) string {
	if v == 0 {
		return "0"
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

// The shared adapter contract, run against the Droid fixture environment.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "droid",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			dir := t.TempDir()
			writeSettings(t, dir, "sess-a.settings.json", "2026-08-14T10:00:00.000Z", 100, 50)
			writeSettings(t, dir, "sess-b.settings.json", "2026-08-15T11:30:00.000Z", 200, 20)
			t.Setenv(DroidSessionsDirEnv, dir)
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("droid", shared)
			if !ok {
				t.Fatalf("droid not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}
