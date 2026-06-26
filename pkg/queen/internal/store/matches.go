package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MatchStatus values mirror the SQLite CHECK constraint on matches.status.
const (
	MatchStatusPending   = "pending"
	MatchStatusActive    = "active"
	MatchStatusCompleted = "completed"
	MatchStatusCancelled = "cancelled"
	MatchStatusForfeited = "forfeited"
)

// Match is a queued/active/historical match between two players.
type Match struct {
	ID               int64      `json:"id"`
	Player1ID        int64      `json:"player1_id"`
	Player2ID        int64      `json:"player2_id"`
	VersionProfileID int64      `json:"version_profile_id"`
	Category         string     `json:"category"`
	SeedValue        int64      `json:"seed_value"`
	Status           string     `json:"status"`
	WinnerID         *int64     `json:"winner_id,omitempty"`
	P1EloBefore      *int       `json:"p1_elo_before,omitempty"`
	P2EloBefore      *int       `json:"p2_elo_before,omitempty"`
	P1EloAfter       *int       `json:"p1_elo_after,omitempty"`
	P2EloAfter       *int       `json:"p2_elo_after,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
}

// CreateMatch inserts a new match and atomically marks both players'
// queue entries as 'matched', so a duplicate /queue entry can't claim
// either player twice in the same tick.
//
// Returns the new match row.
func (s *Store) CreateMatch(ctx context.Context, p1, p2 int64, category string, seed int64) (*Match, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const insertQ = `
		INSERT INTO matches (player1_id, player2_id, category, seed_value, status)
		VALUES (?, ?, ?, ?, 'pending')
		RETURNING id, player1_id, player2_id, version_profile_id, category, seed_value,
		          status, winner_id, p1_elo_before, p2_elo_before, p1_elo_after, p2_elo_after,
		          created_at, started_at, ended_at
	`
	m, err := scanMatchRow(tx.QueryRowContext(ctx, insertQ, p1, p2, category, seed))
	if err != nil {
		return nil, fmt.Errorf("insert match: %w", err)
	}

	const updateQ = `UPDATE queue_entries SET status = 'matched'
	                 WHERE player_id IN (?, ?) AND status = 'waiting'`
	if _, err := tx.ExecContext(ctx, updateQ, p1, p2); err != nil {
		return nil, fmt.Errorf("mark queue matched: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return m, nil
}

// GetMatch returns the match by ID or ErrNotFound.
func (s *Store) GetMatch(ctx context.Context, id int64) (*Match, error) {
	const q = `
		SELECT id, player1_id, player2_id, version_profile_id, category, seed_value,
		       status, winner_id, p1_elo_before, p2_elo_before, p1_elo_after, p2_elo_after,
		       created_at, started_at, ended_at
		FROM matches WHERE id = ?
	`
	return s.scanMatch(ctx, q, id)
}

func (s *Store) scanMatch(ctx context.Context, q string, args ...any) (*Match, error) {
	m, err := scanMatchRow(s.DB.QueryRowContext(ctx, q, args...))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	return m, err
}

// scanMatchRow is shared between scanMatch (regular query) and CreateMatch
// (RETURNING inside a transaction). It maps all 15 match columns, scanning
// timestamps with scanTime / scanTimePtr so the code stays robust against
// modernc/sqlite returning string OR int64 for the same column.
func scanMatchRow(row *sql.Row) (*Match, error) {
	var (
		m        Match
		createdV any
		startedV any
		endedV   any
	)
	if err := row.Scan(
		&m.ID, &m.Player1ID, &m.Player2ID, &m.VersionProfileID,
		&m.Category, &m.SeedValue,
		&m.Status, &m.WinnerID,
		&m.P1EloBefore, &m.P2EloBefore, &m.P1EloAfter, &m.P2EloAfter,
		&createdV, &startedV, &endedV,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	createdAt, err := scanTime(createdV)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	m.CreatedAt = createdAt

	if err := scanTimePtr(startedV, &m.StartedAt); err != nil {
		return nil, fmt.Errorf("scan started_at: %w", err)
	}
	if err := scanTimePtr(endedV, &m.EndedAt); err != nil {
		return nil, fmt.Errorf("scan ended_at: %w", err)
	}
	return &m, nil
}
