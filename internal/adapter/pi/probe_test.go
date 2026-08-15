package pi

import (
	"testing"

	"github.com/wujunwei/ccusage-go/internal/core"
)

func TestProbePiCost(t *testing.T) {
	shared := &core.SharedArgs{Mode: core.ModeAuto, Offline: true}
	entries, err := LoadEntries(LoadOptions{Shared: shared, Pricing: core.LoadWithOverrides(true, false, nil)})
	if err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if entries[i].Model != nil && *entries[i].Model == "[pi] deepseek-v4-flash" && entries[i].Cost != 0 {
			t.Logf("entry cost=%v missing=%v tokens in=%d out=%d cr=%d", entries[i].Cost,
				entries[i].MissingPricingModel, entries[i].Data.Message.Usage.InputTokens,
				entries[i].Data.Message.Usage.OutputTokens, entries[i].Data.Message.Usage.CacheReadInputTokens)
			return
		}
	}
	t.Log("no nonzero deepseek entry found")
}
