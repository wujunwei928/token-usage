package cline

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
	"github.com/wujunwei928/token-usage/internal/core"
)

// The shared adapter contract, run against a synthetic sessions fixture that
// mirrors the real ~/.cline/data/sessions layout.
func TestAdapterContract(t *testing.T) {
	common.RunContract(t, common.AdapterContract{
		Agent:   "cline",
		Profile: Profile,
		Build: func(t *testing.T) (common.Adapter, common.LoadRequest) {
			root := t.TempDir()
			writeSession(t, root, "1788000000000_day1",
				"D:/proj/a", 1786701600000, "m1", 100, 50, 30)
			writeSession(t, root, "1788000000001_day2",
				"D:\\proj\\b", 1786788000000, "m2", 200, 20, 0)
			t.Setenv(DataDirEnv, root)
			utc := "UTC"
			shared := &core.SharedArgs{Timezone: &utc, Offline: true}
			adapter, ok := common.BuildAdapter("cline", shared)
			if !ok {
				t.Fatalf("cline not registered in the adapter registry")
			}
			return adapter, common.LoadRequest{Shared: shared}
		},
	})
}

// writeSession creates one session directory with the <id>.json manifest and
// <id>.messages.json pair; each conversation carries one user message
// (no usage) and one assistant message (the usage carrier) with a
// provider-reported cost.
func writeSession(t *testing.T, root, id, cwd string, ts int64, msgID string, in, out, cacheRead uint64) {
	t.Helper()
	dir := filepath.Join(root, "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	sessionJSON := `{"session_id":"` + id + `","cwd":"` + cwd + `","workspace_root":"/wr/` + id + `"}`
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(sessionJSON), 0o644); err != nil {
		t.Fatalf("write session json: %v", err)
	}
	messagesJSON := `{"version":1,"sessionId":"` + id + `","messages":[` +
		`{"id":"user_1","role":"user","ts":` + strconv.FormatInt(ts, 10) + `,"content":[]},` +
		`{"id":"` + msgID + `","role":"assistant","ts":` + strconv.FormatInt(ts+1000, 10) + `,` +
		`"metrics":{"inputTokens":` + strconv.FormatInt(int64(in), 10) + `,"outputTokens":` + strconv.FormatInt(int64(out), 10) + `,"cacheReadTokens":` + strconv.FormatInt(int64(cacheRead), 10) + `,"cacheWriteTokens":0,"cost":0.01},` +
		`"modelInfo":{"id":"MiniMax-M3","provider":"minimax-cn-coding-plan"}}` +
		`]}`
	if err := os.WriteFile(filepath.Join(dir, id+".messages.json"), []byte(messagesJSON), 0o644); err != nil {
		t.Fatalf("write messages json: %v", err)
	}
}
