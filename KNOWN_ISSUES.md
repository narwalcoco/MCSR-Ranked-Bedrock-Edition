# Known Issues / Problem Log

> A living document of **non-trivial bugs, gotchas, and edge cases** we
> have hit (and resolved) while building MCSR Ranked Bedrock. When a
> bug surfaces again, future humans and AI agents should be able to
> **find it here in seconds** instead of rediscovering it from scratch.

---

## How to use this file

When you hit something weird:

1. **Check this file first.** Your "new" bug might already be solved
   here — many look unrelated but share a root cause.
2. **Reproduce minimally.** Make the *smallest* test or `curl`
   command that shows the problem.
3. **Add an entry** under **Open / Unresolved** using the template
   below.
4. **Once fixed**, move the entry down to **Resolved (watch out for
   regressions)** with a link to the commit / file that fixed it.

The point is **faster debugging next time**, not a perfect bug
database. A rough note in here beats a perfect bug in a JIRA no one
reads.

---

## Entry template

Copy this when logging a new issue:

```markdown
### YYYY-MM-DD: <short title>

- **Phase:** <number + title>
- **Component(s):** <paths like pkg/queen/internal/store/foo.go>
- **Trigger:** <curl / command / click sequence>
- **Symptom:** <exact error text or wrong behavior>
- **Root cause:** <one or two sentences — *why*, not just *what*>
- **Workaround / Fix:** <what we did>
- **Why we kept the workaround:** <only if workaround, not real fix>
- **Watch out:** <signs it's happening again>
```

---

## Open / Unresolved

### 2025-06-26 — Phase 3: Worker re-registration ignores changed `max_slots`

- **Phase:** 3 — Local Worker Agent
- **Component(s):** `pkg/queen/internal/store/workers.go` → `RegisterWorker`
- **Trigger:** Worker A registers with `max_slots=4` (4 `runner_slots` rows created). Worker A restarts with `--max-slots=8` and re-registers (same name+host).
- **Symptom:** The queen refreshes the worker's `auth_token` and `last_heartbeat` but does NOT add the 4 missing slot rows. The worker advertises 8 slots, the queen only has 4. Mismatch.
- **Root cause:** Re-registration is idempotent by `(name, host)` — it refreshes the row but doesn't reconcile `max_slots`. Slot creation only happens on the **first** registration.
- **Workaround / Fix:** For Phase 3 dev testing, use a unique worker name when changing `--max-slots`. Phase 4+ should either (a) reconcile slots on re-registration (add/remove rows to match), or (b) reject re-registration with 409 if `max_slots` differs.
- **Watch out:** If a worker shows fewer slots in `GET /workers/{id}/slots` than its `max_slots` field claims, this is the cause.
- **Why unresolved:** Low priority for Phase 3 (single-worker dev). Blocking for Phase 4 multi-worker deployments.

---

## Resolved (watch out for regressions)

### 2025 — Phase 2: `modernc.org/sqlite` sometimes returns INTEGER unix-epoch columns as **strings**

- **Phase:** 2 — Accounts, Queue, Matchmaking
- **Component(s):**
  - New helper: `pkg/queen/internal/store/scan_time.go`
  - All repository methods that read `created_at`, `updated_at`,
    `queued_at`, `started_at`, `ended_at`
    (`pkg/queen/internal/store/{players,queue,matches}.go`)
- **Trigger:** Define a SQLite column as `INTEGER DEFAULT (unixepoch())`
  (or `INTEGER NOT NULL DEFAULT (strftime('%s','now'))`), insert a row,
  read it back with `database/sql` `Scan(...)` into a `time.Time`.
- **Symptom:** One of:
  - `sql: Scan error on column index N, name "created_at":
     unsupported Scan, storing driver.Value type string into
     type *time.Time`
  - Or, after softening the scan to accept `any`: `parsing time
     "1782258817" as "2006-01-02 15:04:05": cannot parse "2258817"
     as "15"`.
- **Root cause:** `modernc.org/sqlite` is the **pure-Go SQLite
  driver** we use (no CGo). Its row-decoding behavior is
  **inconsistent across versions and column types** — for some
  `INTEGER` columns it returns `int64`; for others it returns a
  decimal `string` like `"1782258817"`. Go's `database/sql.Scan`
  rules then refuse to coerce `string` → `time.Time`, and a naive
  parser that assumes "this is a datetime string" explodes on what
  is really a number.
- **Workaround / Fix:** New centralized helper
  `scanTime(any) (time.Time, error)` (and `scanTimePtr` for nullable
  columns). It handles `nil`, `int64`, `int`, and:
  - For `string`: first tries `parseUnixSeconds` (must be **all
    digits**, else rejected), then falls back to parsing SQLite
    formatted datetime `"2006-01-02 15:04:05"`.
  - Defaults: returns an explicit error so we notice if the driver
    starts returning something new.
  Every timestamp column is now routed through `scanTime` /
  `scanTimePtr`. The schema was also migrated from
  `TEXT DEFAULT (datetime('now'))` to
  `INTEGER DEFAULT (unixepoch())` for type-friendliness, but the
  helper defends against either representation.
- **Why we kept a workaround (didn't fix the driver):**
  - Fixing upstream would mean either forking `modernc.org/sqlite`
    or swapping to a CGo driver (which we specifically avoided to
    keep portable single-file builds).
  - The helper is ~30 lines, centralized in one file, and trivially
    unit-testable.
  - Driver behavior could change with any minor version bump, and
    the helper makes that future change visible instead of silent.
- **Watch out (signs it's happening again):**
  - Any new `INTEGER DEFAULT (unixepoch())` column added in a new
    migration: **must** route its `Scan` through `scanTime`. Symlink
    the helper into your new repository method.
  - Any new `time.Time` field in a `Row.Scan(...)` call: grep it,
    make sure it uses the helper.
  - After any bump in `modernc.org/sqlite` version in `go.mod`,
    re-run the queen end-to-end and watch for `Scan error` in logs.
  - If `parseUnixSeconds` ever sees a string like `"2024-01-02T..."`,
    it correctly rejects it (non-digit chars) — don't relax the
    digit-only check without tests.

---

### Other resolved items

*None yet — add future bugs here as we encounter them.*
