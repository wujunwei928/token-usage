package common

import (
	"github.com/wujunwei928/token-usage/internal/core"
)

// Adapter is the unified Agent Adapter contract (ADR 0009): one interface for
// discovering and loading one agent's Usage Entries, consumed by the unified
// report, the CLI, and the leaderboard snapshot.
type Adapter interface {
	// Agent returns the roster name ("droid", "zcode", ...).
	Agent() string

	// HasData reports whether the agent's local data source exists at all,
	// independent of any date window. Feeds the Detected verdict; adapters
	// without a cheap existence probe return false.
	HasData() bool

	// LoadEntries loads the agent's Usage Entries. Adapters that price
	// internally need no pricing table; the rest receive one through their
	// factory closure, never from package state.
	LoadEntries(req LoadRequest) (LoadResult, error)
}

// LoadRequest carries the context every adapter load needs. Pricing travels
// through the adapter's factory closure (each agent keeps its load semantics
// there until candidate 3 unifies them), so the request stays at one field.
// Agent-specific CLI flags (path overrides, project filters) enter the same
// way — the interface does not grow a field per agent.
type LoadRequest struct {
	Shared *core.SharedArgs
}

// LoadResult is one adapter's contribution: its entries plus the Detected
// verdict (entries non-empty OR the data source exists).
type LoadResult struct {
	Entries  []core.LoadedEntry
	Detected bool
}
