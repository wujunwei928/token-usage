// Package zcode ports the zcode adapter: analytics database discovery under
// ~/.zcode/cli (ZCODE_DATA_DIR overrides), the SQLite usage loader, and
// report summarization.
package zcode

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DataDirEnv names the override for the zcode data directory.
const DataDirEnv = "ZCODE_DATA_DIR"

// DBFilename is the analytics database file name inside the data directory.
const DBFilename = "db.sqlite"

// dataDir resolves the data root: the env override when set, otherwise
// ~/.zcode/cli (~/.zcode/v2 is config only and carries no usage data).
func dataDir() string {
	if raw, ok := os.LookupEnv(DataDirEnv); ok {
		return raw
	}
	home := agentHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".zcode", "cli")
}

// DBPaths returns the analytics databases to read, in order; only paths whose
// db/db.sqlite exists are kept.
func DBPaths() []string {
	root := dataDir()
	if root == "" {
		return nil
	}
	dbPath := filepath.Join(root, "db", DBFilename)
	if info, err := os.Stat(dbPath); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return []string{dbPath}
}

// HasData reports whether a zcode analytics database exists, even when date
// filters leave no entries.
func HasData() bool {
	return len(DBPaths()) > 0
}

// agentHomeDir mirrors the reference home_dir(): HOME, then USERPROFILE, then
// HOMEDRIVE+HOMEPATH; empty when none resolve.
func agentHomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		return profile
	}
	drive, path := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH")
	if drive != "" && path != "" {
		return drive + path
	}
	return ""
}

// openDB opens the analytics database without ever writing to it. A read-only
// open fails when an unchecked WAL needs recovery, so that attempt falls back
// to a query-only connection.
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err == nil {
		err = db.Ping()
	}
	if err == nil {
		return db, nil
	}
	if db != nil {
		db.Close()
	}
	db, err = sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=query_only")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
