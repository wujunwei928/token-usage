package statusline

import (
	"strings"
	"testing"
)

// The expected messages below were captured from the reference binary; the
// parser must reproduce serde_json's wording and line/column arithmetic.
func TestParseHookErrorsMatchReference(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"not json", "not json", "expected ident at line 1 column 2"},
		{"bare ident", "x", "expected value at line 1 column 1"},
		{"empty object", "{}", "missing field `session_id` at line 1 column 2"},
		{"missing transcript", `{"session_id":"s"}`, "missing field `transcript_path` at line 1 column 18"},
		{"missing model", `{"session_id":"s","transcript_path":"t"}`, "missing field `model` at line 1 column 40"},
		{"model empty", `{"session_id":"s","transcript_path":"t","model":{}}`, "missing field `display_name` at line 1 column 50"},
		{"int session id", `{"session_id":123}`, "invalid type: integer `123`, expected a string at line 1 column 17"},
		{"string cost", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"cost":{"total_cost_usd":"x"}}`, `invalid type: string "x", expected f64 at line 1 column 97`},
		{"object then garbage", "{}x", "missing field `session_id` at line 1 column 2"},
		{"trailing comma", `{"session_id":"s",}`, "trailing comma at line 1 column 19"},
		{"open bracket", "[", "EOF while parsing a list at line 1 column 1"},
		{"ident eof", `{"session_id":tr`, "EOF while parsing a value at line 1 column 16"},
		{"missing separator", `{"session_id":"s" "x":1}`, "expected `,` or `}` at line 1 column 19"},
		{"null document", "null", "invalid type: null, expected struct StatuslineHook at line 1 column 4"},
		{"null session id", `{"session_id":null}`, "invalid type: null, expected a string at line 1 column 18"},
		{"model scalar", `{"session_id":"s","transcript_path":"t","model":5}`, "invalid type: integer `5`, expected struct HookModel at line 1 column 49"},
		{"model id int", `{"session_id":"s","transcript_path":"t","model":{"id":1,"display_name":"D"}}`, "invalid type: integer `1`, expected a string at line 1 column 55"},
		{"cost scalar", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"cost":5}`, "invalid type: integer `5`, expected struct HookCost at line 1 column 77"},
		{"negative u64", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"context_window":{"total_input_tokens":-1,"context_window_size":10}}`, "invalid value: integer `-1`, expected u64 at line 1 column 110"},
		{"float u64", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"context_window":{"total_input_tokens":1.5,"context_window_size":10}}`, "invalid type: floating point `1.5`, expected u64 at line 1 column 111"},
		{"seq session id", `{"session_id":[],"transcript_path":"t","model":{"display_name":"D"}}`, "invalid type: sequence, expected a string at line 1 column 14"},
		{"bool session id", `{"session_id":true,"transcript_path":"t","model":{"display_name":"D"}}`, "invalid type: boolean `true`, expected a string at line 1 column 18"},
		{"map session id", `{"session_id":{},"transcript_path":"t","model":{"display_name":"D"}}`, "invalid type: map, expected a string at line 1 column 14"},
		{"seq transcript", `{"session_id":"s","transcript_path":[],"model":{"display_name":"D"}}`, "invalid type: sequence, expected a string at line 1 column 36"},
		{"model from empty seq", `{"session_id":"s","transcript_path":"t","model":[]}`, "invalid length 0, expected struct HookModel with 2 elements at line 1 column 50"},
		{"model seq then field", `{"session_id":"s","transcript_path":"t","model":[],"x":1}`, "invalid length 0, expected struct HookModel with 2 elements at line 1 column 50"},
		{"top seq", "[]", "invalid length 0, expected struct StatuslineHook with 6 elements at line 1 column 2"},
		{"top int", "123", "invalid type: integer `123`, expected struct StatuslineHook at line 1 column 3"},
		{"top string", `"str"`, `invalid type: string "str", expected struct StatuslineHook at line 1 column 5`},
		{"unterminated string", `{"session_id":"abc`, "EOF while parsing a string at line 1 column 18"},
		{"bad escape", `{"session_id":"a\q"}`, "invalid escape at line 1 column 18"},
		{"eof object", `{"session_id":"a"`, "EOF while parsing an object at line 1 column 17"},
		{"int key", "{1:2}", "key must be a string at line 1 column 2"},
		{"missing colon", `{"a" 1}`, "expected `:` at line 1 column 6"},
		{"garbage after value", `{"session_id":"s"x}`, "expected `,` or `}` at line 1 column 18"},
		{"cost missing field", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"cost":{}}`, "missing field `total_cost_usd` at line 1 column 78"},
		{"context missing field", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"context_window":{}}`, "missing field `total_input_tokens` at line 1 column 88"},
		{"effort missing field", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"effort":{}}`, "missing field `level` at line 1 column 80"},
		{"broken exponent", `{"session_id":"s","transcript_path":"t","model":{"display_name":"D"},"cost":{"total_cost_usd":1e}}`, "invalid number at line 1 column 97"},
		{"duplicate session id", `{"session_id":"a","session_id":"b","transcript_path":"t","model":{"display_name":"D"}}`, "duplicate field `session_id` at line 1 column 30"},
		{"duplicate model", `{"session_id":"a","transcript_path":"t","model":{"display_name":"D"},"model":{"display_name":"E"}}`, "duplicate field `model` at line 1 column 76"},
		{"u64 overflow becomes float", `{"session_id":"a","transcript_path":"t","model":{"display_name":"D"},"context_window":{"total_input_tokens":18446744073709551616,"context_window_size":10}}`, "invalid type: floating point `1.8446744073709552e+19`, expected u64 at line 1 column 128"},
		{"float overflow", `{"session_id":"a","transcript_path":"t","model":{"display_name":"D"},"context_window":{"total_input_tokens":1e400,"context_window_size":10}}`, "number out of range at line 1 column 113"},
		{"trailing characters", `{"session_id":"a","transcript_path":"t","model":{"display_name":"D"}} trailing`, "trailing characters at line 1 column 71"},
		{"inner trailing comma", `{"session_id":"a","transcript_path":"t","model":{"display_name":"D"},}`, "trailing comma at line 1 column 70"},
		{"null display name", `{"session_id":"a","transcript_path":"t","model":{"display_name":null}}`, "invalid type: null, expected a string at line 1 column 68"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseHook(tc.input)
			if err == nil {
				t.Fatalf("ParseHook(%q) succeeded, want error %q", tc.input, tc.want)
			}
			if err.Error() != tc.want {
				t.Errorf("ParseHook(%q)\n got %q\nwant %q", tc.input, err.Error(), tc.want)
			}
		})
	}
}

func TestParseHookMultiLineErrorPositions(t *testing.T) {
	// A pretty-printed hook missing session_id reports the closing brace of
	// the object on its own line.
	input := "{\n\t\"transcript_path\": \"t\",\n\t\"model\": {\"display_name\": \"D\"}\n}"
	_, err := ParseHook(input)
	want := "missing field `session_id` at line 4 column 1"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestParseHookSuccess(t *testing.T) {
	input := "{\n" +
		"\t\"session_id\": \"73cc9f9a-2775-4418-beec-bc36b62a1c6f\",\n" +
		"\t\"transcript_path\": \"/Users/x/.config/claude/projects/p/73cc.jsonl\",\n" +
		"\t\"cwd\": \"/Users/x\",\n" +
		"\t\"model\": {\"id\": \"claude-sonnet-4-20250514\", \"display_name\": \"Sonnet 4\"},\n" +
		"\t\"version\": \"1.0.88\",\n" +
		"\t\"cost\": {\"total_cost_usd\": 0.056266149999999994, \"total_duration_ms\": 164055},\n" +
		"\t\"context_window\": {\"total_input_tokens\": 42500, \"total_output_tokens\": 3200, \"context_window_size\": 200000},\n" +
		"\t\"exceeds_200k_tokens\": false\n" +
		"}"
	hook, err := ParseHook(input)
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if hook.SessionID != "73cc9f9a-2775-4418-beec-bc36b62a1c6f" {
		t.Errorf("SessionID = %q", hook.SessionID)
	}
	if hook.Model.ID == nil || *hook.Model.ID != "claude-sonnet-4-20250514" {
		t.Errorf("Model.ID = %v", hook.Model.ID)
	}
	if hook.Model.DisplayName != "Sonnet 4" {
		t.Errorf("Model.DisplayName = %q", hook.Model.DisplayName)
	}
	if hook.Cost == nil || hook.Cost.TotalCostUSD != 0.056266149999999994 {
		t.Errorf("Cost = %+v", hook.Cost)
	}
	if hook.ContextWindow == nil || hook.ContextWindow.TotalInputTokens != 42500 ||
		hook.ContextWindow.ContextWindowSize != 200000 {
		t.Errorf("ContextWindow = %+v", hook.ContextWindow)
	}
	if hook.Effort != nil {
		t.Errorf("Effort = %+v, want nil", hook.Effort)
	}
}

func TestParseHookOptionalNulls(t *testing.T) {
	hook, err := ParseHook(`{
		"session_id": "s",
		"transcript_path": "t",
		"model": {"id": null, "display_name": "D"},
		"cost": null,
		"context_window": null,
		"effort": null
	}`)
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if hook.Model.ID != nil || hook.Cost != nil || hook.ContextWindow != nil || hook.Effort != nil {
		t.Errorf("optional nulls not respected: %+v", hook)
	}
}

func TestParseHookEffortAndEscapes(t *testing.T) {
	hook, err := ParseHook(`{"session_id":"😀","transcript_path":"a\nb","model":{"display_name":"D"},"effort":{"level":"high"}}`)
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if hook.SessionID != "😀" {
		t.Errorf("SessionID = %q", hook.SessionID)
	}
	if hook.TranscriptPath != "a\nb" {
		t.Errorf("TranscriptPath = %q", hook.TranscriptPath)
	}
	if hook.Effort == nil || hook.Effort.Level != "high" {
		t.Errorf("Effort = %+v", hook.Effort)
	}
}

func TestParseHookModelFromSeq(t *testing.T) {
	// serde accepts the positional sequence form of a struct.
	hook, err := ParseHook(`{"session_id":"s","transcript_path":"t","model":["the-id","Opus 4.6"]}`)
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if hook.Model.ID == nil || *hook.Model.ID != "the-id" || hook.Model.DisplayName != "Opus 4.6" {
		t.Errorf("Model = %+v", hook.Model)
	}
}

func TestParseHookTrimsWhitespace(t *testing.T) {
	hook, err := ParseHook("\n\t {\"session_id\":\"s\",\"transcript_path\":\"t\",\"model\":{\"display_name\":\"D\"}} \n ")
	if err != nil {
		t.Fatalf("ParseHook: %v", err)
	}
	if hook.SessionID != "s" {
		t.Errorf("SessionID = %q", hook.SessionID)
	}
	if strings.TrimSpace(hook.SessionID) != hook.SessionID {
		t.Errorf("unexpected whitespace in %q", hook.SessionID)
	}
}
