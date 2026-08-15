package statusline

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Hook is the Claude Code statusline stdin payload (the reference StatuslineHook).
type Hook struct {
	SessionID      string
	TranscriptPath string
	Model          HookModel
	Cost           *HookCost
	ContextWindow  *HookContext
	Effort         *HookEffort
}

// HookModel identifies the model reported by the hook.
type HookModel struct {
	ID          *string
	DisplayName string
}

// HookCost carries Claude Code's own cost accounting.
type HookCost struct {
	TotalCostUSD float64
}

// HookContext carries the context-window usage reported by the hook.
type HookContext struct {
	TotalInputTokens  uint64
	ContextWindowSize uint64
}

// HookEffort carries the reasoning effort level.
type HookEffort struct {
	Level string
}

// ParseHook deserializes the hook JSON with serde_json-compatible error
// messages ("expected ident at line 1 column 2", "missing field `session_id`
// at line 1 column 2", "invalid type: ..."), because the reference surfaces
// those messages verbatim through the Invalid input format CliError.
func ParseHook(input string) (*Hook, error) {
	p := &serdeParser{data: []byte(input)}
	hook := &Hook{}
	if err := p.parseStatuslineHook(hook); err != nil {
		return nil, err
	}
	p.skipWS()
	if p.pos < len(p.data) {
		return nil, p.peekErr(codeTrailingChars)
	}
	return hook, nil
}

// serde_json error codes.
const (
	codeExpectedValue      = "expected value"
	codeExpectedIdent      = "expected ident"
	codeExpectedColon      = "expected `:`"
	codeExpectedCommaOrEnd = "expected `,` or `}`"
	codeExpectedCommaOrBrk = "expected `,` or `]`"
	codeTrailingComma      = "trailing comma"
	codeTrailingChars      = "trailing characters"
	codeKeyMustBeString    = "key must be a string"
	codeEOFValue           = "EOF while parsing a value"
	codeEOFObject          = "EOF while parsing an object"
	codeEOFList            = "EOF while parsing a list"
	codeEOFString          = "EOF while parsing a string"
	codeInvalidNumber      = "invalid number"
	codeNumberOutOfRange   = "number out of range"
	codeInvalidEscape      = "invalid escape"
	codeControlChar        = `control character (\u0000-\u001F) found while parsing a string`
	codeLoneSurrogate      = "lone leading surrogate in hex escape"
	codeUnexpectedHexEnd   = "unexpected end of hex escape"
)

type serdeParser struct {
	data []byte
	pos  int
}

// errAt renders a serde_json style position: the column counts consumed bytes
// since the last newline (serde reports the 1-based column of the last
// consumed byte; peek-style errors pass index+1).
func (p *serdeParser) errAt(index int, code string) error {
	line, lastNL := 1, -1
	limit := index
	if limit > len(p.data) {
		limit = len(p.data)
	}
	for i := 0; i < limit; i++ {
		if p.data[i] == '\n' {
			line++
			lastNL = i
		}
	}
	return fmt.Errorf("%s at line %d column %d", code, line, index-lastNL-1)
}

// peekErr reports an error at the peeked (not yet consumed) byte.
func (p *serdeParser) peekErr(code string) error {
	return p.errAt(p.pos+1, code)
}

func (p *serdeParser) skipWS() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\n', '\t', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *serdeParser) peekByte() (byte, bool) {
	if p.pos < len(p.data) {
		return p.data[p.pos], true
	}
	return 0, false
}

func (p *serdeParser) invalidTypeAt(index int, found, expected string) error {
	return p.errAt(index, "invalid type: "+found+", expected "+expected)
}

// ---------------------------------------------------------------------------
// Struct schemas
// ---------------------------------------------------------------------------

// hookField is one struct member. parse consumes the field value after the
// ':' has been read; the receiver it mutates is captured by the closure.
type hookField struct {
	name string
	// required fields raise "missing field" errors; optional ones do not.
	required bool
	parse    func(p *serdeParser) error
}

func statuslineHookFields(hook *Hook) []hookField {
	return []hookField{
		{name: "session_id", required: true, parse: func(p *serdeParser) error {
			return p.parseStringInto(&hook.SessionID)
		}},
		{name: "transcript_path", required: true, parse: func(p *serdeParser) error {
			return p.parseStringInto(&hook.TranscriptPath)
		}},
		{name: "model", required: true, parse: func(p *serdeParser) error {
			return p.parseHookModel(&hook.Model)
		}},
		{name: "cost", parse: func(p *serdeParser) error {
			hook.Cost = nil
			return p.parseOptionalStruct("HookCost", func() error {
				hook.Cost = &HookCost{}
				return p.parseHookCost(hook.Cost)
			})
		}},
		{name: "context_window", parse: func(p *serdeParser) error {
			hook.ContextWindow = nil
			return p.parseOptionalStruct("HookContext", func() error {
				hook.ContextWindow = &HookContext{}
				return p.parseHookContext(hook.ContextWindow)
			})
		}},
		{name: "effort", parse: func(p *serdeParser) error {
			hook.Effort = nil
			return p.parseOptionalStruct("HookEffort", func() error {
				hook.Effort = &HookEffort{}
				return p.parseHookEffort(hook.Effort)
			})
		}},
	}
}

func (p *serdeParser) parseStatuslineHook(hook *Hook) error {
	return p.parseStruct("StatuslineHook", statuslineHookFields(hook), true)
}

func (p *serdeParser) parseHookModel(model *HookModel) error {
	return p.parseStruct("HookModel", []hookField{
		{name: "id", parse: func(p *serdeParser) error {
			return p.parseOptStringInto(&model.ID)
		}},
		{name: "display_name", required: true, parse: func(p *serdeParser) error {
			return p.parseStringInto(&model.DisplayName)
		}},
	}, false)
}

func (p *serdeParser) parseHookCost(cost *HookCost) error {
	return p.parseStruct("HookCost", []hookField{
		{name: "total_cost_usd", required: true, parse: func(p *serdeParser) error {
			value, err := p.parseF64("f64")
			if err != nil {
				return err
			}
			cost.TotalCostUSD = value
			return nil
		}},
	}, false)
}

func (p *serdeParser) parseHookContext(context *HookContext) error {
	return p.parseStruct("HookContext", []hookField{
		{name: "total_input_tokens", required: true, parse: func(p *serdeParser) error {
			value, err := p.parseU64("u64")
			if err != nil {
				return err
			}
			context.TotalInputTokens = value
			return nil
		}},
		{name: "context_window_size", required: true, parse: func(p *serdeParser) error {
			value, err := p.parseU64("u64")
			if err != nil {
				return err
			}
			context.ContextWindowSize = value
			return nil
		}},
	}, false)
}

func (p *serdeParser) parseHookEffort(effort *HookEffort) error {
	return p.parseStruct("HookEffort", []hookField{
		{name: "level", required: true, parse: func(p *serdeParser) error {
			return p.parseStringInto(&effort.Level)
		}},
	}, false)
}

// parseOptionalStruct accepts null (leaving the target unset) or a struct.
func (p *serdeParser) parseOptionalStruct(name string, parseBody func() error) error {
	p.skipWS()
	b, ok := p.peekByte()
	if ok && b == 'n' {
		return p.parseIdent("null")
	}
	if err := p.expectStructShape(name); err != nil {
		return err
	}
	return parseBody()
}

// expectStructShape verifies the next value opens a struct ({ or [),
// consuming nothing; scalars raise fully described invalid-type errors.
func (p *serdeParser) expectStructShape(name string) error {
	p.skipWS()
	b, ok := p.peekByte()
	if !ok {
		return p.errAt(p.pos, codeEOFValue)
	}
	if b == '{' || b == '[' {
		return nil
	}
	found, err := p.parseScalarFound()
	if err != nil {
		return err
	}
	return p.invalidTypeAt(p.pos, found, "struct "+name)
}

// parseStruct deserializes one struct-shaped value in map or seq form.
// Unknown map keys are skipped like serde's default visitor. top marks the
// outermost struct, whose seq-form invalid-length error is positioned after
// the closing bracket, matching the reference's fix_position behavior.
func (p *serdeParser) parseStruct(name string, fields []hookField, top bool) error {
	p.skipWS()
	b, ok := p.peekByte()
	if !ok {
		return p.errAt(p.pos, codeEOFValue)
	}
	switch b {
	case '{':
		return p.parseStructMap(fields)
	case '[':
		return p.parseStructSeq(name, fields, top)
	}
	found, err := p.parseScalarFound()
	if err != nil {
		return err
	}
	return p.invalidTypeAt(p.pos, found, "struct "+name)
}

func (p *serdeParser) parseStructMap(fields []hookField) error {
	p.pos++ // consume '{'
	seen := map[string]bool{}
	first := true
	for {
		p.skipWS()
		b, ok := p.peekByte()
		if !ok {
			return p.errAt(p.pos, codeEOFObject)
		}
		if b == '}' {
			p.pos++
			break
		}
		if !first {
			if b != ',' {
				return p.peekErr(codeExpectedCommaOrEnd)
			}
			p.pos++ // consume ','
			p.skipWS()
			b, ok = p.peekByte()
			if !ok {
				return p.errAt(p.pos, codeEOFObject)
			}
			if b == '}' {
				return p.peekErr(codeTrailingComma)
			}
		}
		first = false
		if b != '"' {
			return p.peekErr(codeKeyMustBeString)
		}
		key, err := p.parseStringBody()
		if err != nil {
			return err
		}
		if seen[key] {
			return p.errAt(p.pos, "duplicate field `"+key+"`")
		}
		p.skipWS()
		cb, ok := p.peekByte()
		if !ok {
			return p.errAt(p.pos, codeEOFObject)
		}
		if cb != ':' {
			return p.peekErr(codeExpectedColon)
		}
		p.pos++ // consume ':'
		handled := false
		for i := range fields {
			if fields[i].name == key {
				if err := fields[i].parse(p); err != nil {
					return err
				}
				handled = true
				break
			}
		}
		if !handled {
			if err := p.skipValue(); err != nil {
				return err
			}
		}
		seen[key] = true
	}
	// Missing-field check in declaration order, positioned after '}'.
	for i := range fields {
		if fields[i].required && !seen[fields[i].name] {
			return p.errAt(p.pos, "missing field `"+fields[i].name+"`")
		}
	}
	return nil
}

func (p *serdeParser) parseStructSeq(name string, fields []hookField, top bool) error {
	p.pos++ // consume '['
	parsed := 0
	for i := range fields {
		if i > 0 {
			p.skipWS()
			b, ok := p.peekByte()
			if !ok {
				return p.errAt(p.pos, codeEOFList)
			}
			if b == ']' {
				return p.seqInvalidLength(name, parsed, len(fields), top)
			}
			if b != ',' {
				return p.peekErr(codeExpectedCommaOrBrk)
			}
			p.pos++
		}
		p.skipWS()
		b, ok := p.peekByte()
		if !ok {
			return p.errAt(p.pos, codeEOFList)
		}
		if b == ']' {
			return p.seqInvalidLength(name, parsed, len(fields), top)
		}
		if err := fields[i].parse(p); err != nil {
			return err
		}
		parsed++
	}
	// Drain to the closing bracket; surplus elements are trailing characters.
	p.skipWS()
	b, ok := p.peekByte()
	if !ok {
		return p.errAt(p.pos, codeEOFList)
	}
	if b == ']' {
		p.pos++
		return nil
	}
	return p.peekErr(codeTrailingChars)
}

func (p *serdeParser) seqInvalidLength(name string, got, want int, top bool) error {
	_ = top
	message := fmt.Sprintf("invalid length %d, expected struct %s with %d elements", got, name, want)
	// The reference stamps the error after end_seq consumes the bracket.
	p.pos++
	return p.errAt(p.pos, message)
}

// ---------------------------------------------------------------------------
// Scalars
// ---------------------------------------------------------------------------

// parseScalarFound parses any non-object/non-array scalar and returns its
// serde Unexpected description ("null", "boolean `true`", "integer `5`",
// "floating point `1.5`", "string \"x\"").
func (p *serdeParser) parseScalarFound() (string, error) {
	b, ok := p.peekByte()
	if !ok {
		return "", p.errAt(p.pos, codeEOFValue)
	}
	switch {
	case b == '"':
		s, err := p.parseStringBody()
		if err != nil {
			return "", err
		}
		return "string " + rustDebugQuote(s), nil
	case b == 't' || b == 'f':
		lit := "true"
		if b == 'f' {
			lit = "false"
		}
		if err := p.parseIdent(lit); err != nil {
			return "", err
		}
		return "boolean `" + lit + "`", nil
	case b == 'n':
		if err := p.parseIdent("null"); err != nil {
			return "", err
		}
		return "null", nil
	case b == '-' || (b >= '0' && b <= '9'):
		return p.parseNumberFound()
	}
	p.pos++
	return "", p.errAt(p.pos, codeExpectedValue)
}

func (p *serdeParser) parseIdent(lit string) error {
	for i := 0; i < len(lit); i++ {
		if p.pos >= len(p.data) {
			return p.errAt(p.pos, codeEOFValue)
		}
		if p.data[p.pos] != lit[i] {
			p.pos++
			return p.errAt(p.pos, codeExpectedIdent)
		}
		p.pos++
	}
	return nil
}

type serdeNumber struct {
	isFloat     bool
	negativeInt bool
	u64Value    uint64
	i64Value    int64
	f64Value    float64
}

func (n serdeNumber) found() string {
	if n.isFloat {
		return "floating point `" + strconv.FormatFloat(n.f64Value, 'g', -1, 64) + "`"
	}
	if n.negativeInt {
		return fmt.Sprintf("integer `%d`", n.i64Value)
	}
	return fmt.Sprintf("integer `%d`", n.u64Value)
}

// parseNumber scans a JSON number. Errors are positioned at the last consumed
// byte like serde_json's number parser.
func (p *serdeParser) parseNumber() (serdeNumber, error) {
	start := p.pos
	n := serdeNumber{}
	if p.pos < len(p.data) && p.data[p.pos] == '-' {
		p.pos++
		if p.pos >= len(p.data) {
			return n, p.errAt(p.pos, codeEOFValue)
		}
		n.negativeInt = true
	}
	b := p.data[p.pos]
	switch {
	case b == '0':
		p.pos++
		if p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
			return n, p.errAt(p.pos, codeInvalidNumber)
		}
	case b >= '1' && b <= '9':
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	default:
		p.pos++
		return n, p.errAt(p.pos, codeInvalidNumber)
	}
	if p.pos < len(p.data) && p.data[p.pos] == '.' {
		n.isFloat = true
		p.pos++
		if p.pos >= len(p.data) || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
			return n, p.errAt(p.pos+1, codeInvalidNumber)
		}
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		n.isFloat = true
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		if p.pos >= len(p.data) || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
			return n, p.errAt(p.pos+1, codeInvalidNumber)
		}
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	text := string(p.data[start:p.pos])
	if n.isFloat {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return n, p.errAt(p.pos, codeNumberOutOfRange)
		}
		n.f64Value = f
		return n, nil
	}
	if n.negativeInt {
		i, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			n.i64Value = i
			return n, nil
		}
		f, _ := strconv.ParseFloat(text, 64)
		n.isFloat, n.f64Value = true, f
		return n, nil
	}
	u, err := strconv.ParseUint(text, 10, 64)
	if err == nil {
		n.u64Value = u
		return n, nil
	}
	f, _ := strconv.ParseFloat(text, 64)
	n.isFloat, n.f64Value = true, f
	return n, nil
}

func (p *serdeParser) parseNumberFound() (string, error) {
	n, err := p.parseNumber()
	if err != nil {
		return "", err
	}
	return n.found(), nil
}

func (p *serdeParser) parseStringInto(dst *string) error {
	s, err := p.parseString("a string")
	if err != nil {
		return err
	}
	*dst = s
	return nil
}

func (p *serdeParser) parseOptStringInto(dst **string) error {
	p.skipWS()
	if b, ok := p.peekByte(); ok && b == 'n' {
		if err := p.parseIdent("null"); err != nil {
			return err
		}
		*dst = nil
		return nil
	}
	s, err := p.parseString("a string")
	if err != nil {
		return err
	}
	*dst = &s
	return nil
}

// parseString parses a string, or raises serde invalid-type errors for other
// shapes: sequences/maps are reported before their opening bracket is
// consumed, scalars after they are consumed.
func (p *serdeParser) parseString(expected string) (string, error) {
	p.skipWS()
	b, ok := p.peekByte()
	if !ok {
		return "", p.errAt(p.pos, codeEOFValue)
	}
	switch b {
	case '"':
		return p.parseStringBody()
	case '{':
		return "", p.invalidTypeAt(p.pos, "map", expected)
	case '[':
		return "", p.invalidTypeAt(p.pos, "sequence", expected)
	}
	found, err := p.parseScalarFound()
	if err != nil {
		return "", err
	}
	return "", p.invalidTypeAt(p.pos, found, expected)
}

func (p *serdeParser) parseU64(expected string) (uint64, error) {
	p.skipWS()
	b, ok := p.peekByte()
	if !ok {
		return 0, p.errAt(p.pos, codeEOFValue)
	}
	switch b {
	case '{':
		return 0, p.invalidTypeAt(p.pos, "map", expected)
	case '[':
		return 0, p.invalidTypeAt(p.pos, "sequence", expected)
	}
	if b == '-' || (b >= '0' && b <= '9') {
		n, err := p.parseNumber()
		if err != nil {
			return 0, err
		}
		if n.isFloat {
			return 0, p.invalidTypeAt(p.pos, n.found(), expected)
		}
		if n.negativeInt {
			return 0, p.errAt(p.pos, fmt.Sprintf("invalid value: integer `%d`, expected %s", n.i64Value, expected))
		}
		return n.u64Value, nil
	}
	found, err := p.parseScalarFound()
	if err != nil {
		return 0, err
	}
	return 0, p.invalidTypeAt(p.pos, found, expected)
}

func (p *serdeParser) parseF64(expected string) (float64, error) {
	p.skipWS()
	b, ok := p.peekByte()
	if !ok {
		return 0, p.errAt(p.pos, codeEOFValue)
	}
	switch b {
	case '{':
		return 0, p.invalidTypeAt(p.pos, "map", expected)
	case '[':
		return 0, p.invalidTypeAt(p.pos, "sequence", expected)
	}
	if b == '-' || (b >= '0' && b <= '9') {
		n, err := p.parseNumber()
		if err != nil {
			return 0, err
		}
		if n.isFloat {
			return n.f64Value, nil
		}
		if n.negativeInt {
			return float64(n.i64Value), nil
		}
		return float64(n.u64Value), nil
	}
	found, err := p.parseScalarFound()
	if err != nil {
		return 0, err
	}
	return 0, p.invalidTypeAt(p.pos, found, expected)
}

func (p *serdeParser) parseStringBody() (string, error) {
	p.pos++ // consume opening '"'
	var out []byte
	for {
		if p.pos >= len(p.data) {
			return "", p.errAt(p.pos, codeEOFString)
		}
		b := p.data[p.pos]
		switch {
		case b == '"':
			p.pos++
			return string(out), nil
		case b == '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return "", p.errAt(p.pos, codeEOFString)
			}
			esc := p.data[p.pos]
			p.pos++
			switch esc {
			case '"', '\\', '/':
				out = append(out, esc)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				r, err := p.parseUnicodeEscape()
				if err != nil {
					return "", err
				}
				out = utf8.AppendRune(out, r)
			default:
				return "", p.errAt(p.pos, codeInvalidEscape)
			}
		case b < 0x20:
			return "", p.errAt(p.pos+1, codeControlChar)
		default:
			out = append(out, b)
			p.pos++
		}
	}
}

func (p *serdeParser) parseUnicodeEscape() (rune, error) {
	unit, err := p.parseHex4()
	if err != nil {
		return 0, err
	}
	if unit >= 0xDC00 && unit <= 0xDFFF {
		return 0, p.errAt(p.pos, codeLoneSurrogate)
	}
	if unit >= 0xD800 && unit <= 0xDBFF {
		if p.pos >= len(p.data) || p.data[p.pos] != '\\' {
			return 0, p.errAt(p.pos, codeUnexpectedHexEnd)
		}
		p.pos++
		if p.pos >= len(p.data) || p.data[p.pos] != 'u' {
			return 0, p.errAt(p.pos, codeUnexpectedHexEnd)
		}
		p.pos++
		low, err := p.parseHex4()
		if err != nil {
			return 0, err
		}
		if low < 0xDC00 || low > 0xDFFF {
			return 0, p.errAt(p.pos, codeLoneSurrogate)
		}
		return rune(0x10000 + (unit-0xD800)<<10 + (low - 0xDC00)), nil
	}
	return rune(unit), nil
}

func (p *serdeParser) parseHex4() (int, error) {
	value := 0
	for i := 0; i < 4; i++ {
		if p.pos >= len(p.data) {
			return 0, p.errAt(p.pos, codeEOFString)
		}
		b := p.data[p.pos]
		var digit int
		switch {
		case b >= '0' && b <= '9':
			digit = int(b - '0')
		case b >= 'a' && b <= 'f':
			digit = int(b-'a') + 10
		case b >= 'A' && b <= 'F':
			digit = int(b-'A') + 10
		default:
			p.pos++
			return 0, p.errAt(p.pos, codeInvalidEscape)
		}
		value = value<<4 | digit
		p.pos++
	}
	return value, nil
}

// skipValue consumes any JSON value (unknown fields use IgnoredAny).
func (p *serdeParser) skipValue() error {
	p.skipWS()
	b, ok := p.peekByte()
	if !ok {
		return p.errAt(p.pos, codeEOFValue)
	}
	switch {
	case b == '"':
		_, err := p.parseStringBody()
		return err
	case b == 't' || b == 'f':
		lit := "true"
		if b == 'f' {
			lit = "false"
		}
		return p.parseIdent(lit)
	case b == 'n':
		return p.parseIdent("null")
	case b == '-' || (b >= '0' && b <= '9'):
		_, err := p.parseNumber()
		return err
	case b == '[':
		return p.skipSeq()
	case b == '{':
		return p.skipMap()
	}
	p.pos++
	return p.errAt(p.pos, codeExpectedValue)
}

func (p *serdeParser) skipSeq() error {
	p.pos++ // '['
	first := true
	for {
		p.skipWS()
		b, ok := p.peekByte()
		if !ok {
			return p.errAt(p.pos, codeEOFList)
		}
		if b == ']' {
			p.pos++
			return nil
		}
		if !first {
			if b != ',' {
				return p.peekErr(codeExpectedCommaOrBrk)
			}
			p.pos++
			p.skipWS()
			b, ok = p.peekByte()
			if !ok {
				return p.errAt(p.pos, codeEOFList)
			}
			if b == ']' {
				return p.peekErr(codeTrailingComma)
			}
		}
		first = false
		if err := p.skipValue(); err != nil {
			return err
		}
	}
}

func (p *serdeParser) skipMap() error {
	p.pos++ // '{'
	first := true
	for {
		p.skipWS()
		b, ok := p.peekByte()
		if !ok {
			return p.errAt(p.pos, codeEOFObject)
		}
		if b == '}' {
			p.pos++
			return nil
		}
		if !first {
			if b != ',' {
				return p.peekErr(codeExpectedCommaOrEnd)
			}
			p.pos++
			p.skipWS()
			b, ok = p.peekByte()
			if !ok {
				return p.errAt(p.pos, codeEOFObject)
			}
			if b == '}' {
				return p.peekErr(codeTrailingComma)
			}
		}
		first = false
		if b != '"' {
			return p.peekErr(codeKeyMustBeString)
		}
		if _, err := p.parseStringBody(); err != nil {
			return err
		}
		p.skipWS()
		cb, ok := p.peekByte()
		if !ok {
			return p.errAt(p.pos, codeEOFObject)
		}
		if cb != ':' {
			return p.peekErr(codeExpectedColon)
		}
		p.pos++
		if err := p.skipValue(); err != nil {
			return err
		}
	}
}

// rustDebugQuote renders a string the way Rust's Debug (and serde's
// Unexpected::Str) escapes it.
func rustDebugQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case 0:
			b.WriteString("\\0")
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, "\\u{%x}", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
