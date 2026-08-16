package kimi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against a synthetic wire.jsonl fixture.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "kimi",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			root := t.TempDir()
			sessions := filepath.Join(root, "sessions", "day1", "sess-a")
			if err := os.MkdirAll(sessions, 0o755); err != nil {
				t.Fatalf("mkdir sessions: %v", err)
			}
			write := func(dir, name string, time int64, in, out uint64) {
				content := `{"type":"usage.record","usageScope":"turn","time":` + i64(time) + `,"model":"kimi-code/k2",` +
					`"usage":{"inputOther":` + u64(in) + `,"output":` + u64(out) + `}}`
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write wire file: %v", err)
				}
			}
			write(sessions, "wire.jsonl", 1786701600000, 100, 50)
			day2 := filepath.Join(root, "sessions", "day2", "sess-b")
			if err := os.MkdirAll(day2, 0o755); err != nil {
				t.Fatalf("mkdir day2: %v", err)
			}
			write(day2, "wire.jsonl", 1786788000000, 200, 20)
			t.Setenv(DataDirEnv, root)
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("kimi", shared)
			if !ok {
				t.Fatalf("kimi not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}

func u64(v uint64) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

func i64(v int64) string {
	negative := v < 0
	if negative {
		v = -v
	}
	digits := u64(uint64(v))
	if negative {
		return "-" + digits
	}
	return digits
}
