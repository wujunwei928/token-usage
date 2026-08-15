package copilot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wujunwei928/token-usage/internal/core"
)

func writeOtel(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "copilot.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParsesCopilotChatSpans(t *testing.T) {
	path := writeOtel(t,
		`{"type": "metric", "name": "gen_ai.client.token.usage"}`,
		`{"type": "span", "traceId": "trace-1", "spanId": "span-1", "name": "chat claude-sonnet-4", "endTime": [1775934264, 967317833], "attributes": {"gen_ai.operation.name": "chat", "gen_ai.request.model": "claude-sonnet-4", "gen_ai.response.model": "claude-sonnet-4", "gen_ai.conversation.id": "conv-1", "gen_ai.usage.input_tokens": 19452, "gen_ai.usage.output_tokens": 281, "gen_ai.usage.cache_read.input_tokens": 123, "gen_ai.usage.cache_creation.input_tokens": 25, "gen_ai.usage.reasoning.output_tokens": 128}}`)

	entries, err := parseOtelFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	got := entries[0]
	if got.timestampText != "2026-04-11T19:04:24.967Z" {
		t.Errorf("timestamp text = %q", got.timestampText)
	}
	if got.sessionID != "conv-1" {
		t.Errorf("session id = %q", got.sessionID)
	}
	if got.model != "claude-sonnet-4" {
		t.Errorf("model = %q", got.model)
	}
	if got.inputTokens != 19329 {
		t.Errorf("input tokens = %d, want 19329", got.inputTokens)
	}
	if got.outputTokens != 281 {
		t.Errorf("output tokens = %d, want 281", got.outputTokens)
	}
	if got.cacheCreationTokens != 25 {
		t.Errorf("cache creation = %d, want 25", got.cacheCreationTokens)
	}
	if got.cacheReadTokens != 123 {
		t.Errorf("cache read = %d, want 123", got.cacheReadTokens)
	}
	if got.reasoningOutputTokens != 128 {
		t.Errorf("reasoning = %d, want 128", got.reasoningOutputTokens)
	}
	if got.dedupKey != "trace-1:span-1" {
		t.Errorf("dedup key = %q", got.dedupKey)
	}
}

func TestSuppressesLowerPriorityRecordsForSameResponse(t *testing.T) {
	path := writeOtel(t,
		`{"type": "span", "traceId": "trace-dupe", "spanId": "agent-1", "name": "invoke_agent GitHub Copilot Chat", "attributes": {"gen_ai.operation.name": "invoke_agent", "gen_ai.response.model": "gpt-5.4-mini", "gen_ai.conversation.id": "conv-dupe", "gen_ai.response.id": "resp-dupe", "gen_ai.usage.input_tokens": 100, "gen_ai.usage.output_tokens": 30}}`,
		`{"hrTime": [1775934263, 0], "attributes": {"event.name": "gen_ai.client.inference.operation.details", "gen_ai.response.model": "gpt-5.4-mini", "gen_ai.response.id": "resp-dupe", "gen_ai.usage.input_tokens": 80, "gen_ai.usage.output_tokens": 20}, "_body": "GenAI inference: gpt-5.4-mini"}`,
		`{"type": "span", "traceId": "trace-dupe", "spanId": "chat-1", "name": "chat gpt-5.4-mini", "attributes": {"gen_ai.operation.name": "chat", "gen_ai.response.model": "gpt-5.4-mini", "gen_ai.conversation.id": "conv-dupe", "gen_ai.response.id": "resp-dupe", "gen_ai.usage.input_tokens": 60, "gen_ai.usage.output_tokens": 10}}`)

	entries, err := parseOtelFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].dedupKey != "trace-dupe:chat-1" {
		t.Errorf("dedup key = %q", entries[0].dedupKey)
	}
	if entries[0].inputTokens != 60 {
		t.Errorf("input tokens = %d, want 60", entries[0].inputTokens)
	}
	if entries[0].outputTokens != 10 {
		t.Errorf("output tokens = %d, want 10", entries[0].outputTokens)
	}
}

func TestIncludesReasoningTokensInTotalTokens(t *testing.T) {
	path := writeOtel(t,
		`{"type": "span", "traceId": "trace-1", "spanId": "span-1", "name": "chat test-model", "endTime": [1775934264, 0], "attributes": {"gen_ai.operation.name": "chat", "gen_ai.response.model": "test-model", "gen_ai.conversation.id": "conv-1", "gen_ai.usage.input_tokens": 100, "gen_ai.usage.output_tokens": 50, "gen_ai.usage.cache_read.input_tokens": 10, "gen_ai.usage.cache_creation.input_tokens": 20, "gen_ai.usage.reasoning.output_tokens": 5}}`)

	pricing := core.NewPricingMap()
	pricing.LoadJSON(`{"test-model":{"input_cost_per_token":1,"output_cost_per_token":2,"cache_creation_input_token_cost":3,"cache_read_input_token_cost":4}}`)

	loaded, err := readOtelFile(path, time.UTC, core.ModeAuto, pricing)
	if err != nil {
		t.Fatal(err)
	}
	rows := SummarizeEntries(loaded, KindDaily)
	report := core.SerializeJ(ReportJSON(rows, KindDaily))

	want := `{
  "daily": [
    {
      "cacheCreationTokens": 20,
      "cacheReadTokens": 10,
      "date": "2026-04-11",
      "inputTokens": 90,
      "modelBreakdowns": [
        {
          "cacheCreationTokens": 20,
          "cacheReadTokens": 10,
          "cost": 300.0,
          "inputTokens": 90,
          "modelName": "test-model",
          "outputTokens": 50
        }
      ],
      "modelsUsed": [
        "test-model"
      ],
      "outputTokens": 50,
      "totalCost": 300.0,
      "totalTokens": 175
    }
  ],
  "totals": {
    "cacheCreationTokens": 20,
    "cacheReadTokens": 10,
    "inputTokens": 90,
    "outputTokens": 50,
    "totalCost": 300.0,
    "totalTokens": 175
  }
}`
	if report != want {
		t.Errorf("report mismatch:\n%s", report)
	}
}

func TestFallsBackToTotalTokensWhenCopilotPartsAreMissing(t *testing.T) {
	path := writeOtel(t,
		`{"type": "span", "traceId": "trace-1", "spanId": "span-1", "name": "chat test-model", "endTime": [1775934264, 0], "attributes": {"gen_ai.operation.name": "chat", "gen_ai.response.model": "test-model", "gen_ai.conversation.id": "conv-1", "gen_ai.usage.total_tokens": 567}}`)

	entries, err := parseOtelFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].outputTokens != 567 {
		t.Errorf("output tokens = %d, want 567", entries[0].outputTokens)
	}
	if entries[0].reasoningOutputTokens != 0 {
		t.Errorf("reasoning tokens = %d, want 0", entries[0].reasoningOutputTokens)
	}
}
