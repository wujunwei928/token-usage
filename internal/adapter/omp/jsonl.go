package omp

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Lenient JSON field helpers mirrored from the pi adapter (ported from rust
// adapters/common/src/jsonl.rs) so typed structs behave like the historical
// dynamic-Value navigation: an unexpectedly typed field never fails the
// whole record. Mirrored, not shared: changes to pi's copy do not propagate.

// nonEmptyString trims a string value and drops empty/non-string values.
func nonEmptyString(raw json.RawMessage) *string {
	if !isJSONString(raw) {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// lenientU64 mirrors serde_json Value::as_u64: only non-negative integers that
// fit u64 count; floats, strings, nulls, and negatives become 0.
func lenientU64(raw json.RawMessage) uint64 {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || !allDigits(text) {
		return 0
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// lenientF64 mirrors Value::as_f64: any JSON number yields a value; strings,
// nulls, and non-numbers become absent.
func lenientF64(raw json.RawMessage) *float64 {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || !isJSONNumber(text) {
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil
	}
	return &value
}

// isJSONObject reports whether the raw value is a JSON object.
func isJSONObject(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "{")
}

func isJSONString(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "\"")
}

// isJSONNumber accepts the JSON number grammar (no leading zeros beyond "0").
func isJSONNumber(text string) bool {
	if text == "" {
		return false
	}
	start := 0
	if text[0] == '-' {
		start = 1
		if len(text) == 1 {
			return false
		}
	}
	digits := text[start:]
	i := 0
	if digits[0] == '0' {
		i = 1
	} else {
		for i < len(digits) && digits[i] >= '0' && digits[i] <= '9' {
			i++
		}
	}
	if i == 0 {
		return false
	}
	rest := digits[i:]
	if rest != "" && rest[0] == '.' {
		rest = rest[1:]
		frac := 0
		for frac < len(rest) && rest[frac] >= '0' && rest[frac] <= '9' {
			frac++
		}
		if frac == 0 {
			return false
		}
		rest = rest[frac:]
	}
	if rest != "" && (rest[0] == 'e' || rest[0] == 'E') {
		rest = rest[1:]
		if rest != "" && (rest[0] == '+' || rest[0] == '-') {
			rest = rest[1:]
		}
		exp := 0
		for exp < len(rest) && rest[exp] >= '0' && rest[exp] <= '9' {
			exp++
		}
		if exp == 0 {
			return false
		}
		rest = rest[exp:]
	}
	return rest == ""
}

func allDigits(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

// rawFields decodes a JSON object into its raw field map. ok=false when the
// value is not an object.
func rawFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if !isJSONObject(raw) {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	return fields, true
}

// field returns the raw value of a field; nil when absent or null.
func field(fields map[string]json.RawMessage, name string) json.RawMessage {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return nil
	}
	return raw
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}
