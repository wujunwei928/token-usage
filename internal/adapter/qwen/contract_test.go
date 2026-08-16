package qwen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against a synthetic chat file fixture.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "qwen",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			root := t.TempDir()
			chats := filepath.Join(root, "projects", "demo", "chats")
			if err := os.MkdirAll(chats, 0o755); err != nil {
				t.Fatalf("mkdir chats: %v", err)
			}
			write := func(name, stamped string, prompt, candidates, cached uint64) {
				content := `{"type":"assistant","timestamp":"` + stamped +
					`","model":"qwen3-coder","usageMetadata":{"promptTokenCount":` + u64(prompt) +
					`,"candidatesTokenCount":` + u64(candidates) +
					`,"cachedContentTokenCount":` + u64(cached) + `}}`
				if err := os.WriteFile(filepath.Join(chats, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write chat file: %v", err)
				}
			}
			write("sess-a.jsonl", "2026-08-14T10:00:00.000Z", 100, 50, 10)
			write("sess-b.jsonl", "2026-08-15T11:00:00.000Z", 200, 20, 0)
			t.Setenv(DataDirEnv, root)
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("qwen", shared)
			if !ok {
				t.Fatalf("qwen not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}

// HasData feeds Detected even when no line parses into an entry.
func TestAdapterHasDataDetected(t *testing.T) {
	root := t.TempDir()
	chats := filepath.Join(root, "projects", "demo", "chats")
	if err := os.MkdirAll(chats, 0o755); err != nil {
		t.Fatalf("mkdir chats: %v", err)
	}
	// A chat file whose only line carries no usageMetadata: the source exists
	// but yields no entries — the agent must still count as Detected.
	if err := os.WriteFile(filepath.Join(chats, "empty.jsonl"),
		[]byte(`{"type":"user","timestamp":"2026-08-14T10:00:00.000Z"}`), 0o644); err != nil {
		t.Fatalf("write chat file: %v", err)
	}
	t.Setenv(DataDirEnv, root)
	utc := "UTC"
	shared := &core.SharedArgs{Timezone: &utc, Offline: true}
	adapter, ok := common.BuildAdapter("qwen", shared)
	if !ok {
		t.Fatalf("qwen not registered")
	}
	result, err := adapter.LoadEntries(common.LoadRequest{Shared: shared})
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("entries = %d, want 0 from the unparsable chat file", len(result.Entries))
	}
	if !adapter.HasData() || !result.Detected {
		t.Error("HasData/Detected = false, want true (data source exists)")
	}
}

func u64(v uint64) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
