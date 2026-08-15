package core

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// JSON value model matching serde_json pretty output: object keys sorted
// alphabetically, floats in ryu style (always with a decimal point).

type jKind int

const (
	jNull jKind = iota
	jBool
	jUint
	jInt
	jFloat
	jString
	jObject
	jArray
)

// J is a JSON value; build with the J* constructors.
type J struct {
	kind  jKind
	b     bool
	u     uint64
	i     int64
	f     float64
	s     string
	keys  []string
	vals  map[string]J
	items []J
}

// JNullV is the JSON null value.
var JNullV = J{kind: jNull}

// JBoolV builds a boolean.
func JBoolV(b bool) J { return J{kind: jBool, b: b} }

// JUintV builds an unsigned integer.
func JUintV(u uint64) J { return J{kind: jUint, u: u} }

// JIntV builds a signed integer.
func JIntV(i int64) J { return J{kind: jInt, i: i} }

// JFloatV builds a float formatted like Rust's serde_json.
func JFloatV(f float64) J { return J{kind: jFloat, f: f} }

// JStrV builds a string.
func JStrV(s string) J { return J{kind: jString, s: s} }

// JOptStrV builds a string or null.
func JOptStrV(s *string) J {
	if s == nil {
		return JNullV
	}
	return JStrV(*s)
}

// JObjV builds an object from alternating key/value pairs; duplicate keys
// keep the last value.
func JObjV(pairs ...any) J {
	obj := J{kind: jObject, vals: map[string]J{}}
	for i := 0; i+1 < len(pairs); i += 2 {
		key := pairs[i].(string)
		val, ok := pairs[i+1].(J)
		if !ok {
			val = JNullV
		}
		if _, exists := obj.vals[key]; !exists {
			obj.keys = append(obj.keys, key)
		}
		obj.vals[key] = val
	}
	sort.Strings(obj.keys)
	return obj
}

// JArrV builds an array.
func JArrV(items ...J) J { return J{kind: jArray, items: items} }

// JOrdObjV builds an object preserving insertion order (used where the
// reference emits an ordered map, e.g. --sections JSON).
func JOrdObjV(pairs ...any) J {
	obj := J{kind: jObject, vals: map[string]J{}}
	for i := 0; i+1 < len(pairs); i += 2 {
		key := pairs[i].(string)
		val, ok := pairs[i+1].(J)
		if !ok {
			val = JNullV
		}
		if _, exists := obj.vals[key]; !exists {
			obj.keys = append(obj.keys, key)
		}
		obj.vals[key] = val
	}
	return obj
}

// SerializeJ renders the value in serde_json pretty style (two-space indent).
func SerializeJ(v J) string {
	var sb strings.Builder
	writeJ(&sb, v, 0)
	return sb.String()
}

func writeJ(sb *strings.Builder, v J, depth int) {
	indent := strings.Repeat("  ", depth)
	switch v.kind {
	case jNull:
		sb.WriteString("null")
	case jBool:
		sb.WriteString(strconv.FormatBool(v.b))
	case jUint:
		sb.WriteString(strconv.FormatUint(v.u, 10))
	case jInt:
		sb.WriteString(strconv.FormatInt(v.i, 10))
	case jFloat:
		sb.WriteString(RustF64(v.f))
	case jString:
		writeJString(sb, v.s)
	case jObject:
		if len(v.keys) == 0 {
			sb.WriteString("{}")
			return
		}
		sb.WriteString("{\n")
		for i, key := range v.keys {
			sb.WriteString(indent + "  ")
			writeJString(sb, key)
			sb.WriteString(": ")
			writeJ(sb, v.vals[key], depth+1)
			if i+1 < len(v.keys) {
				sb.WriteString(",")
			}
			sb.WriteString("\n")
		}
		sb.WriteString(indent + "}")
	case jArray:
		if len(v.items) == 0 {
			sb.WriteString("[]")
			return
		}
		sb.WriteString("[\n")
		for i, item := range v.items {
			sb.WriteString(indent + "  ")
			writeJ(sb, item, depth+1)
			if i+1 < len(v.items) {
				sb.WriteString(",")
			}
			sb.WriteString("\n")
		}
		sb.WriteString(indent + "]")
	}
}

func writeJString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		case '\r':
			sb.WriteString("\\r")
		case '\t':
			sb.WriteString("\\t")
		case '\b':
			sb.WriteString("\\b")
		case '\f':
			sb.WriteString("\\f")
		default:
			if r < 0x20 {
				sb.WriteString("\\u")
				sb.WriteString(strings.ToLower(strconv.FormatInt(int64(r)|0x10000, 16))[1:])
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
}

// RustF64 formats a float the way Rust's serde_json (ryu) does: shortest
// round-trip digits, always with a fractional part in fixed notation, and
// exponent notation (no plus sign, no leading exponent zeros) outside
// [1e-5, 1e16).
func RustF64(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "null"
	}
	abs := math.Abs(v)
	var s string
	if abs != 0 && (abs < 1e-5 || abs >= 1e16) {
		s = strconv.FormatFloat(v, 'e', -1, 64)
		// "1e+30" -> "1e30"; "1.5e-06" -> "1.5e-6"
		if i := strings.IndexAny(s, "eE"); i >= 0 {
			mantissa, exp := s[:i], s[i+1:]
			neg := strings.HasPrefix(exp, "-")
			exp = strings.TrimLeft(strings.TrimLeft(exp, "+-"), "0")
			if exp == "" {
				exp = "0"
			}
			if neg {
				exp = "-" + exp
			}
			if !strings.ContainsAny(mantissa, ".") {
				mantissa += ".0"
			}
			s = mantissa + "e" + exp
		}
		return s
	}
	s = strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
