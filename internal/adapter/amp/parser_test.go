package amp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei/ccusage-go/internal/core"
)

func writeThread(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "thread.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFallsBackToTotalTokensWhenAmpPartsAreMissing(t *testing.T) {
	path := writeThread(t,
		`{"id":"thread-a","usageLedger":{"events":[{"id":"event-a","timestamp":"2026-01-02T00:00:00.000Z","model":"gpt-5","tokens":{"total":345}}]}}`)

	entries, err := readThreadFile(path, nil, core.ModeAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if got := entries[0].Data.Message.Usage.OutputTokens; got != 345 {
		t.Errorf("output tokens = %d, want 345", got)
	}
	if got := entries[0].ExtraTotalTokens; got != 0 {
		t.Errorf("extra total tokens = %d, want 0", got)
	}
}

func TestReadsUsageFromMessagesWhenLedgerIsMissing(t *testing.T) {
	path := writeThread(t, `{
		"id":"T-thread-a",
		"messages":[
			{"role":"user","content":"hi"},
			{"role":"assistant","usage":{
				"model":"claude-haiku-4-5-20251001",
				"inputTokens":10,
				"outputTokens":178,
				"cacheCreationInputTokens":986,
				"cacheReadInputTokens":11372,
				"totalInputTokens":12368,
				"timestamp":"2026-01-19T11:42:10.652Z"
			}},
			{"role":"assistant","usage":{
				"model":"claude-haiku-4-5-20251001",
				"inputTokens":5,
				"outputTokens":42,
				"cacheCreationInputTokens":0,
				"cacheReadInputTokens":12000,
				"timestamp":"2026-01-19T11:43:00.000Z"
			}}
		]
	}`)

	entries, err := readThreadFile(path, nil, core.ModeAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	first := entries[0].Data.Message.Usage
	if first.InputTokens != 10 || first.OutputTokens != 178 ||
		first.CacheCreationInputTokens != 986 || first.CacheReadInputTokens != 11372 {
		t.Errorf("first usage = %+v", first)
	}
	if model := entries[0].Data.Message.Model; model == nil || *model != "claude-haiku-4-5-20251001" {
		t.Errorf("first model = %v", model)
	}
	if entries[0].SessionID != "T-thread-a" {
		t.Errorf("session id = %q", entries[0].SessionID)
	}
	if entries[1].Data.Message.Usage.InputTokens != 5 {
		t.Errorf("second input tokens = %d, want 5", entries[1].Data.Message.Usage.InputTokens)
	}
}

func TestLedgerEventsTakePrecedenceOverMessagesUsage(t *testing.T) {
	path := writeThread(t, `{
		"id":"thread-a",
		"usageLedger":{"events":[{
			"id":"event-a",
			"timestamp":"2026-01-02T00:00:00.000Z",
			"model":"gpt-5",
			"tokens":{"input":1,"output":2}
		}]},
		"messages":[
			{"role":"assistant","usage":{
				"model":"claude-haiku-4-5-20251001",
				"inputTokens":99,
				"outputTokens":99,
				"timestamp":"2026-01-19T11:42:10.652Z"
			}}
		]
	}`)

	entries, err := readThreadFile(path, nil, core.ModeAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if model := entries[0].Data.Message.Model; model == nil || *model != "gpt-5" {
		t.Errorf("model = %v, want gpt-5", model)
	}
	if got := entries[0].Data.Message.Usage.InputTokens; got != 1 {
		t.Errorf("input tokens = %d, want 1", got)
	}
}

func TestSkipsMessagesWithNoUsageTokens(t *testing.T) {
	path := writeThread(t, `{
		"id":"T-thread-a",
		"messages":[
			{"role":"assistant","usage":{
				"model":"claude-haiku-4-5-20251001",
				"inputTokens":0,
				"outputTokens":0,
				"cacheCreationInputTokens":0,
				"cacheReadInputTokens":0,
				"timestamp":"2026-01-19T11:42:10.652Z"
			}}
		]
	}`)

	entries, err := readThreadFile(path, nil, core.ModeAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want 0", len(entries))
	}
}

func TestFallsBackToTotalTokensInMessagesPath(t *testing.T) {
	path := writeThread(t, `{
		"id":"T-thread-a",
		"messages":[
			{"role":"assistant","usage":{
				"model":"claude-haiku-4-5-20251001",
				"totalTokens":345,
				"timestamp":"2026-01-19T11:42:10.652Z"
			}}
		]
	}`)

	entries, err := readThreadFile(path, nil, core.ModeAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if got := entries[0].Data.Message.Usage.OutputTokens; got != 345 {
		t.Errorf("output tokens = %d, want 345", got)
	}
	if got := entries[0].ExtraTotalTokens; got != 0 {
		t.Errorf("extra total tokens = %d, want 0", got)
	}
}

func TestEmptyUsageLedgerFallsBackToMessageUsage(t *testing.T) {
	path := writeThread(t, `{
		"id":"T-thread-a",
		"usageLedger":{},
		"messages":[
			{"role":"assistant","usage":{
				"model":"claude-haiku-4-5-20251001",
				"inputTokens":10,
				"outputTokens":178,
				"timestamp":"2026-01-19T11:42:10.652Z"
			}}
		]
	}`)

	entries, err := readThreadFile(path, nil, core.ModeAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	usage := entries[0].Data.Message.Usage
	if usage.InputTokens != 10 || usage.OutputTokens != 178 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestMalformedMessageElementDoesNotDropTheThread(t *testing.T) {
	path := writeThread(t, `{
		"id":"T-thread-a",
		"usageLedger":"not-an-object",
		"messages":[
			"garbage",
			{"role":"assistant","usage":{
				"model":"claude-haiku-4-5-20251001",
				"inputTokens":10,
				"outputTokens":178,
				"timestamp":"2026-01-19T11:42:10.652Z"
			}}
		]
	}`)

	entries, err := readThreadFile(path, nil, core.ModeAuto, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	usage := entries[0].Data.Message.Usage
	if usage.InputTokens != 10 || usage.OutputTokens != 178 {
		t.Errorf("usage = %+v", usage)
	}
}
