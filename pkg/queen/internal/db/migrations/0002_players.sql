-- 0002_players.sql
--
-- Phase 2: Players table.
--
-- xuid is the platform identity (Xbox XUID today, but stored as TEXT so
-- the schema is portable to other identity providers later).
--
-- Timestamps: INTEGER columns holding Unix epoch seconds. modernc.org/sqlite
-- scans these directly into Go's time.Time, no per-column parsing needed.
-- elo defaults to 1200 (FIDE-like standard) and `provisioned` is 1 by
-- default so brand-new players have a higher K-factor (Phase 6 reads both).

CREATE TABLE IF NOT EXISTS players (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    xuid          TEXT    NOT NULL UNIQUE,
    gamertag      TEXT    NOT NULL,
    elo           INTEGER NOT NULL DEFAULT 1200,
    provisioned   INTEGER NOT NULL DEFAULT 1 CHECK (provisioned IN (0, 1)),
    created_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at    INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_players_xuid ON players(xuid);
