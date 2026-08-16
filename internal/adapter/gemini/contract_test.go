package gemini

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against a synthetic chat log fixture.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "gemini",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			root := t.TempDir()
			write := func(name, stamped string, in, out uint64) {
				content := `{"type":"gemini","model":"gemini-2.5-pro","timestamp":"` + stamped +
					`","tokens":{"input":` + u64(in) + `,"output":` + u64(out) + `}}`
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write chat log: %v", err)
				}
			}
			write("sess-a.jsonl", "2026-08-14T10:00:00.000Z", 100, 50)
			write("sess-b.jsonl", "2026-08-15T11:00:00.000Z", 200, 20)
			t.Setenv(DataDirEnv, root)
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("gemini", shared)
			if !ok {
				t.Fatalf("gemini not registered in the adapter registry")
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
