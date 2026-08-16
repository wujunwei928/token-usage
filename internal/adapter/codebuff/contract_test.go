package codebuff

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against the golden Codebuff fixture data.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "codebuff",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			t.Setenv(CodebuffDataDirEnv, "../../../testdata/fixtures/codebuff-data")
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("codebuff", shared)
			if !ok {
				t.Fatalf("codebuff not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}
