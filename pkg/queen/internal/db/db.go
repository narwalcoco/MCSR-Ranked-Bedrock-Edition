// Package db manages the SQLite database used by the queen service.
//
// It opens a single connection, ensures foreign keys are enabled, and runs
// any pending migrations from the embedded migrations/ directory.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB bundles a *sql.DB with the path it was opened from.
type DB struct {
	*sql.DB
	Path string
}

// Open opens (or creates) the SQLite database at path and applies any
// pending migrations.
//
// Pragmas applied:
//   - foreign_keys = ON  (so future migrations with FKs behave correctly)
//   - journal_mode = WAL (better concurrency for a small backend service)
//   - busy_timeout = 5s  (avoid spurious "database is locked" errors)
//
// Timestamp strategy: all schema timestamps are INTEGER columns holding
// Unix epoch seconds (via unixepoch()), which modernc.org/sqlite scans
// directly into Go's time.Time. No _time_format / _loc connection params.
func Open(ctx context.Context, path string) (*DB, error) {
	conn, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	// SQLite is single-writer; keep the pool small to avoid contention.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	pCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := conn.PingContext(pCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	d := &DB{DB: conn, Path: path}
	if err := d.applyMigrations(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return d, nil
}

// buildDSN returns the connection string for the given SQLite path.
// In-memory uses journal_mode=memory; on-disk uses WAL for concurrency.
func buildDSN(path string) string {
	const sharedParams = "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		return ":memory:" + sharedParams + "&_pragma=journal_mode(memory)"
	}
	return "file:" + filepath.ToSlash(path) + sharedParams + "&_pragma=journal_mode(wal)"
}

// SchemaVersion returns the highest applied migration version, or 0 if none.
func (d *DB) SchemaVersion(ctx context.Context) (int, error) {
	// The migrations runner creates this table.
	row := d.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version")
	var v int
	if err := row.Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func (d *DB) applyMigrations(ctx context.Context) error {
	applied, err := d.SchemaVersion(ctx)
	if err != nil {
		// If the table doesn't exist yet, treat it as version 0.
		if !isNoTable(err) {
			return err
		}
		applied = 0
	}

	files, err := loadMigrations(migrationsFS)
	if err != nil {
		return err
	}

	for _, m := range files {
		if m.Version <= applied {
			continue
		}
		slog.Info("applying migration", "version", m.Version, "description", m.Description)
		if err := d.execMigration(ctx, m); err != nil {
			return fmt.Errorf("apply %04d (%s): %w", m.Version, m.Description, err)
		}
	}
	return nil
}

type migration struct {
	Version     int
	Description string
	SQL         string
}

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		raw, err := fs.ReadFile(fsys, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		version, desc, err := parseMigrationName(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{
			Version:     version,
			Description: desc,
			SQL:         string(raw),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// parseMigrationName expects "<version4>_<description>.sql".
func parseMigrationName(name string) (int, string, error) {
	idx := strings.IndexByte(name, '_')
	if idx <= 0 || len(name) < len(".sql") || !strings.HasSuffix(name, ".sql") {
		return 0, "", fmt.Errorf("invalid migration filename %q (want NNNN_description.sql)", name)
	}
	v, err := strconv.Atoi(name[:idx])
	if err != nil {
		return 0, "", fmt.Errorf("invalid migration version in %q: %w", name, err)
	}
	desc := strings.TrimSuffix(name[idx+1:], ".sql")
	return v, desc, nil
}

func (d *DB) execMigration(ctx context.Context, m migration) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_version (version, description) VALUES (?, ?)",
		m.Version, m.Description,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func isNoTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		errors.Is(err, sql.ErrNoRows) ||
		strings.Contains(msg, "schema_version")
}
