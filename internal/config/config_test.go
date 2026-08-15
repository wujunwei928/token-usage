package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/wujunwei/ccusage-go/internal/core"
)

// mustParse parses strict JSON with number preservation, mirroring the loader.
func mustParse(t *testing.T, doc string) map[string]any {
	t.Helper()
	value, ok := parseConfigObject(doc)
	if !ok {
		t.Fatalf("config did not parse: %s", doc)
	}
	return value
}

func strPtr(v string) *string { return &v }

func TestParseConfigObjectStrictness(t *testing.T) {
	if _, ok := parseConfigObject(`{"a":1}`); !ok {
		t.Fatal("object config should parse")
	}
	if _, ok := parseConfigObject(`{"a":1} trailing`); ok {
		t.Fatal("trailing garbage should reject the config")
	}
	if _, ok := parseConfigObject(`[1,2,3]`); ok {
		t.Fatal("non-object config should be skipped")
	}
	if _, ok := parseConfigObject(`"string"`); ok {
		t.Fatal("string config should be skipped")
	}
	if _, ok := parseConfigObject(`nope`); ok {
		t.Fatal("malformed JSON should be skipped")
	}
}

func TestDiscoverConfigPathsOrder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/one, /tmp/two")
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	paths := DiscoverConfigPaths()
	want := []string{
		filepath.Join(dir, ".ccusage", "ccusage.json"),
		filepath.Join("/tmp/one", "ccusage.json"),
		filepath.Join("/tmp/two", "ccusage.json"),
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %s, want %s", i, paths[i], want[i])
		}
	}

	// CLAUDE_CONFIG_DIR replaces the home lookup entirely, like the reference.
	os.Unsetenv("CLAUDE_CONFIG_DIR")
	paths = DiscoverConfigPaths()
	want = []string{
		filepath.Join(dir, ".ccusage", "ccusage.json"),
		filepath.Join(home, ".config", "claude", "ccusage.json"),
		filepath.Join(home, ".claude", "ccusage.json"),
	}
	if len(paths) != len(want) {
		t.Fatalf("paths without CLAUDE_CONFIG_DIR = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %s, want %s", i, paths[i], want[i])
		}
	}
}

func TestLoadConfigValueDiscoveryOrder(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	agentDir := filepath.Join(root, "agent")
	homeDir := filepath.Join(root, "home")
	for _, dir := range []string{filepath.Join(cwd, ".ccusage"), agentDir, filepath.Join(homeDir, ".claude")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(cwd)
	t.Setenv("HOME", homeDir)
	// Start without CLAUDE_CONFIG_DIR so the home fallback is in play.
	os.Unsetenv("CLAUDE_CONFIG_DIR")

	write := func(path, order string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`{"defaults":{"order":"`+order+`"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	orderOf := func(value map[string]any) string {
		options := SharedOptionsFromMap(objectAt(value, "defaults"))
		if options.Order == nil {
			return ""
		}
		if *options.Order == core.OrderDesc {
			return "desc"
		}
		return "asc"
	}

	// Nothing discovered.
	if value := LoadConfigValue(""); value != nil {
		t.Fatalf("expected no config, got %v", value)
	}

	// Home dir only.
	homeConfig := filepath.Join(homeDir, ".claude", "ccusage.json")
	write(homeConfig, "desc")
	if got := orderOf(LoadConfigValue("")); got != "desc" {
		t.Fatalf("home config not discovered, order=%q", got)
	}

	// CLAUDE_CONFIG_DIR beats home.
	t.Setenv("CLAUDE_CONFIG_DIR", agentDir)
	agentConfig := filepath.Join(agentDir, "ccusage.json")
	write(agentConfig, "asc")
	if got := orderOf(LoadConfigValue("")); got != "asc" {
		t.Fatalf("agent dir should beat home, order=%q", got)
	}

	// Working dir beats CLAUDE_CONFIG_DIR.
	cwdConfig := filepath.Join(cwd, ".ccusage", "ccusage.json")
	write(cwdConfig, "desc")
	if got := orderOf(LoadConfigValue("")); got != "desc" {
		t.Fatalf("cwd config should beat agent dir, order=%q", got)
	}

	// An explicit path bypasses discovery entirely, even when missing.
	if value := LoadConfigValue(filepath.Join(root, "missing.json")); value != nil {
		t.Fatalf("missing explicit path should yield no config, got %v", value)
	}
	// ...and when it targets a file that is not a JSON object.
	nonObject := filepath.Join(root, "array.json")
	if err := os.WriteFile(nonObject, []byte(`[1]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if value := LoadConfigValue(nonObject); value != nil {
		t.Fatalf("non-object explicit path should yield no config, got %v", value)
	}
}

func TestScanConfigPath(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"daily", "--config", "/tmp/a.json"}, "/tmp/a.json"},
		{[]string{"daily", "--config=/tmp/a.json"}, "/tmp/a.json"},
		{[]string{"daily", "--config="}, ""},
		{[]string{"daily", "--config"}, ""},
		{[]string{"daily", "--mode", "auto", "--config", "/tmp/a.json"}, "/tmp/a.json"},
		{[]string{"daily", "--offline"}, ""},
	}
	for _, tc := range cases {
		if got := ScanConfigPath(tc.args); got != tc.want {
			t.Errorf("ScanConfigPath(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestDetectConfigCommand(t *testing.T) {
	cases := []struct {
		args []string
		want CommandInfo
	}{
		{nil, CommandInfo{Raw: "daily", Report: "daily"}},
		{[]string{"daily"}, CommandInfo{Raw: "daily", Report: "daily"}},
		{[]string{"claude", "daily"}, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"}},
		{[]string{"claude:daily"}, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"}},
		{[]string{"claude"}, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"}},
		{[]string{"claude", "weekly"}, CommandInfo{Raw: "claude weekly", Agent: "claude", Report: "weekly"}},
		{[]string{"claude", "bogus"}, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"}},
		{[]string{"--json", "--mode", "calculate", "claude", "daily"}, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"}},
		{[]string{"--json", "daily"}, CommandInfo{Raw: "daily", Report: "daily"}},
		{[]string{"--order=desc", "weekly"}, CommandInfo{Raw: "weekly", Report: "weekly"}},
		{[]string{"blocks"}, CommandInfo{Raw: "blocks", Report: "blocks"}},
		{[]string{"grok", "session"}, CommandInfo{Raw: "grok session", Agent: "grok", Report: "session"}},
	}
	for _, tc := range cases {
		if got := DetectConfigCommand(tc.args); got != tc.want {
			t.Errorf("DetectConfigCommand(%v) = %+v, want %+v", tc.args, got, tc.want)
		}
	}
}

// The option-map order was verified against the reference binary: for
// `claude daily`, later maps override earlier ones.
func TestOptionMapsPrecedence(t *testing.T) {
	root := mustParse(t, `{
		"defaults": {"order": "desc"},
		"commands": {
			"claude daily": {"order": "asc"},
			"daily": {"breakdown": true},
			"claude:daily": {"offline": true},
			"weekly": {"order": "desc"}
		},
		"claude": {
			"defaults": {"order": "desc"},
			"commands": {"daily": {"order": "asc"}}
		}
	}`)
	maps := optionMapsFor(root, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"})
	if len(maps) != 6 {
		t.Fatalf("claude daily should see 6 maps, got %d", len(maps))
	}
	shared := &core.SharedArgs{}
	for _, m := range maps {
		applySharedOptions(shared, SharedOptionsFromMap(m), func(string) bool { return false })
	}
	if shared.Order != core.OrderAsc {
		t.Errorf("claude.commands.daily should win, got order %v", shared.Order)
	}
	if !shared.Breakdown {
		t.Error("commands.daily.breakdown should apply")
	}
	if !shared.Offline {
		t.Error("commands['claude:daily'].offline should apply")
	}

	// The all-agent daily report reads defaults + commands.daily only.
	maps = optionMapsFor(root, CommandInfo{Raw: "daily", Report: "daily"})
	if len(maps) != 2 {
		t.Fatalf("daily should see 2 maps, got %d", len(maps))
	}

	// Unknown agents keep shared options through their own section.
	grok := mustParse(t, `{
		"grok": {"defaults": {"offline": true}, "commands": {"session": {"json": true}}}
	}`)
	maps = optionMapsFor(grok, CommandInfo{Raw: "grok session", Agent: "grok", Report: "session"})
	shared = &core.SharedArgs{}
	for _, m := range maps {
		applySharedOptions(shared, SharedOptionsFromMap(m), func(string) bool { return false })
	}
	if !shared.Offline || !shared.JSON {
		t.Errorf("grok namespace should keep shared options, got %+v", shared)
	}
}

func TestSharedOptionsTypeStrictness(t *testing.T) {
	root := mustParse(t, `{
		"defaults": {
			"since": "2026-01-01",
			"json": "not a bool",
			"mode": 42,
			"order": "sideways",
			"debugSamples": 5.0,
			"breakdown": true,
			"timezone": "Asia/Tokyo",
			"compact": 1,
			"noCost": true,
			"unknownKey": {"nested": true}
		}
	}`)
	opts := SharedOptionsFromMap(objectAt(root, "defaults"))
	if opts.Since == nil || *opts.Since != "2026-01-01" {
		t.Errorf("since = %v, want 2026-01-01", opts.Since)
	}
	if opts.JSON != nil {
		t.Error(`json: "not a bool" should be ignored`)
	}
	if opts.Mode != nil {
		t.Error("mode: 42 should be ignored")
	}
	if opts.Order != nil {
		t.Error(`order: "sideways" should be ignored`)
	}
	if opts.DebugSamples != nil {
		t.Error("debugSamples: 5.0 should be ignored (float literal)")
	}
	if opts.Breakdown == nil || !*opts.Breakdown {
		t.Error("breakdown should parse")
	}
	if opts.Compact != nil {
		t.Error("compact: 1 should be ignored")
	}
	if opts.NoCost == nil || !*opts.NoCost {
		t.Error("noCost should parse")
	}
	if opts.Timezone == nil || *opts.Timezone != "Asia/Tokyo" {
		t.Errorf("timezone = %v", opts.Timezone)
	}
}

func TestReportSpecificOptions(t *testing.T) {
	root := mustParse(t, `{
		"commands": {
			"daily": {"instances": true, "project": "proj-a", "projectAliases": "a=B"},
			"weekly": {"startOfWeek": "monday"},
			"blocks": {"active": true, "recent": false, "tokenLimit": "500000", "sessionLength": 6.5},
			"statusline": {
				"offline": false, "noOffline": true, "visualBurnRate": "emoji-text",
				"costSource": "both", "cache": false, "noCache": true, "refreshInterval": 3,
				"contextLowThreshold": 45, "contextMediumThreshold": 300, "timezone": "UTC",
				"debug": true,
				"modelLabelAliases": {"arn:x": "opus", "bad": 1}
			}
		}
	}`)
	commands := objectAt(root, "commands")

	daily := DailySpecificOptionsFromMap(objectAt(commands, "daily"))
	if daily.Instances == nil || !*daily.Instances || daily.Project == nil || *daily.Project != "proj-a" ||
		daily.ProjectAliases == nil || *daily.ProjectAliases != "a=B" {
		t.Errorf("daily options = %+v", daily)
	}

	weekly := WeeklySpecificOptionsFromMap(objectAt(commands, "weekly"))
	if weekly.StartOfWeek == nil || *weekly.StartOfWeek != core.Monday {
		t.Errorf("startOfWeek = %+v", weekly.StartOfWeek)
	}

	blocks := BlocksSpecificOptionsFromMap(objectAt(commands, "blocks"))
	if blocks.Active == nil || !*blocks.Active || blocks.TokenLimit == nil || *blocks.TokenLimit != "500000" ||
		blocks.SessionLength == nil || *blocks.SessionLength != 6.5 {
		t.Errorf("blocks options = %+v", blocks)
	}

	statusline := StatuslineSpecificOptionsFromMap(objectAt(commands, "statusline"))
	if statusline.Offline == nil || *statusline.Offline {
		t.Error("statusline offline should be false")
	}
	if statusline.VisualBurnRate == nil || *statusline.VisualBurnRate != VisualBurnRateEmojiText {
		t.Errorf("visualBurnRate = %+v", statusline.VisualBurnRate)
	}
	if statusline.CostSource == nil || *statusline.CostSource != CostSourceBoth {
		t.Errorf("costSource = %+v", statusline.CostSource)
	}
	if statusline.ContextLowThreshold == nil || *statusline.ContextLowThreshold != 45 {
		t.Errorf("contextLowThreshold = %+v", statusline.ContextLowThreshold)
	}
	// 300 does not fit u8; from_map keeps u64 and apply drops it, mirroring
	// u8::try_from(...).ok().
	if statusline.ContextMediumThreshold == nil || *statusline.ContextMediumThreshold != 300 {
		t.Errorf("contextMediumThreshold = %+v", statusline.ContextMediumThreshold)
	}
	args := NewStatuslineArgs()
	for _, m := range optionMapsFor(root, CommandInfo{Raw: "statusline", Report: "statusline"}) {
		applyStatuslineOptions(&args, StatuslineSpecificOptionsFromMap(m), func(string) bool { return false })
	}
	if args.ContextMediumThreshold != 80 {
		t.Errorf("out-of-range threshold must stay at default, got %d", args.ContextMediumThreshold)
	}
	if args.ContextLowThreshold != 45 {
		t.Errorf("contextLowThreshold should apply, got %d", args.ContextLowThreshold)
	}
	if statusline.ModelLabelAliases == nil || statusline.ModelLabelAliases["arn:x"] != "opus" {
		t.Errorf("modelLabelAliases = %+v", statusline.ModelLabelAliases)
	}
	if _, ok := statusline.ModelLabelAliases["bad"]; ok {
		t.Error("non-string alias values should be skipped")
	}
}

func TestAgentSpecificOptions(t *testing.T) {
	root := mustParse(t, `{
		"codex": {"defaults": {"speed": "fast"}},
		"pi": {"defaults": {"piPath": "/tmp/pi"}},
		"openclaw": {"defaults": {"openClawPath": "/tmp/openclaw"}}
	}`)
	agent := func(agent string) *AgentArgs {
		t.Helper()
		args := &AgentArgs{}
		for _, m := range optionMapsFor(root, CommandInfo{Raw: agent + " daily", Agent: agent, Report: "daily"}) {
			applyAgentOptions(args, m, func(string) bool { return false })
		}
		return args
	}
	if got := agent("codex"); got.CodexSpeed != CodexSpeedFast {
		t.Errorf("codex speed = %v", got.CodexSpeed)
	}
	if got := agent("pi"); got.PIPath == nil || *got.PIPath != "/tmp/pi" {
		t.Errorf("pi path = %v", got.PIPath)
	}
	if got := agent("openclaw"); got.OpenClawPath == nil || *got.OpenClawPath != "/tmp/openclaw" {
		t.Errorf("openclaw path = %v", got.OpenClawPath)
	}
}

func TestPricingOverrideParsing(t *testing.T) {
	good := mustParse(t, `{
		"defaults": {"pricingOverrides": {
			"model-a": {
				"inputCostPerToken": 3e-6,
				"outputCostPerToken": 0,
				"maxInputTokens": 1000000,
				"fastMultiplier": 2,
				"unknownField": "ignored"
			}
		}}
	}`)
	overrides := SharedOptionsFromMap(objectAt(good, "defaults")).PricingOverrides
	entry, ok := overrides["model-a"]
	if !ok {
		t.Fatalf("override missing: %+v", overrides)
	}
	if entry.InputCostPerToken == nil || *entry.InputCostPerToken != 3e-6 {
		t.Errorf("inputCostPerToken = %v", entry.InputCostPerToken)
	}
	if entry.OutputCostPerToken == nil || *entry.OutputCostPerToken != 0 {
		t.Errorf("outputCostPerToken = %v", entry.OutputCostPerToken)
	}
	if entry.MaxInputTokens == nil || *entry.MaxInputTokens != 1_000_000 {
		t.Errorf("maxInputTokens = %v", entry.MaxInputTokens)
	}
	if entry.FastMultiplier == nil || *entry.FastMultiplier != 2 {
		t.Errorf("fastMultiplier = %v", entry.FastMultiplier)
	}

	dropped := []string{
		`{"defaults":{"pricingOverrides":{"m":{"inputCostPerToken":"oops"}}}}`,
		`{"defaults":{"pricingOverrides":{"m":{"maxInputTokens":1.5}}}}`,
		`{"defaults":{"pricingOverrides":{"m":5}}}`,
		`{"defaults":{"pricingOverrides":{"m":{"inputCostPerToken":true}}}}`,
		`{"defaults":{"pricingOverrides":"nope"}}`,
	}
	for _, doc := range dropped {
		root := mustParse(t, doc)
		if overrides := SharedOptionsFromMap(objectAt(root, "defaults")).PricingOverrides; overrides != nil {
			t.Errorf("one bad entry should drop the whole map for %s", doc)
		}
	}
}

func TestMergePricingOverridesFieldLevel(t *testing.T) {
	current := map[string]core.PricingOverride{}
	input := 2.5e-6
	output := 1.5e-5
	current["[pi] gpt-5.4"] = core.PricingOverride{
		InputCostPerToken:  &input,
		OutputCostPerToken: &output,
	}
	max := uint64(1_000_000)
	incoming := map[string]ConfigPricingOverride{
		"[pi] gpt-5.4": {MaxInputTokens: &max},
	}
	MergePricingOverrides(current, incoming)
	merged := current["[pi] gpt-5.4"]
	if merged.InputCostPerToken == nil || *merged.InputCostPerToken != 2.5e-6 {
		t.Errorf("parent input preserved, got %v", merged.InputCostPerToken)
	}
	if merged.OutputCostPerToken == nil || *merged.OutputCostPerToken != 1.5e-5 {
		t.Errorf("parent output preserved, got %v", merged.OutputCostPerToken)
	}
	if merged.MaxInputTokens == nil || *merged.MaxInputTokens != 1_000_000 {
		t.Errorf("child maxInputTokens applied, got %v", merged.MaxInputTokens)
	}

	// Child overrides one field, leaves the others alone.
	cacheRead := 3e-7
	current = map[string]core.PricingOverride{
		"model-a": {InputCostPerToken: &input, CacheReadInputTokenCost: &cacheRead},
	}
	newInput := 2e-6
	incoming = map[string]ConfigPricingOverride{
		"model-a": {InputCostPerToken: &newInput},
	}
	MergePricingOverrides(current, incoming)
	merged = current["model-a"]
	if *merged.InputCostPerToken != 2e-6 {
		t.Errorf("input overridden, got %v", *merged.InputCostPerToken)
	}
	if *merged.CacheReadInputTokenCost != 3e-7 {
		t.Errorf("cache read preserved, got %v", *merged.CacheReadInputTokenCost)
	}
}

func TestPricingOverridesMergeAcrossSections(t *testing.T) {
	root := mustParse(t, `{
		"defaults": {"pricingOverrides": {"m": {"inputCostPerToken": 1e-6}}},
		"claude": {"commands": {"daily": {"pricingOverrides": {"m": {"outputCostPerToken": 2e-6}}}}}
	}`)
	shared := &core.SharedArgs{}
	for _, m := range optionMapsFor(root, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"}) {
		applySharedOptions(shared, SharedOptionsFromMap(m), func(string) bool { return false })
	}
	override, ok := shared.PricingOverrides["m"]
	if !ok {
		t.Fatal("pricing override missing")
	}
	if override.InputCostPerToken == nil || *override.InputCostPerToken != 1e-6 {
		t.Errorf("defaults input preserved, got %v", override.InputCostPerToken)
	}
	if override.OutputCostPerToken == nil || *override.OutputCostPerToken != 2e-6 {
		t.Errorf("section output merged, got %v", override.OutputCostPerToken)
	}
}

func TestNamedPIStoreErrors(t *testing.T) {
	cases := []struct {
		doc  string
		want string
	}{
		{`{"pi":{"stores":"nope"}}`, "Invalid ccusage config: pi.stores must be an array"},
		{`{"pi":{"stores":["nope"]}}`, "Invalid ccusage config: pi.stores[0] must contain string fields 'name' and 'path'"},
		{`{"pi":{"stores":[{"name":"omp"}]}}`, "Invalid ccusage config: pi.stores[0] must contain string fields 'name' and 'path'"},
		{`{"pi":{"stores":[{"path":"/tmp"}]}}`, "Invalid ccusage config: pi.stores[0] must contain string fields 'name' and 'path'"},
		{`{"pi":{"stores":[{"name":"omp","path":"  "}]}}`, "Invalid ccusage config: pi.stores[0] ('omp'): path must be a non-empty string"},
		{`{"pi":{"stores":[{"name":"Omp","path":"/tmp"}]}}`, "Invalid ccusage config: pi.stores[0].name must match ^[a-z][a-z0-9_-]{0,31}$"},
		{`{"pi":{"stores":[{"name":"pi","path":"/tmp"}]}}`, "Invalid ccusage config: pi.stores name 'pi' collides with a built-in agent"},
		{`{"pi":{"stores":[{"name":"all","path":"/tmp"}]}}`, "Invalid ccusage config: pi.stores name 'all' collides with a built-in agent"},
		{`{"pi":{"stores":[{"name":"omp","path":"/a"},{"name":"omp","path":"/b"}]}}`, "Invalid ccusage config: duplicate pi.stores name 'omp'"},
	}
	for _, tc := range cases {
		_, err := ParseNamedPIStores(mustParse(t, tc.doc))
		if err == nil {
			t.Errorf("%s: expected error", tc.doc)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.doc, err.Error(), tc.want)
		}
	}

	stores, err := ParseNamedPIStores(mustParse(t, `{
		"pi": {"stores": [{"name": "omp", "path": "~/.omp/agent/sessions"}, {"name": "o3-fork", "path": "/tmp"}]}
	}`))
	if err != nil {
		t.Fatalf("valid stores rejected: %v", err)
	}
	if len(stores) != 2 || stores[0].Name != "omp" || stores[0].Path != "~/.omp/agent/sessions" || stores[1].Name != "o3-fork" {
		t.Errorf("stores = %+v", stores)
	}

	if stores, err := ParseNamedPIStores(mustParse(t, `{"pi":{"stores":[]}}`)); err != nil || len(stores) != 0 {
		t.Errorf("empty stores should be a no-op, got %+v err %v", stores, err)
	}
	if stores, err := ParseNamedPIStores(mustParse(t, `{"defaults":{"json":true}}`)); err != nil || stores != nil {
		t.Errorf("missing pi section should be a no-op, got %+v err %v", stores, err)
	}
}

func TestConfigErrorOnlyForAllAgentReports(t *testing.T) {
	bad := mustParse(t, `{"pi":{"stores":[{"name":"omp"}]}}`)
	agentCtx := FromValue(bad, CommandInfo{Raw: "claude daily", Agent: "claude", Report: "daily"})
	if err := agentCtx.ConfigError(); err != nil {
		t.Errorf("claude daily must ignore pi.stores errors, got %v", err)
	}
	blocksCtx := FromValue(bad, CommandInfo{Raw: "blocks", Report: "blocks"})
	if err := blocksCtx.ConfigError(); err != nil {
		t.Errorf("blocks must ignore pi.stores errors, got %v", err)
	}
	dailyCtx := FromValue(bad, CommandInfo{Raw: "daily", Report: "daily"})
	err := dailyCtx.ConfigError()
	if err == nil {
		t.Fatal("all-agent daily must surface pi.stores errors")
	}
	if !strings.HasPrefix(err.Error(), "Invalid ccusage config: ") {
		t.Errorf("error text = %q", err.Error())
	}
}

func TestApplyConfigFillsOnlyUnsetValues(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ccusage.json")
	err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"since": "2026-01-01",
			"until": "2026-01-31",
			"json": true,
			"mode": "calculate",
			"debugSamples": 9,
			"order": "desc",
			"breakdown": true,
			"noCost": true,
			"timezone": "Asia/Tokyo",
			"jq": ".totals"
		},
		"commands": {"daily": {"instances": true, "project": "proj-a"}}
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	previous := os.Args
	os.Args = []string{"ccusage", "claude", "daily", "--config", configPath}
	defer func() { os.Args = previous }()

	shared := &core.SharedArgs{}
	daily := &DailyArgs{}
	ApplyConfig("daily", "claude", shared, nil, daily)

	if shared.Since == nil || *shared.Since != "20260101" {
		t.Errorf("since should be dash-normalized, got %v", shared.Since)
	}
	if shared.Until == nil || *shared.Until != "20260131" {
		t.Errorf("until = %v", shared.Until)
	}
	if !shared.JSON || shared.Mode != core.ModeCalculate || shared.DebugSamples != 9 ||
		shared.Order != core.OrderDesc || !shared.Breakdown || !shared.NoCost {
		t.Errorf("shared = %+v", shared)
	}
	if shared.Timezone == nil || *shared.Timezone != "Asia/Tokyo" || shared.JQ == nil || *shared.JQ != ".totals" {
		t.Errorf("timezone/jq = %v %v", shared.Timezone, shared.JQ)
	}
	if !daily.Instances || daily.Project == nil || *daily.Project != "proj-a" {
		t.Errorf("daily = %+v", daily)
	}

	// The same run with the CLI having set json, order, since, and instances:
	// only those stay CLI-owned; everything else still comes from config.
	shared = &core.SharedArgs{}
	daily = &DailyArgs{}
	changed := map[string]bool{"json": true, "order": true, "since": true, "instances": true}
	ApplyConfig("daily", "claude", shared, func(name string) bool { return changed[name] }, daily)
	if shared.JSON {
		t.Error("CLI-set json must win")
	}
	if shared.Order != core.OrderAsc {
		t.Error("CLI-set order must win")
	}
	if shared.Since != nil {
		t.Error("CLI-set since must win")
	}
	if daily.Instances {
		t.Error("CLI-set instances must win")
	}
	if shared.Mode != core.ModeCalculate || !shared.Breakdown || !shared.NoCost {
		t.Errorf("unset values should come from config, got %+v", shared)
	}
	if daily.Project == nil || *daily.Project != "proj-a" {
		t.Errorf("unset daily values should come from config, got %+v", daily)
	}
}

func TestApplyConfigBlocksAndWeekly(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ccusage.json")
	err := os.WriteFile(configPath, []byte(`{
		"commands": {
			"weekly": {"startOfWeek": "monday"},
			"blocks": {"tokenLimit": "500000", "sessionLength": 6.5, "active": true}
		}
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Args
	os.Args = []string{"ccusage", "claude", "weekly", "--config", configPath}
	defer func() { os.Args = previous }()

	weekly := &WeeklyArgs{StartOfWeek: core.Sunday}
	ApplyConfig("weekly", "claude", &core.SharedArgs{}, nil, weekly)
	if weekly.StartOfWeek != core.Monday {
		t.Errorf("startOfWeek = %v, want monday", weekly.StartOfWeek)
	}

	blocks := &BlocksArgs{SessionLength: 5}
	ApplyConfig("blocks", "claude", &core.SharedArgs{}, nil, blocks)
	if !blocks.Active || blocks.TokenLimit == nil || *blocks.TokenLimit != "500000" || blocks.SessionLength != 6.5 {
		t.Errorf("blocks = %+v", blocks)
	}
}

func TestApplyToFlagsAppliesThroughFlagSet(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ccusage.json")
	err := os.WriteFile(configPath, []byte(`{
		"defaults": {"order": "desc", "breakdown": true},
		"commands": {"daily": {"instances": true, "project": "proj-a"}}
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	var orderRaw, projectFilter string
	var instances bool
	var breakdown bool
	flags := pflag.NewFlagSet("daily", pflag.ContinueOnError)
	flags.StringVarP(&orderRaw, "order", "o", "asc", "")
	flags.BoolVarP(&instances, "instances", "i", false, "")
	flags.StringVarP(&projectFilter, "project", "p", "", "")
	flags.BoolVarP(&breakdown, "breakdown", "b", false, "")
	if err := flags.Parse([]string{"--order", "asc"}); err != nil {
		t.Fatal(err)
	}
	// ApplyToFlags scans os.Args for --config, mirroring the reference.
	previous := os.Args
	os.Args = []string{"ccusage", "claude", "daily", "--config", configPath}
	defer func() { os.Args = previous }()

	shared := &core.SharedArgs{}
	if err := ApplyToFlags("claude", "daily", flags, shared); err != nil {
		t.Fatal(err)
	}
	if orderRaw != "asc" {
		t.Errorf("CLI-set order must win, got %q", orderRaw)
	}
	if !instances || projectFilter != "proj-a" || !breakdown {
		t.Errorf("config should fill unset flags: instances=%v project=%q breakdown=%v", instances, projectFilter, breakdown)
	}
	if !flags.Changed("order") {
		t.Error("order should be marked changed by the CLI")
	}
}

func TestApplyToFlagsSkipsUnknownFlagsAndInvalidValues(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ccusage.json")
	err := os.WriteFile(configPath, []byte(`{
		"commands": {"weekly": {"startOfWeek": "monday", "tokenLimit": "5", "mode": "bogus"}}
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	var startOfWeekRaw, modeRaw string
	flags := pflag.NewFlagSet("weekly", pflag.ContinueOnError)
	flags.StringVarP(&startOfWeekRaw, "start-of-week", "w", "sunday", "")
	flags.StringVarP(&modeRaw, "mode", "m", "auto", "")
	if err := flags.Parse(nil); err != nil {
		t.Fatal(err)
	}
	previous := os.Args
	os.Args = []string{"ccusage", "claude", "weekly", "--config", configPath}
	defer func() { os.Args = previous }()
	if err := ApplyToFlags("claude", "weekly", flags, &core.SharedArgs{}); err != nil {
		t.Fatal(err)
	}
	if startOfWeekRaw != "monday" {
		t.Errorf("startOfWeek = %q, want monday", startOfWeekRaw)
	}
	if modeRaw != "auto" {
		t.Errorf("invalid config mode must be ignored, got %q", modeRaw)
	}
	// tokenLimit has no flag on weekly: no error, no effect.
}

func TestFlagName(t *testing.T) {
	cases := map[string]string{
		"noOffline":           "no-offline",
		"debugSamples":        "debug-samples",
		"startOfWeek":         "start-of-week",
		"contextLowThreshold": "context-low-threshold",
		"mode":                "mode",
		"pricingOverrides":    "pricingOverrides",
	}
	for key, want := range cases {
		if got := FlagName(key); got != want {
			t.Errorf("FlagName(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestNormalizeDateBound(t *testing.T) {
	if got := NormalizeDateBound("2026-01-09"); got != "20260109" {
		t.Errorf("NormalizeDateBound = %q", got)
	}
	if got := NormalizeDateBound("20260109"); got != "20260109" {
		t.Errorf("NormalizeDateBound = %q", got)
	}
}
