package core

import (
	"fmt"
	"os"
	"testing"
)

// Local-only guard: when a real models.dev snapshot is present (/tmp), the
// official-provider selection must hold against live data.
func TestProbeRealModelsDev(t *testing.T) {
	body, err := os.ReadFile("/tmp/modelsdev.json")
	if err != nil {
		t.Skip("no local modelsdev snapshot")
	}
	for i := 0; i < 20; i++ {
		m := NewPricingMap()
		if _, ok := m.loadModelsDevJSONMissing(string(body)); !ok {
			t.Fatal("real doc must parse")
		}
		p := m.Find("glm-5.3")
		if p == nil {
			t.Fatal("glm-5.3 missing from live catalog")
		}
		floatEq(t, "glm-5.3 official input", p.Input, 1.4e-6)
		floatEq(t, "glm-5.3 official output", p.Output, 4.4e-6)
		floatEq(t, "glm-5.3 official cache read", p.CacheRead, 0.26e-6)
		if i == 0 {
			fmt.Printf("real snapshot: glm-5.3 in=$%.2f out=$%.2f read=$%.2f (per 1M)\n",
				p.Input*1e6, p.Output*1e6, p.CacheRead*1e6)
		}
	}
}
