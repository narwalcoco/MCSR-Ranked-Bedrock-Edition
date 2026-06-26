// Package store wraps the queen SQLite database with typed repositories
// for players, queue entries, and matches.
//
// All write methods that span multiple tables run inside an explicit
// transaction. Single-table writes delegate directly to *sql.DB.
package store

import (
	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/db"
)

// Store bundles *db.DB plus repositories for each domain entity.
// Repositories are constructed lazily as methods so callers don't have to
// reason about ordering.
type Store struct {
	DB *db.DB
}

// New wraps an open database in a Store.
func New(database *db.DB) *Store { return &Store{DB: database} }

// tx is used internally by CreateMatch etc. to run multi-statement work.
type tx interface {
	Exec(query string, args ...any) (any, error)
}
