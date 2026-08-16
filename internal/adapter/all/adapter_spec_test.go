package all

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// A fake adapter standing in for a registered agent: two dates, two sessions,
// known token masses.
type adapterSpecFake struct{}

func (adapterSpecFake) Agent() string { return "adapter-spec-fake" }
func (adapterSpecFake) HasData() bool { return false }

func (adapterSpecFake) LoadEntries(req common.LoadRequest) (common.LoadResult, error) {
	model := "fake-model"
	// base is 2026-08-14T10:00:00.000Z, consistent with the Date fields.
	const base = int64(1786701600000)
	mk := func(ts int64, date, session string, in uint64) core.LoadedEntry {
		return core.LoadedEntry{
			Data: core.UsageEntry{Message: core.UsageMessage{
				Usage: core.TokenUsageRaw{InputTokens: in}, Model: &model,
			}},
			Timestamp: ts, Date: date, SessionID: session, Project: "fake",
			ProjectPath: "Fake", Model: &model,
		}
	}
	entries := []core.LoadedEntry{
		mk(base, "2026-08-14", "A", 100),
		mk(base+24*3600000, "2026-08-15", "A", 50),
		mk(base+25*3600000, "2026-08-15", "B", 10),
	}
	return common.LoadResult{Entries: entries, Detected: true}, nil
}

func init() {
	common.RegisterAgent("adapter-spec-fake", func(shared *core.SharedArgs) common.Adapter {
		return adapterSpecFake{}
	})
}

// The new registry-backed consumption path must produce the same unified rows
// the hand-written spec pipelines produced: detect, window, summarize,
// convert to rows.
func TestAdapterSpecDaily(t *testing.T) {
	shared := &core.SharedArgs{}
	spec := AdapterSpec("adapter-spec-fake", SpecOptions{Profile: common.ReportProfile{}})(shared)
	if spec.Agent != "adapter-spec-fake" {
		t.Fatalf("Agent = %q", spec.Agent)
	}
	rows, err := spec.Load(KindDaily)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !rows.Detected {
		t.Error("Detected = false, want true")
	}
	if len(rows.Rows) != 2 ||
		rows.Rows[0].Period != "2026-08-14" || rows.Rows[0].InputTokens != 100 ||
		rows.Rows[1].Period != "2026-08-15" || rows.Rows[1].InputTokens != 60 {
		t.Fatalf("daily rows = %+v", rows.Rows)
	}
	if rows.Rows[0].Agent != "adapter-spec-fake" || rows.Rows[0].MetadataAgents[0] != "adapter-spec-fake" {
		t.Errorf("row agent fields = %q / %v", rows.Rows[0].Agent, rows.Rows[0].MetadataAgents)
	}
}

func TestAdapterSpecSessionWindow(t *testing.T) {
	since := "20260815"
	shared := &core.SharedArgs{Since: &since}
	// Filter-after profile: session A survives whole (150) because its last
	// activity reaches into the window.
	spec := AdapterSpec("adapter-spec-fake", SpecOptions{Profile: common.ReportProfile{SessionByActivity: true, SessionFilterAfter: true}})(shared)
	rows, err := spec.Load(KindSession)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rows.Rows) != 2 || rows.Rows[0].InputTokens != 150 || rows.Rows[1].InputTokens != 10 {
		t.Fatalf("session rows = %+v, want A=150 B=10", rows.Rows)
	}
}

func TestAdapterSpecUnknownAgentIsEmpty(t *testing.T) {
	shared := &core.SharedArgs{}
	spec := AdapterSpec("never-registered-agent", SpecOptions{})(shared)
	rows, err := spec.Load(KindDaily)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rows.Detected || len(rows.Rows) != 0 {
		t.Fatalf("unknown agent produced %+v", rows)
	}
}
