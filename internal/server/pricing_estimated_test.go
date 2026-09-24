package server

import (
	"os"
	"path/filepath"
	"testing"
)

// Ticket 08: a family-estimated card must never inherit a $0 cache rate from
// its ancestor — zeros are derived from the input price (read 0.1×, write5m
// 1.25×, write1h 2× per the existing derivation convention) so unknown models
// can't silently price 99%-cache-read usage at zero.
func TestFamilyFallbackDerivesZeroCacheRates(t *testing.T) {
	table, err := LoadPricing("")
	if err != nil {
		t.Fatal(err)
	}
	card, ok := table.resolve("glm-9.9-unheard-of")
	if !ok || card.Source != SourceFamily {
		t.Fatalf("family fallback missing: %+v ok=%v", card, ok)
	}
	if card.Input <= 0 {
		t.Fatalf("test premise: ancestor must carry a positive input rate, got %+v", card)
	}
	if d := card.CacheRead - 0.1*card.Input; d > 1e-9 || d < -1e-9 {
		t.Errorf("estimated cache read = %v, want 0.1×input (%v)", card.CacheRead, 0.1*card.Input)
	}
	if d := card.CacheWrite5m - 1.25*card.Input; d > 1e-9 || d < -1e-9 {
		t.Errorf("estimated cache write 5m = %v, want 1.25×input (%v)", card.CacheWrite5m, 1.25*card.Input)
	}
	if d := card.CacheWrite1h - 2*card.Input; d > 1e-9 || d < -1e-9 {
		t.Errorf("estimated cache write 1h = %v, want 2×input (%v)", card.CacheWrite1h, 2*card.Input)
	}
	// Official cards keep their real (possibly non-zero) cache rates untouched.
	off, ok := table.Card("glm-4.7")
	if !ok || off.Source != SourceSeed || off.CacheRead != 0.11 {
		t.Fatalf("official glm-4.7 card altered: %+v ok=%v", off, ok)
	}
	// Overrides win over every derivation.
	dir := t.TempDir()
	path := filepath.Join(dir, "prices.json")
	os.WriteFile(path, []byte(`{"models":{"glm-9.9-unheard-of":{
		"input": 1.0, "output": 2.0, "cacheRead": 0.3, "cacheWrite5m": 0.4, "cacheWrite1h": 0.5}}}`), 0o644)
	over, err := LoadPricing(path)
	if err != nil {
		t.Fatal(err)
	}
	card, _ = over.resolve("glm-9.9-unheard-of")
	if card.Source != SourceOverride || card.CacheRead != 0.3 || card.CacheWrite5m != 0.4 || card.CacheWrite1h != 0.5 {
		t.Fatalf("override rates not honored verbatim: %+v", card)
	}
}

// Ticket 08: the pricing override lives under the ADR 0007 config namespace
// and is auto-loaded by web/server when --pricing is not passed; absence is
// the normal no-override state.
func TestConfigPricingPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if _, ok := ConfigPricingPath(); ok {
		t.Fatal("no file yet: ConfigPricingPath must report absent")
	}
	full := filepath.Join(dir, "token-usage", "model-prices.json")
	os.MkdirAll(filepath.Dir(full), 0o755)
	os.WriteFile(full, []byte(`{"models":{}}`), 0o644)
	got, ok := ConfigPricingPath()
	if !ok || got != full {
		t.Fatalf("ConfigPricingPath = %q ok=%v, want %q true", got, ok, full)
	}
}
