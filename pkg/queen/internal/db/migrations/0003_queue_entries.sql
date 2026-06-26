-- 0003_queue_entries.sql
--
-- Phase 2: Queue entries table.
--
-- One row per queued player (UNIQUE on player_id). Status is a tiny state
-- machine: 'waiting' (in queue), 'matched' (paired into a match), 'left'
-- (user cancelled). We update status rather than DELETE so there's an
-- audit trail.
--
-- Timestamps: INTEGER column with unixepoch() so the Go side can Scan
-- directly into time.Time.

CREATE TABLE IF NOT EXISTS queue_entries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id  INTEGER NOT NULL UNIQUE,
    status     TEXT    NOT NULL DEFAULT 'waiting'
               CHECK (status IN ('waiting', 'matched', 'left')),
    queued_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_queue_status_queued
    ON queue_entries(status, queued_at);
