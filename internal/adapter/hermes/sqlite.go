package hermes

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
)

// This file implements the minimal read-only SQLite database access the
// Hermes adapter needs: open the file (applying a checkpointed-or-live WAL
// frame overlay), walk a table's B-tree, and decode row records. It replaces
// the reference's bundled SQLite library with a purpose-built reader.

const sqliteHeaderMagic = "SQLite format 3\x00"

// sqliteValue is one decoded column value.
type sqliteValue struct {
	isNull  bool
	isInt   bool
	isFloat bool
	isText  bool
	int     int64
	float   float64
	text    []byte
}

type sqliteDB struct {
	data     []byte
	pageSize int
	usable   int
	walPages map[int][]byte
}

func openSQLiteDatabase(path string) (*sqliteDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 100 || string(data[:16]) != sqliteHeaderMagic {
		return nil, errors.New("not a SQLite database")
	}
	pageSize := int(binary.BigEndian.Uint16(data[16:18]))
	if pageSize == 1 {
		pageSize = 65536
	}
	if pageSize < 512 || pageSize&(pageSize-1) != 0 {
		return nil, errors.New("invalid SQLite page size")
	}
	reserved := int(data[20])
	db := &sqliteDB{
		data:     data,
		pageSize: pageSize,
		usable:   pageSize - reserved,
		walPages: map[int][]byte{},
	}
	db.loadWAL(path + "-wal")
	return db, nil
}

// loadWAL overlays committed WAL frames onto the main file image. Checksums
// are trusted; any structural surprise drops the whole WAL.
func (db *sqliteDB) loadWAL(path string) {
	content, err := os.ReadFile(path)
	if err != nil || len(content) < 32 {
		return
	}
	magic := binary.BigEndian.Uint32(content[0:4])
	if magic != 0x377f0682 && magic != 0x377f0683 {
		return
	}
	pageSize := int(binary.BigEndian.Uint32(content[8:12]))
	if pageSize != db.pageSize {
		return
	}
	salt1 := binary.BigEndian.Uint32(content[16:20])
	salt2 := binary.BigEndian.Uint32(content[20:24])
	frameSize := 24 + pageSize
	frames := map[int][]byte{}
	committed := map[int][]byte{}
	offset := 32
	for offset+frameSize <= len(content) {
		header := content[offset : offset+24]
		pageNumber := int(binary.BigEndian.Uint32(header[0:4]))
		commitSize := binary.BigEndian.Uint32(header[4:8])
		frameSalt1 := binary.BigEndian.Uint32(header[8:12])
		frameSalt2 := binary.BigEndian.Uint32(header[12:16])
		if frameSalt1 != salt1 || frameSalt2 != salt2 {
			break
		}
		if pageNumber > 0 {
			frames[pageNumber] = content[offset+24 : offset+frameSize]
		}
		if commitSize > 0 {
			for key, page := range frames {
				committed[key] = page
			}
		}
		offset += frameSize
	}
	db.walPages = committed
}

// page returns the image of page n (1-based).
func (db *sqliteDB) page(n int) ([]byte, bool) {
	if page, ok := db.walPages[n]; ok {
		return page, true
	}
	start := (n - 1) * db.pageSize
	if n < 1 || start+db.pageSize > len(db.data) {
		return nil, false
	}
	return db.data[start : start+db.pageSize], true
}

// tableInfo locates a table's root page and column order from sqlite_master.
func (db *sqliteDB) tableInfo(name string) (root int, columns []string, ok bool) {
	var found bool
	err := db.scanTable(1, func(rowid int64, record []sqliteValue) bool {
		if len(record) < 5 || !record[0].isText || !record[1].isText || !record[3].isInt || !record[4].isText {
			return true
		}
		if string(record[0].text) != "table" || string(record[1].text) != name {
			return true
		}
		root = int(record[3].int)
		columns = parseCreateTableColumns(string(record[4].text))
		found = true
		return false
	})
	if err != nil || !found || root < 1 || len(columns) == 0 {
		return 0, nil, false
	}
	return root, columns, true
}

// scanTable walks a table B-tree in rowid order.
func (db *sqliteDB) scanTable(root int, visit func(rowid int64, record []sqliteValue) bool) error {
	visited := map[int]bool{}
	var walk func(pageNumber int) error
	walk = func(pageNumber int) error {
		if visited[pageNumber] {
			return nil
		}
		visited[pageNumber] = true
		page, ok := db.page(pageNumber)
		if !ok {
			return nil
		}
		headerOffset := 0
		if pageNumber == 1 {
			headerOffset = 100
		}
		pageType := page[headerOffset]
		cellCount := int(binary.BigEndian.Uint16(page[headerOffset+3 : headerOffset+5]))
		switch pageType {
		case 0x0D: // table leaf
			pointerStart := headerOffset + 8
			for i := 0; i < cellCount; i++ {
				cellOffset := int(binary.BigEndian.Uint16(page[pointerStart+2*i : pointerStart+2*i+2]))
				if cellOffset <= 0 || cellOffset >= len(page) {
					continue
				}
				payloadLength, n1 := readVarint(page[cellOffset:])
				rowid, n2 := readVarint(page[cellOffset+n1:])
				recordStart := cellOffset + n1 + n2
				payload, err := db.readPayload(page, recordStart, int(payloadLength))
				if err != nil {
					continue
				}
				record, err := decodeRecord(payload)
				if err != nil {
					continue
				}
				if !visit(rowid, record) {
					return errStopWalk
				}
			}
			return nil
		case 0x05: // table interior
			pointerStart := headerOffset + 12
			for i := 0; i < cellCount; i++ {
				cellOffset := int(binary.BigEndian.Uint16(page[pointerStart+2*i : pointerStart+2*i+2]))
				if cellOffset <= 0 || cellOffset+4 > len(page) {
					continue
				}
				child := int(binary.BigEndian.Uint32(page[cellOffset : cellOffset+4]))
				if child > 0 {
					if err := walk(child); err != nil {
						return err
					}
				}
			}
			rightmost := int(binary.BigEndian.Uint32(page[headerOffset+8 : headerOffset+12]))
			if rightmost > 0 {
				if err := walk(rightmost); err != nil {
					return err
				}
			}
			return nil
		default:
			return nil
		}
	}
	err := walk(root)
	if err == errStopWalk {
		return nil
	}
	return err
}

var errStopWalk = errors.New("stop walk")

// readPayload assembles a table-leaf cell payload, following overflow pages.
func (db *sqliteDB) readPayload(page []byte, start, payloadLength int) ([]byte, error) {
	if start < 0 || payloadLength < 0 {
		return nil, errors.New("bad cell")
	}
	u := db.usable
	maxLocal := u - 35
	local := payloadLength
	if payloadLength > maxLocal {
		minLocal := (u-12)*32/255 - 23
		k := minLocal + (payloadLength-minLocal)%(u-4)
		if k <= maxLocal {
			local = k
		} else {
			local = minLocal
		}
	}
	if start+local > len(page) {
		return nil, errors.New("truncated cell")
	}
	if payloadLength <= maxLocal {
		return page[start : start+payloadLength], nil
	}
	if start+local+4 > len(page) {
		return nil, errors.New("truncated overflow pointer")
	}
	payload := make([]byte, 0, payloadLength)
	payload = append(payload, page[start:start+local]...)
	overflow := int(binary.BigEndian.Uint32(page[start+local : start+local+4]))
	remaining := payloadLength - local
	for remaining > 0 && overflow > 0 {
		next, ok := db.page(overflow)
		if !ok {
			return nil, errors.New("missing overflow page")
		}
		chunk := u - 4
		if chunk > remaining {
			chunk = remaining
		}
		if 4+chunk > len(next) {
			return nil, errors.New("truncated overflow page")
		}
		payload = append(payload, next[4:4+chunk]...)
		remaining -= chunk
		overflow = int(binary.BigEndian.Uint32(next[0:4]))
	}
	if remaining > 0 {
		return nil, errors.New("broken overflow chain")
	}
	return payload, nil
}

// decodeRecord decodes a SQLite record into its column values.
func decodeRecord(payload []byte) ([]sqliteValue, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty record")
	}
	headerLength, n := readVarint(payload)
	if int(headerLength) > len(payload) || n <= 0 {
		return nil, errors.New("bad record header")
	}
	var serialTypes []int64
	offset := n
	for offset < int(headerLength) {
		serial, advanced := readVarint(payload[offset:])
		if advanced <= 0 {
			return nil, errors.New("bad serial type")
		}
		serialTypes = append(serialTypes, serial)
		offset += advanced
	}
	values := make([]sqliteValue, 0, len(serialTypes))
	body := int(headerLength)
	for _, serial := range serialTypes {
		value, size, err := decodeValue(payload, body, serial)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		body += size
	}
	return values, nil
}

func decodeValue(payload []byte, offset int, serial int64) (sqliteValue, int, error) {
	switch serial {
	case 0:
		return sqliteValue{isNull: true}, 0, nil
	case 1, 2, 3, 4, 5, 6:
		size := int(serialTypeSize(serial))
		if offset+size > len(payload) {
			return sqliteValue{}, 0, errors.New("truncated integer")
		}
		var value int64
		for i := 0; i < size; i++ {
			value = value<<8 | int64(payload[offset+i])
		}
		// Sign-extend.
		shift := uint(64 - 8*size)
		value = value << shift >> shift
		return sqliteValue{isInt: true, int: value}, size, nil
	case 7:
		if offset+8 > len(payload) {
			return sqliteValue{}, 0, errors.New("truncated float")
		}
		bits := binary.BigEndian.Uint64(payload[offset : offset+8])
		return sqliteValue{isFloat: true, float: math.Float64frombits(bits)}, 8, nil
	case 8:
		return sqliteValue{isInt: true, int: 0}, 0, nil
	case 9:
		return sqliteValue{isInt: true, int: 1}, 0, nil
	default:
		if serial >= 12 && serial%2 == 0 {
			size := int((serial - 12) / 2)
			if offset+size > len(payload) {
				return sqliteValue{}, 0, errors.New("truncated blob")
			}
			return sqliteValue{isNull: true}, size, nil
		}
		if serial >= 13 {
			size := int((serial - 13) / 2)
			if offset+size > len(payload) {
				return sqliteValue{}, 0, errors.New("truncated text")
			}
			return sqliteValue{isText: true, text: payload[offset : offset+size]}, size, nil
		}
		return sqliteValue{}, 0, errors.New("unsupported serial type")
	}
}

func serialTypeSize(serial int64) int {
	switch serial {
	case 1:
		return 1
	case 2:
		return 2
	case 3:
		return 3
	case 4:
		return 4
	case 5:
		return 6
	case 6:
		return 8
	default:
		return 0
	}
}

// readVarint decodes a SQLite varint, returning its value and byte length.
func readVarint(b []byte) (int64, int) {
	var value uint64
	for i := 0; i < 8 && i < len(b); i++ {
		value = value<<7 | uint64(b[i]&0x7f)
		if b[i]&0x80 == 0 {
			return int64(value), i + 1
		}
	}
	if len(b) >= 9 {
		value = value<<8 | uint64(b[8])
		return int64(value), 9
	}
	return int64(value), len(b)
}

// parseCreateTableColumns extracts the column names from CREATE TABLE SQL.
func parseCreateTableColumns(sql string) []string {
	open := indexByteAfter(sql, '(')
	if open < 0 {
		return nil
	}
	depth := 0
	var columns []string
	start := open + 1
	for i := open; i < len(sql); i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				if part := sql[start:i]; part != "" {
					if name, ok := columnDefinitionName(part); ok {
						columns = append(columns, name)
					}
				}
				return columns
			}
		case ',':
			if depth == 1 {
				if part := sql[start:i]; part != "" {
					if name, ok := columnDefinitionName(part); ok {
						columns = append(columns, name)
					}
				}
				start = i + 1
			}
		}
	}
	return columns
}

func indexByteAfter(s string, target byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == target {
			return i
		}
	}
	return -1
}

// columnDefinitionName returns the identifier of one column definition,
// skipping table-level constraints.
func columnDefinitionName(definition string) (string, bool) {
	trimmed := trimASCIISpace(definition)
	if trimmed == "" {
		return "", false
	}
	upper := trimmed
	// Table constraints have no leading identifier.
	for _, keyword := range []string{"PRIMARY KEY", "UNIQUE", "CHECK", "FOREIGN KEY", "CONSTRAINT"} {
		if len(upper) >= len(keyword) && equalFoldASCII(upper[:len(keyword)], keyword) {
			return "", false
		}
	}
	switch trimmed[0] {
	case '"', '`', '\'':
		quote := trimmed[0]
		for i := 1; i < len(trimmed); i++ {
			if trimmed[i] == quote {
				if i+1 < len(trimmed) && trimmed[i+1] == quote {
					i++
					continue
				}
				return trimmed[1:i], true
			}
		}
		return trimmed[1:], true
	case '[':
		if end := indexByteAfter(trimmed, ']'); end > 0 {
			return trimmed[1:end], true
		}
		return "", false
	default:
		end := 0
		for end < len(trimmed) && trimmed[end] != ' ' && trimmed[end] != '\t' && trimmed[end] != '\n' {
			end++
		}
		return trimmed[:end], true
	}
}

func trimASCIISpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
