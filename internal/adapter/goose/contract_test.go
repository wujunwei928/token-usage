package goose

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against the golden Goose fixture data.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "goose",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			t.Setenv(GoosePathRootEnv, "../../../testdata/fixtures/goose")
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("goose", shared)
			if !ok {
				t.Fatalf("goose not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}
