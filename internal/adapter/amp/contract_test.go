package amp

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against the golden Amp fixture data.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "amp",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			t.Setenv(ampDataDirEnv, "../../../testdata/fixtures/amp")
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("amp", shared)
			if !ok {
				t.Fatalf("amp not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}
