-- 0004_matches.sql
--
-- Phase 2: Matches table.
--
-- Status state machine:
--   pending    — pair created, awaiting worker to claim (Phase 3)
--   active     — BDS is running with both players connected (Phase 4/5)
--   completed  — winner decided, ratings updated (Phase 6 close-out)
--   cancelled  — match was torn down before it started
--   forfeited  — one player left mid-match (Phase 5 closes this)
--
-- Timestamps: INTEGER columns with unixepoch() defaults (or NULL when not
-- yet set). modernc.org/sqlite scans these directly into Go's time.Time.
-- Elo columns (p1/p2_elow_before/_after) are written by Phase 6;
-- nullable so Phase 2 matches can stay pending without denormalized
-- rating state.

CREATE TABLE IF NOT EXISTS matches (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    player1_id        INTEGER NOT NULL,
    player2_id        INTEGER NOT NULL,
    version_profile_id INTEGER NOT NULL DEFAULT 1,
    category          TEXT    NOT NULL,
    seed_value        INTEGER NOT NULL,
    status            TEXT    NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'active', 'completed', 'cancelled', 'forfeited')),
    winner_id         INTEGER,
    p1_elo_before     INTEGER,
    p2_elo_before     INTEGER,
    p1_elo_after      INTEGER,
    p2_elo_after      INTEGER,
    created_at        INTEGER NOT NULL DEFAULT (unixepoch()),
    started_at        INTEGER,
    ended_at          INTEGER,
    FOREIGN KEY (player1_id) REFERENCES players(id),
    FOREIGN KEY (player2_id) REFERENCES players(id),
    FOREIGN KEY (winner_id)  REFERENCES players(id),
    CHECK (player1_id != player2_id)
);

CREATE INDEX IF NOT EXISTS idx_matches_player1 ON matches(player1_id);
CREATE INDEX IF NOT EXISTS idx_matches_player2 ON matches(player2_id);
CREATE INDEX IF NOT EXISTS idx_matches_status  ON matches(status);
CREATE INDEX IF NOT EXISTS idx_matches_p1_status ON matches(player1_id, status);
CREATE INDEX IF NOT EXISTS idx_matches_p2_status ON matches(player2_id, status);
