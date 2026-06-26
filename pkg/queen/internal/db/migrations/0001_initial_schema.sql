-- 0001_initial_schema.sql
--
-- Phase 0: Track applied migrations. Domain tables (players, queue_entries,
-- matches, runner_slots, etc.) will be added in subsequent migrations
-- (Phase 2: accounts/queue; Phase 6: ratings; Phase 10: multi-worker).

CREATE TABLE IF NOT EXISTS schema_version (
    version      INTEGER PRIMARY KEY,
    description  TEXT    NOT NULL,
    applied_at   TEXT    NOT NULL DEFAULT (datetime('now'))
);
