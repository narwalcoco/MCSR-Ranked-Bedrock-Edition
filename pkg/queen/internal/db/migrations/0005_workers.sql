-- 0005: Workers + runner slots for Phase 3.
--
-- Workers are the BDS-hosting agents.  Each worker advertises a fixed
-- number of runner slots; the queen assigns matches into free slots.
-- Timestamps are INTEGER Unix epoch seconds (unixepoch()) per the
-- convention established in 0002–0004.

CREATE TABLE workers (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT    NOT NULL,
    host            TEXT    NOT NULL,
    port            INTEGER NOT NULL DEFAULT 0,
    auth_token      TEXT    NOT NULL UNIQUE,
    max_slots       INTEGER NOT NULL DEFAULT 4,
    status          TEXT    NOT NULL DEFAULT 'online'
                    CHECK (status IN ('online', 'offline', 'draining')),
    last_heartbeat  INTEGER NOT NULL DEFAULT (unixepoch()),
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE runner_slots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id       INTEGER NOT NULL REFERENCES workers(id),
    slot_index      INTEGER NOT NULL,
    status          TEXT    NOT NULL DEFAULT 'free'
                    CHECK (status IN ('free', 'allocated', 'busy')),
    match_id        INTEGER REFERENCES matches(id),
    port            INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE(worker_id, slot_index)
);
