package common

import (
	"github.com/wujunwei928/token-usage/internal/core"
)

// Factory builds one adapter for a run. Closures capture agent-specific CLI
// flags and pricing-load semantics, keeping them out of the shared interface.
type Factory func(shared *core.SharedArgs) Adapter

// registeredAgents maps roster names to factories. Registration happens in
// adapter package init functions; duplicates are programmer errors.
var registeredAgents = map[string]Factory{}

// rosterOrder is the unified-report display order: the reference roster with
// adapters beyond upstream appended (ADR 0006); grok is gone (ADR 0010 — the
// frozen port is resurrected from git history when the reference ships it).
var rosterOrder = []string{
	"claude", "codex", "opencode", "amp", "droid", "codebuff", "hermes",
	"pi", "goose", "openclaw", "kilo", "copilot", "gemini", "kimi", "qwen",
	"zcode", "omp", "cline",
}

// RegisterAgent installs an adapter factory under its roster name. Duplicate
// or nil registrations panic at init time — roster names are compile-time
// facts, not configuration.
func RegisterAgent(name string, factory Factory) {
	if factory == nil {
		panic("token-usage: nil adapter factory for " + name)
	}
	if _, dup := registeredAgents[name]; dup {
		panic("token-usage: duplicate adapter registration for " + name)
	}
	registeredAgents[name] = factory
}

// Roster returns the adapter names in unified-report order.
func Roster() []string {
	return append([]string(nil), rosterOrder...)
}

// BuildAdapter instantiates the named adapter; ok=false for roster names that
// have not landed (the caller decides placeholder semantics).
func BuildAdapter(name string, shared *core.SharedArgs) (Adapter, bool) {
	factory, ok := registeredAgents[name]
	if !ok {
		return nil, false
	}
	return factory(shared), true
}
