package copilot

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against the golden Copilot OTel fixture.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "copilot",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			t.Setenv(CopilotOtelFileExporterPathEnv, "../../../testdata/fixtures/copilot/otel.jsonl")
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("copilot", shared)
			if !ok {
				t.Fatalf("copilot not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}
