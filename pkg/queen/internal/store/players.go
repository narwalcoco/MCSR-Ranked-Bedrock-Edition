package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a lookup expected a row but found none.
var ErrNotFound = errors.New("store: not found")

// Player represents a registry entry. Elo is denormalized at the table
// level for fast lookups; Phase 6 ratifies updates transactionally with
// match finalization.
type Player struct {
	ID          int64
	XUID        string
	Gamertag    string
	Elo         int
	Provisional bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UpsertPlayer inserts the player if xuid is new, otherwise updates only
// the gamertag (so a renamed gamer keeps their elo + match history).
// Returns the resulting row.
func (s *Store) UpsertPlayer(ctx context.Context, xuid, gamertag string) (*Player, error) {
	const q = `
		INSERT INTO players (xuid, gamertag) VALUES (?, ?)
		ON CONFLICT(xuid) DO UPDATE SET gamertag = excluded.gamertag,
		                              updated_at = unixepoch()
		RETURNING id, xuid, gamertag, elo, provisioned, created_at, updated_at
	`
	row := s.DB.QueryRowContext(ctx, q, xuid, gamertag)
	p, err := scanPlayerRow(row)
	if err != nil {
		return nil, fmt.Errorf("upsert player: %w", err)
	}
	return p, nil
}

// GetPlayerByXUID returns the player with the given XUID or ErrNotFound.
func (s *Store) GetPlayerByXUID(ctx context.Context, xuid string) (*Player, error) {
	const q = `SELECT id, xuid, gamertag, elo, provisioned, created_at, updated_at
	           FROM players WHERE xuid = ?`
	row := s.DB.QueryRowContext(ctx, q, xuid)
	p, err := scanPlayerRow(row)
	if err != nil {
		return nil, fmt.Errorf("get player by xuid: %w", err)
	}
	return p, nil
}

// GetPlayerByID returns the player with the given internal ID.
func (s *Store) GetPlayerByID(ctx context.Context, id int64) (*Player, error) {
	const q = `SELECT id, xuid, gamertag, elo, provisioned, created_at, updated_at
	           FROM players WHERE id = ?`
	row := s.DB.QueryRowContext(ctx, q, id)
	p, err := scanPlayerRow(row)
	if err != nil {
		return nil, fmt.Errorf("get player by id: %w", err)
	}
	return p, nil
}

// scanPlayerRow materializes one player row, scanning timestamps via scanTime
// (defensive — survives modernc/sqlite returning either int64 or string).
func scanPlayerRow(row *sql.Row) (*Player, error) {
	var (
		p        Player
		prov     int
		createdV any
		updatedV any
	)
	if err := row.Scan(&p.ID, &p.XUID, &p.Gamertag, &p.Elo, &prov, &createdV, &updatedV); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.Provisional = prov == 1
	if createdAt, err := scanTime(createdV); err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	} else {
		p.CreatedAt = createdAt
	}
	if updatedAt, err := scanTime(updatedV); err != nil {
		return nil, fmt.Errorf("scan updated_at: %w", err)
	} else {
		p.UpdatedAt = updatedAt
	}
	return &p, nil
}
