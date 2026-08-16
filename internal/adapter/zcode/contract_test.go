package zcode

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against the SQLite fixture database.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "zcode",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			dir := t.TempDir()
			createFixtureDB(t, dir)
			t.Setenv(DataDirEnv, dir)
			shared := fixtureShared(core.ModeAuto)
			adapter, ok := common.BuildAdapter("zcode", shared)
			if !ok {
				t.Fatalf("zcode not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}
