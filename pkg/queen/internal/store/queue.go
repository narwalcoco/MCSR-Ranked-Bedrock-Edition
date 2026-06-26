package store

import (
	"context"
	"fmt"
	"time"
)

// QueueEntryStatus values match the SQLite CHECK constraint on
// queue_entries.status. Keep these in sync with the migration file.
const (
	QueueStatusWaiting = "waiting"
	QueueStatusMatched = "matched"
	QueueStatusLeft    = "left"
)

// QueueEntry is one row in the queue.
type QueueEntry struct {
	ID       int64     `json:"id"`
	PlayerID int64     `json:"player_id"`
	XUID     string    `json:"xuid"`
	Gamertag string    `json:"gamertag"`
	Status   string    `json:"status"`
	QueuedAt time.Time `json:"queued_at"`
}

// JoinQueue inserts a queue entry for the given player. If the player
// already has a 'waiting' entry it is returned as-is (idempotent).
// If they have a 'matched' or 'left' entry it's re-queued (status=waiting,
// queued_at=now) so they can find a new match.
func (s *Store) JoinQueue(ctx context.Context, playerID int64) (*QueueEntry, error) {
	const q = `
		INSERT INTO queue_entries (player_id, status, queued_at)
		VALUES (?, 'waiting', unixepoch())
		ON CONFLICT(player_id) DO UPDATE SET
			status = 'waiting',
			queued_at = unixepoch()
		RETURNING id, player_id, status, queued_at
	`
	var e QueueEntry
	var tsV any
	err := s.DB.QueryRowContext(ctx, q, playerID).Scan(
		&e.ID, &e.PlayerID, &e.Status, &tsV,
	)
	if err != nil {
		return nil, fmt.Errorf("join queue: %w", err)
	}
	t, err := scanTime(tsV)
	if err != nil {
		return nil, fmt.Errorf("scan queued_at: %w", err)
	}
	e.QueuedAt = t

	// Fill in convenience fields the API exposes.
	if player, err := s.GetPlayerByID(ctx, playerID); err == nil {
		e.XUID = player.XUID
		e.Gamertag = player.Gamertag
	}
	return &e, nil
}

// LeaveQueueByPlayerID marks the player's queue entry 'left'. Idempotent.
func (s *Store) LeaveQueueByPlayerID(ctx context.Context, playerID int64) (bool, error) {
	const q = `UPDATE queue_entries SET status = 'left'
	           WHERE player_id = ? AND status = 'waiting'`
	res, err := s.DB.ExecContext(ctx, q, playerID)
	if err != nil {
		return false, fmt.Errorf("leave queue: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListWaiting returns up to limit waiting queue entries ordered by
// queued_at ASC (oldest first) and joins player fields for convenience.
func (s *Store) ListWaiting(ctx context.Context, limit int) ([]QueueEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT qe.id, qe.player_id, p.xuid, p.gamertag, qe.status, qe.queued_at
		FROM queue_entries qe
		JOIN players p ON p.id = qe.player_id
		WHERE qe.status = 'waiting'
		ORDER BY qe.queued_at ASC
		LIMIT ?
	`
	rows, err := s.DB.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list waiting: %w", err)
	}
	defer rows.Close()

	var out []QueueEntry
	for rows.Next() {
		var e QueueEntry
		var tsV any
		if err := rows.Scan(&e.ID, &e.PlayerID, &e.XUID, &e.Gamertag, &e.Status, &tsV); err != nil {
			return nil, fmt.Errorf("scan queue row: %w", err)
		}
		t, err := scanTime(tsV)
		if err != nil {
			return nil, fmt.Errorf("scan queued_at: %w", err)
		}
		e.QueuedAt = t
		out = append(out, e)
	}
	return out, rows.Err()
}

// FindLatestActiveMatch returns the most recent match (by created_at DESC)
// involving the given player_id that is in 'pending' or 'active' status.
// Returns ErrNotFound if no such match exists — used by the launcher to
// detect "you've been matched!" without a websocket.
func (s *Store) FindLatestActiveMatch(ctx context.Context, playerID int64) (*Match, error) {
	const q = `
		SELECT id, player1_id, player2_id, version_profile_id, category, seed_value,
		       status, winner_id, p1_elo_before, p2_elo_before, p1_elo_after, p2_elo_after,
		       created_at, started_at, ended_at
		FROM matches
		WHERE (player1_id = ? OR player2_id = ?)
		  AND status IN ('pending', 'active')
		ORDER BY created_at DESC
		LIMIT 1
	`
	return s.scanMatch(ctx, q, playerID, playerID)
}
