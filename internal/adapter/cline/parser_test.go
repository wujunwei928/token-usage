package cline

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

// writeSessionDir lays down one session directory with an optional manifest
// (<id>.json) and one messages file (<id>.messages.json).
func writeSessionDir(t *testing.T, root, id, manifest, messages string) {
	t.Helper()
	dir := filepath.Join(root, "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(manifest), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, id+".messages.json"), []byte(messages), 0o644); err != nil {
		t.Fatalf("write messages: %v", err)
	}
}

// inputTokens already contains the cache buckets, so input excludes them
// (clamped at zero) while the cache classes stay separate.
func TestReadFilePairSubtractsCacheFromInput(t *testing.T) {
	root := t.TempDir()
	writeSessionDir(t, root, "s1", "",
		`{"sessionId":"s1","messages":[{"id":"m1","role":"assistant","ts":1000,`+
			`"metrics":{"inputTokens":100,"outputTokens":10,"cacheReadTokens":30,"cacheWriteTokens":20}}]}`)
	entries := readFilePair(filepath.Join(root, "sessions", "s1", "s1.messages.json"))
	if len(entries) != 1 {
		t.Fatalf("entries = %d want 1", len(entries))
	}
	usage := entries[0].Usage
	if usage.InputTokens != 50 || usage.OutputTokens != 10 ||
		usage.CacheCreationInputTokens != 20 || usage.CacheReadInputTokens != 30 {
		t.Fatalf("usage = %+v want input 50/output 10/cache-create 20/cache-read 30", usage)
	}
	if total := core.TotalUsageTokens(usage); total != 110 {
		t.Fatalf("total = %d want 110 (= inputTokens + outputTokens)", total)
	}
}

// A cache total above inputTokens clamps input at zero instead of underflowing.
func TestReadFilePairCacheOverflowClampsInput(t *testing.T) {
	root := t.TempDir()
	writeSessionDir(t, root, "s1", "",
		`{"sessionId":"s1","messages":[{"id":"m1","role":"assistant","ts":1000,`+
			`"metrics":{"inputTokens":5,"outputTokens":0,"cacheReadTokens":10,"cacheWriteTokens":0}}]}`)
	entries := readFilePair(filepath.Join(root, "sessions", "s1", "s1.messages.json"))
	if len(entries) != 1 {
		t.Fatalf("entries = %d want 1", len(entries))
	}
	if entries[0].Usage.InputTokens != 0 || entries[0].Usage.CacheReadInputTokens != 10 {
		t.Fatalf("usage = %+v want input 0/cache-read 10", entries[0].Usage)
	}
}

// metrics.cost behaves like the reference's extract_non_negative_finite_f64:
// numbers and numeric strings count as provider-reported costUSD; negative,
// NaN, or unparsable values count as absent. A zero-token message survives
// only when it reports a cost.
func TestReadFilePairCostField(t *testing.T) {
	root := t.TempDir()
	writeSessionDir(t, root, "s1", "",
		`{"sessionId":"s1","messages":[`+
			`{"id":"m1","role":"assistant","ts":1000,"metrics":{"inputTokens":0,"outputTokens":0,"cost":0.003}},`+
			`{"id":"m2","role":"assistant","ts":2000,"metrics":{"inputTokens":1,"outputTokens":2,"cost":"0.5"}},`+
			`{"id":"m3","role":"assistant","ts":3000,"metrics":{"inputTokens":3,"outputTokens":4,"cost":"NaN"}},`+
			`{"id":"m4","role":"assistant","ts":4000,"metrics":{"inputTokens":5,"outputTokens":6,"cost":-1}},`+
			`{"id":"m5","role":"assistant","ts":5000,"metrics":{"inputTokens":0,"outputTokens":0}}]}`)
	entries := readFilePair(filepath.Join(root, "sessions", "s1", "s1.messages.json"))
	if len(entries) != 4 {
		t.Fatalf("entries = %d want 4 (m5 drops: zero tokens, no cost)", len(entries))
	}
	wantCosts := map[string]*float64{
		"m1": ptrOf(0.003),
		"m2": ptrOf(0.5),
		"m3": nil,
		"m4": nil,
	}
	for _, entry := range entries {
		want := wantCosts[entry.ID]
		switch {
		case want == nil && entry.CostUSD != nil:
			t.Errorf("%s costUSD = %v want nil", entry.ID, *entry.CostUSD)
		case want != nil && entry.CostUSD == nil:
			t.Errorf("%s costUSD = nil want %v", entry.ID, *want)
		case want != nil && *entry.CostUSD != *want:
			t.Errorf("%s costUSD = %v want %v", entry.ID, *entry.CostUSD, *want)
		}
	}
}

// The manifest seeds the session id, model, provider, and workspace;
// per-message modelInfo and the file's own sessionId override it, and
// workspace_root wins over cwd.
func TestReadFilePairManifestFallbacks(t *testing.T) {
	root := t.TempDir()
	writeSessionDir(t, root, "seeded",
		`{"session_id":"manifest-id","provider":"prov-x","model":"fallback-model",`+
			`"cwd":"/old/root","workspace_root":"/new/root"}`,
		`{"messages":[{"id":"m1","role":"assistant","ts":1000,`+
			`"metrics":{"inputTokens":10,"outputTokens":1}}]}`)
	entries := readFilePair(filepath.Join(root, "sessions", "seeded", "seeded.messages.json"))
	if len(entries) != 1 {
		t.Fatalf("entries = %d want 1", len(entries))
	}
	entry := entries[0]
	if entry.SessionID != "manifest-id" {
		t.Errorf("sessionID = %q want manifest-id", entry.SessionID)
	}
	if entry.Model != "fallback-model" || entry.Provider != "prov-x" {
		t.Errorf("model/provider = %q/%q want fallback-model/prov-x", entry.Model, entry.Provider)
	}
	if entry.CWD != "/new/root" {
		t.Errorf("cwd = %q want /new/root (workspace_root wins over cwd)", entry.CWD)
	}

	// Without a manifest the file's sessionId and stem carry the identity,
	// the model degrades to unknown, and the workspace stays empty.
	writeSessionDir(t, root, "bare", "",
		`{"sessionId":"file-id","messages":[{"id":"m2","role":"assistant","ts":1000,`+
			`"metrics":{"inputTokens":10,"outputTokens":1}}]}`)
	entries = readFilePair(filepath.Join(root, "sessions", "bare", "bare.messages.json"))
	if len(entries) != 1 {
		t.Fatalf("entries = %d want 1", len(entries))
	}
	entry = entries[0]
	if entry.SessionID != "file-id" {
		t.Errorf("sessionID = %q want file-id", entry.SessionID)
	}
	if entry.Model != "unknown" || entry.Provider != "" {
		t.Errorf("model/provider = %q/%q want unknown/empty", entry.Model, entry.Provider)
	}
	if entry.CWD != "" {
		t.Errorf("cwd = %q want empty", entry.CWD)
	}
}

func ptrOf(v float64) *float64 { return &v }

// A provider-reported cost wins in auto mode; calculate always prices from
// tokens; a costless entry with no priced candidate flags missing pricing.
func TestCostModesAndMissingPricing(t *testing.T) {
	pricing := core.NewPricingMap()
	pricing.LoadJSON(`{"priced-model":{"input_cost_per_token":0.001,"output_cost_per_token":0.002}}`)
	withCost := clineEntry{
		Model:   "unpriced-model",
		Usage:   core.TokenUsageRaw{InputTokens: 100, OutputTokens: 50},
		CostUSD: ptrOf(0.75),
	}
	if got := clineCost(withCost, pricing, core.ModeAuto); got != 0.75 {
		t.Errorf("auto cost = %v want 0.75 (provider-reported)", got)
	}
	if got := clineCost(withCost, pricing, core.ModeCalculate); got != 0 {
		t.Errorf("calculate cost = %v want 0 (unpriced model prices to $0)", got)
	}
	if got := clineMissingPricing(withCost, core.ModeAuto, pricing); got != nil {
		t.Errorf("auto missing pricing = %v want nil (costUSD present)", *got)
	}

	withoutCost := withCost
	withoutCost.CostUSD = nil
	if got := clineMissingPricing(withoutCost, core.ModeAuto, pricing); got == nil || *got != "unpriced-model" {
		t.Errorf("missing pricing = %v want unpriced-model", got)
	}

	priced := clineEntry{
		Model:   "priced-model",
		Usage:   core.TokenUsageRaw{InputTokens: 100, OutputTokens: 50},
		CostUSD: ptrOf(9.0),
	}
	if got := clineCost(priced, pricing, core.ModeAuto); got != 9.0 {
		t.Errorf("auto cost = %v want 9.0 (costUSD beats token pricing)", got)
	}
	if got := clineCost(priced, pricing, core.ModeCalculate); math.Abs(got-0.2) > 1e-12 {
		t.Errorf("calculate cost = %v want 0.2", got)
	}

	// The provider-qualified spelling resolves when the bare id does not.
	qualified := clineEntry{
		Model:    "priced-model",
		Provider: "other",
		Usage:    core.TokenUsageRaw{InputTokens: 100, OutputTokens: 50},
	}
	if got := clineMissingPricing(qualified, core.ModeAuto, pricing); got != nil {
		t.Errorf("qualified missing pricing = %v want nil (bare id matches)", *got)
	}
}
