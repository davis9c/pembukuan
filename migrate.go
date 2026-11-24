package main

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func runMigrations(db *sql.DB) error {
	// ensure schema_version table
	db.Exec("CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)")

	// read all migration files
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	type mig struct {
		ver  int
		data string
	}
	var migs []mig
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		verStr := strings.TrimSuffix(strings.SplitN(e.Name(), "_", 2)[0], ".sql")
		ver, err := strconv.Atoi(verStr)
		if err != nil {
			continue
		}
		data, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		migs = append(migs, mig{ver: ver, data: string(data)})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].ver < migs[j].ver })

	// get current version
	var current int
	db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&current)

	// apply pending migrations
	for _, m := range migs {
		if m.ver <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.ver, err)
		}
		// split by semicolon; skip empty and comment-only segments
		for _, stmt := range strings.Split(m.data, ";") {
			st := strings.TrimSpace(stmt)
			// remove leading SQL comment lines
			for strings.HasPrefix(st, "--") {
				if idx := strings.IndexByte(st, '\n'); idx >= 0 {
					st = strings.TrimSpace(st[idx+1:])
				} else {
					st = ""
				}
			}
			if st == "" {
				continue
			}
			if _, err := tx.Exec(st); err != nil {
				errStr := err.Error()
				// squat: skip duplicate column / table errors for idempotent re-runs
				if strings.Contains(errStr, "duplicate column name") ||
					strings.Contains(errStr, "already exists") ||
					strings.Contains(errStr, "duplicate") {
					continue
				}
				tx.Rollback()
				return fmt.Errorf("migration %d: %w (sql: %s)", m.ver, err, st)
			}
		}
		if _, err := tx.Exec("INSERT INTO schema_version (version) VALUES (?)", m.ver); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.ver, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.ver, err)
		}
	}
	return nil
}
