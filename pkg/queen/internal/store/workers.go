package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// WorkerStatus values mirror the SQLite CHECK constraint on workers.status.
const (
	WorkerStatusOnline   = "online"
	WorkerStatusOffline  = "offline"
	WorkerStatusDraining = "draining"
)

// SlotStatus values mirror the SQLite CHECK constraint on runner_slots.status.
const (
	SlotStatusFree      = "free"
	SlotStatusAllocated = "allocated"
	SlotStatusBusy      = "busy"
)

// Worker is a registered BDS-hosting agent.
type Worker struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	AuthToken     string    `json:"auth_token"`
	MaxSlots      int       `json:"max_slots"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// RunnerSlot is a single BDS instance slot on a worker.
type RunnerSlot struct {
	ID        int64     `json:"id"`
	WorkerID  int64     `json:"worker_id"`
	SlotIndex int       `json:"slot_index"`
	Status    string    `json:"status"`
	MatchID   *int64    `json:"match_id,omitempty"`
	Port      int       `json:"port"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RegisterWorker creates a new worker record and maxSlots runner_slot rows
// (slot_index 0..maxSlots-1, all status='free'). The worker gets a
// random hex auth_token that the queen generates.
//
// Registration is idempotent by name+host: if a worker with the same
// (name, host) already exists, its auth_token and last_heartbeat are
// refreshed. New slots are NOT created on re-registration (existing
// slots are preserved).
func (s *Store) RegisterWorker(ctx context.Context, name, host string, port, maxSlots int) (*Worker, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Check for existing worker by (name, host).
	// NOTE: max_slots changes on re-registration are NOT reconciled here.
	// If a worker changes its --max-slots and re-registers, the stored
	// max_slots and slot count reflect the ORIGINAL registration. Phase 4+
	// should reconcile or reject on mismatch (track with KNOWN_ISSUES.md).
	existing, err := s.scanWorkerRow(tx.QueryRowContext(ctx,
		`SELECT id, name, host, port, auth_token, max_slots, status,
		        last_heartbeat, created_at, updated_at
		 FROM workers WHERE name = ? AND host = ? AND status != 'offline'`,
		name, host,
	))
	if err == nil {
		// Idempotent: refresh heartbeat + token, return existing.
		token := newAuthToken()
		_, err = tx.ExecContext(ctx,
			`UPDATE workers SET auth_token = ?, last_heartbeat = unixepoch(),
			      updated_at = unixepoch(), status = 'online', port = ?
			 WHERE id = ?`,
			token, port, existing.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("refresh worker: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		existing.AuthToken = token
		existing.Port = port
		existing.Status = WorkerStatusOnline
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("lookup existing: %w", err)
	}

	// Fresh registration.
	token := newAuthToken()
	const insertW = `
		INSERT INTO workers (name, host, port, auth_token, max_slots)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id, name, host, port, auth_token, max_slots, status,
		          last_heartbeat, created_at, updated_at
	`
	w, err := s.scanWorkerRow(tx.QueryRowContext(ctx, insertW, name, host, port, token, maxSlots))
	if err != nil {
		return nil, fmt.Errorf("insert worker: %w", err)
	}

	// Create slot rows.
	for i := 0; i < maxSlots; i++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO runner_slots (worker_id, slot_index) VALUES (?, ?)`,
			w.ID, i,
		); err != nil {
			return nil, fmt.Errorf("insert slot %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return w, nil
}

// HeartbeatWorker updates last_heartbeat and status='online' for a worker.
func (s *Store) HeartbeatWorker(ctx context.Context, workerID int64) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE workers SET last_heartbeat = unixepoch(), updated_at = unixepoch(),
		      status = CASE WHEN status = 'offline' THEN 'online' ELSE status END
		 WHERE id = ?`, workerID,
	)
	if err != nil {
		return fmt.Errorf("heartbeat worker %d: %w", workerID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListWorkers returns all workers ordered by name.
func (s *Store) ListWorkers(ctx context.Context) ([]Worker, error) {
	const q = `
		SELECT id, name, host, port, auth_token, max_slots, status,
		       last_heartbeat, created_at, updated_at
		FROM workers ORDER BY name
	`
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()

	var out []Worker
	for rows.Next() {
		w, err := s.scanWorker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// GetWorker returns a worker by ID or ErrNotFound.
func (s *Store) GetWorker(ctx context.Context, id int64) (*Worker, error) {
	const q = `
		SELECT id, name, host, port, auth_token, max_slots, status,
		       last_heartbeat, created_at, updated_at
		FROM workers WHERE id = ?
	`
	return s.scanWorkerRow(s.DB.QueryRowContext(ctx, q, id))
}

// ListFreeSlots returns all runner_slots with status='free' whose parent
// worker is 'online', ordered by worker name then slot_index.
func (s *Store) ListFreeSlots(ctx context.Context) ([]RunnerSlot, error) {
	const q = `
		SELECT rs.id, rs.worker_id, rs.slot_index, rs.status, rs.match_id,
		       rs.port, rs.created_at, rs.updated_at
		FROM runner_slots rs
		JOIN workers w ON w.id = rs.worker_id
		WHERE rs.status = 'free' AND w.status = 'online'
		ORDER BY w.name, rs.slot_index
	`
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list free slots: %w", err)
	}
	defer rows.Close()

	var out []RunnerSlot
	for rows.Next() {
		slot, err := scanSlotRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *slot)
	}
	return out, rows.Err()
}

// ListWorkerSlots returns all slots for a given worker.
func (s *Store) ListWorkerSlots(ctx context.Context, workerID int64) ([]RunnerSlot, error) {
	const q = `
		SELECT id, worker_id, slot_index, status, match_id,
		       port, created_at, updated_at
		FROM runner_slots WHERE worker_id = ? ORDER BY slot_index
	`
	rows, err := s.DB.QueryContext(ctx, q, workerID)
	if err != nil {
		return nil, fmt.Errorf("list worker slots %d: %w", workerID, err)
	}
	defer rows.Close()

	var out []RunnerSlot
	for rows.Next() {
		slot, err := scanSlotRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *slot)
	}
	return out, rows.Err()
}

// MarkWorkerOffline sets a worker to offline status. Used when the queen
// detects a missed heartbeat window.
func (s *Store) MarkWorkerOffline(ctx context.Context, workerID int64) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE workers SET status = 'offline', updated_at = unixepoch()
		 WHERE id = ?`, workerID,
	)
	if err != nil {
		return fmt.Errorf("mark offline %d: %w", workerID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DrainWorker marks a worker as draining (no new matches, existing matches
// finish normally). Phase 4+ will also free any still-free slots.
func (s *Store) DrainWorker(ctx context.Context, workerID int64) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE workers SET status = 'draining', updated_at = unixepoch()
		 WHERE id = ?`, workerID,
	)
	if err != nil {
		return fmt.Errorf("drain worker %d: %w", workerID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --------------------------------------------------------------------------
// Scanning helpers (match existing store/scan_time.go patterns)
// --------------------------------------------------------------------------

func (s *Store) scanWorkerRow(row *sql.Row) (*Worker, error) {
	var (
		w          Worker
		heartbeatV any
		createdV   any
		updatedV   any
	)
	if err := row.Scan(
		&w.ID, &w.Name, &w.Host, &w.Port, &w.AuthToken, &w.MaxSlots, &w.Status,
		&heartbeatV, &createdV, &updatedV,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	hb, err := scanTime(heartbeatV)
	if err != nil {
		return nil, fmt.Errorf("scan last_heartbeat: %w", err)
	}
	w.LastHeartbeat = hb
	ca, err := scanTime(createdV)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	w.CreatedAt = ca
	ua, err := scanTime(updatedV)
	if err != nil {
		return nil, fmt.Errorf("scan updated_at: %w", err)
	}
	w.UpdatedAt = ua
	return &w, nil
}

func (s *Store) scanWorker(rows *sql.Rows) (*Worker, error) {
	var (
		w          Worker
		heartbeatV any
		createdV   any
		updatedV   any
	)
	if err := rows.Scan(
		&w.ID, &w.Name, &w.Host, &w.Port, &w.AuthToken, &w.MaxSlots, &w.Status,
		&heartbeatV, &createdV, &updatedV,
	); err != nil {
		return nil, err
	}
	hb, err := scanTime(heartbeatV)
	if err != nil {
		return nil, fmt.Errorf("scan last_heartbeat: %w", err)
	}
	w.LastHeartbeat = hb
	ca, err := scanTime(createdV)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	w.CreatedAt = ca
	ua, err := scanTime(updatedV)
	if err != nil {
		return nil, fmt.Errorf("scan updated_at: %w", err)
	}
	w.UpdatedAt = ua
	return &w, nil
}

// scanSlotRow scans a runner_slots row from *sql.Rows (used in multi-row queries).
// When a single-row slot query is added (Phase 4+), add a companion
// scanSlotRowFromRow(*sql.Row) following the scanWorker/scanWorkerRow pattern.
func scanSlotRow(row *sql.Rows) (*RunnerSlot, error) {
	var (
		s        RunnerSlot
		createdV any
		updatedV any
	)
	if err := row.Scan(
		&s.ID, &s.WorkerID, &s.SlotIndex, &s.Status, &s.MatchID,
		&s.Port, &createdV, &updatedV,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ca, err := scanTime(createdV)
	if err != nil {
		return nil, fmt.Errorf("scan slot created_at: %w", err)
	}
	s.CreatedAt = ca
	ua, err := scanTime(updatedV)
	if err != nil {
		return nil, fmt.Errorf("scan slot updated_at: %w", err)
	}
	s.UpdatedAt = ua
	return &s, nil
}

// newAuthToken generates a random 32-byte hex token for worker auth.
func newAuthToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read only fails on catastrophic system entropy
		// exhaustion; fall back to a distinguishable marker so the
		// problem is visible rather than silent.
		return "RAND_FAIL_" + hex.EncodeToString(make([]byte, 32))
	}
	return hex.EncodeToString(b)
}

// ErrNotFound is re-exported for callers (it is defined in players.go).
// This file reuses the same sentinel so callers can check with errors.Is.
// No duplicate definition — this comment is documentation-only.
