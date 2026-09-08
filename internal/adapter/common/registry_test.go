package common

import (
	"reflect"
	"testing"

	"github.com/wujunwei928/token-usage/internal/core"
)

type registryFake struct{ name string }

func (f registryFake) Agent() string { return f.name }
func (f registryFake) HasData() bool { return false }
func (f registryFake) LoadEntries(req LoadRequest) (LoadResult, error) {
	entry := core.LoadedEntry{Date: "2026-08-14", SessionID: "s1"}
	return LoadResult{Entries: []core.LoadedEntry{entry}, Detected: true}, nil
}

func TestRegistryRegistersByName(t *testing.T) {
	name := "registry-fake"
	RegisterAgent(name, func(shared *core.SharedArgs) Adapter { return registryFake{name} })
	adapter, ok := BuildAdapter(name, &core.SharedArgs{})
	if !ok {
		t.Fatalf("BuildAdapter(%q) missed", name)
	}
	if adapter.Agent() != name {
		t.Errorf("Agent() = %q, want %q", adapter.Agent(), name)
	}
	result, err := adapter.LoadEntries(LoadRequest{Shared: &core.SharedArgs{}})
	if err != nil || len(result.Entries) != 1 || result.Entries[0].Date != "2026-08-14" {
		t.Fatalf("LoadEntries = %+v, %v", result, err)
	}
	if !result.Detected {
		t.Error("Detected = false, want true")
	}
}

func TestRegistryRosterOrder(t *testing.T) {
	// The reference roster with the beyond-upstream adapters appended
	// (zcode, omp and cline, ADR 0006); grok removed with its frozen port (ADR 0010).
	want := []string{
		"claude", "codex", "opencode", "amp", "droid", "codebuff", "hermes",
		"pi", "goose", "openclaw", "kilo", "copilot", "gemini", "kimi", "qwen",
		"zcode", "omp", "cline",
	}
	if got := Roster(); !reflect.DeepEqual(got, want) {
		t.Errorf("Roster() = %v, want %v", got, want)
	}
}

func TestRegistryUnknownNameMisses(t *testing.T) {
	if _, ok := BuildAdapter("no-such-agent", &core.SharedArgs{}); ok {
		t.Error("BuildAdapter for unknown agent hit")
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("duplicate registration did not panic")
		}
	}()
	RegisterAgent("registry-fake", func(shared *core.SharedArgs) Adapter { return registryFake{} })
}

func TestRegistryNilFactoryPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("nil factory registration did not panic")
		}
	}()
	RegisterAgent("registry-nil-fake", nil)
}
