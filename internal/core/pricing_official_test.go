package core

import "testing"

// Ticket 10: same-name models.dev entries must resolve to the official
// provider's first-party rate deterministically, never to a random reseller
// or a $0 coding-plan relay.
func TestModelsDevOfficialProviderWins(t *testing.T) {
	doc := `{
        "zai": {"models": {"glm-x": {
            "cost": {"input": 1.4, "output": 4.4, "cache_read": 0.26}}}},
        "cheapy": {"models": {"glm-x": {
            "cost": {"input": 0.4, "output": 1.4, "cache_read": 0.06}}}},
        "zai-coding-plan": {"models": {"glm-x": {
            "cost": {"input": 0, "output": 0, "cache_read": 0}}}}
    }`
	// Map iteration is randomized; 50 fresh loads must all agree.
	for i := 0; i < 50; i++ {
		m := NewPricingMap()
		loaded, ok := m.loadModelsDevJSONMissing(doc)
		if !ok || loaded != 1 {
			t.Fatalf("load %d: loaded=%d ok=%v", i, loaded, ok)
		}
		p := mustFind(t, m, "glm-x")
		floatEq(t, "official input", p.Input, 1.4e-6)
		floatEq(t, "official output", p.Output, 4.4e-6)
		floatEq(t, "official cache read", p.CacheRead, 0.26e-6)
	}
}

func TestModelsDevDeterministicWithoutOfficial(t *testing.T) {
	doc := `{
        "aaa": {"models": {"mystery-9": {
            "cost": {"input": 2.0, "output": 2.0}}}},
        "bbb": {"models": {"mystery-9": {
            "cost": {"input": 1.0, "output": 1.0, "cache_read": 0.1}}}}
    }`
	for i := 0; i < 50; i++ {
		m := NewPricingMap()
		if _, ok := m.loadModelsDevJSONMissing(doc); !ok {
			t.Fatal("load failed")
		}
		p := mustFind(t, m, "mystery-9")
		// bbb wins on explicit cache_read despite aaa sorting first.
		floatEq(t, "explicit-read winner input", p.Input, 1.0e-6)
		if !p.CacheReadExplicit {
			t.Fatal("winner must be the explicit-cache-read listing")
		}
	}
	// Equal ranks → lexicographically smallest provider key.
	tie := `{
        "bbb": {"models": {"mystery-9": {"cost": {"input": 1.0, "output": 1.0}}}},
        "aaa": {"models": {"mystery-9": {"cost": {"input": 2.0, "output": 2.0}}}}
    }`
	for i := 0; i < 50; i++ {
		m := NewPricingMap()
		m.loadModelsDevJSONMissing(tie)
		p := mustFind(t, m, "mystery-9")
		floatEq(t, "tie-break winner input", p.Input, 2.0e-6)
	}
}

func TestModelsDevCostlessListingsSkipped(t *testing.T) {
	doc := `{
        "broken": {"models": {"glm-y": {"cost": {"input": 0, "output": 0}}}},
        "nocost": {"models": {"glm-z": {}}},
        "zai": {"models": {"glm-free": {"cost": {"input": 0, "output": 0}}}}
    }`
	m := NewPricingMap()
	loaded, ok := m.loadModelsDevJSONMissing(doc)
	if !ok {
		t.Fatal("doc should parse")
	}
	if m.Find("glm-y") != nil || m.Find("glm-z") != nil {
		t.Fatal("reseller $0 metering and costless listings must be skipped")
	}
	if loaded != 1 || m.Find("glm-free") == nil {
		t.Fatal("official-provider $0 free tier must be kept (a genuine list price)")
	}
}

func TestOfficialProviderFor(t *testing.T) {
	for model, want := range map[string]string{
		"glm-5.3":        "zai",
		"GLM-5.3-Flash":  "zai",
		"qwen3.7-max":    "alibaba",
		"claude-opus-5":  "anthropic",
		"gemini-3-pro":   "google",
		"o4-mini-latest": "openai",
		"olmo-2":         "", // prefix "o" deliberately absent: no false positives
	} {
		got, ok := officialProviderFor(model)
		if want == "" && ok {
			t.Fatalf("%s: unexpected official provider %q", model, got)
		}
		if want != "" && (!ok || got != want) {
			t.Fatalf("%s: official provider = %q,%v want %q", model, got, ok, want)
		}
	}
}
