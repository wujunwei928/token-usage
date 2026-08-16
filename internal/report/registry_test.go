package report

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The snapshot iterates the adapter registry: every roster name must resolve
// to a registered adapter, except codex whose Groups pipeline stays behind
// the hand-written event bridge (ADR 0009's permanent exception).
func TestSnapshotRosterCoverage(t *testing.T) {
	shared := &core.SharedArgs{Mode: core.ModeDisplay, Offline: true}
	for _, name := range common.Roster() {
		adapter, ok := common.BuildAdapter(name, shared)
		if name == "codex" {
			if ok {
				t.Error("codex must not register a lossy entries adapter")
			}
			continue
		}
		if !ok {
			t.Errorf("roster name %q has no registered adapter", name)
			continue
		}
		if adapter.Agent() != name {
			t.Errorf("adapter Agent() = %q, want %q", adapter.Agent(), name)
		}
	}
}
