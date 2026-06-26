# MCSR Ranked Bedrock — Architecture & Plan

## 🎯 Vision

Replicate the MCSR Ranked (Java Edition) experience for Minecraft Bedrock Edition. Players queue up, get matched against opponents of similar skill, race through the same seed, and earn ratings — all from a beautiful desktop app.

---

## 🏗️ Architecture Overview (Revised)

### Three-Component System

Instead of a fragile DLL injection approach (Windows-only, requires admin, breaks every MCBE update), we use **three components working together**:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       USER'S COMPUTER (Windows)                         │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  1. TAURI LAUNCHER (the desktop app the user downloads)          │   │
│  │                                                                  │   │
│  │  Engine: Rust 🦀  │  UI: HTML + CSS + JavaScript                 │   │
│  │                                                                  │   │
│  │  • Queue page, stats, match history                              │   │
│  │  • Match found screen with seed category art                     │   │
│  │  • Loading overlay that appears on top of MCBE while it loads    │   │
│  │  • Auto-launches MCBE and connects to the match server           │   │
│  │  • In-match overlay (opponent progress, timer)                   │   │
│  │  • Xbox Live sign-in (Microsoft OAuth)                           │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  2. GO BACKEND (runs as a background service)                    │   │
│  │                                                                  │   │
│  │  • The "Queen" — matchmaking, ratings, seed selection, lifecycle  │   │
│  │  • The "Worker" — manages Bedrock Dedicated Server instances     │   │
│  │  • SQLite database for dev, PostgreSQL-ready schema              │   │
│  │  • HTTP + WebSocket API for launcher communication               │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │  3. MCBE ADD-ON (JavaScript — runs on each BDS instance)         │   │
│  │                                                                  │   │
│  │  • Detects speedrun progress via Script API events               │   │
│  │  • Reports splits/completion to the backend                     │   │
│  │  • Displays opponent progress via boss bars / chat messages      │   │
│  │  • No injection needed — officially supported by Mojang          │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### Why This Architecture (vs. DLL Injection)

| Concern | DLL Injection | Our Approach (Tauri + Add-on) |
|---------|---------------|-------------------------------|
| **Windows only?** | Yes | ✅ Windows first, Mac later |
| **Admin required?** | Usually yes | ✅ No — runs at user level |
| **Breaks on MCBE update?** | Almost always | ✅ URI and add-on are stable |
| **User download size** | Small DLL | ~10MB Tauri app |
| **User experience** | Full in-game overlay | ✅ Beautiful launcher + overlay |
| **Development complexity** | Expert C++/reverse engineering | ✅ Moderate (JS + Rust + Go) |
| **Mobile support?** | No | ✅ Server-side add-on works for all |
| **Maintenance burden** | Very high | ✅ Low — uses stable APIs |

---

## 🧩 Component Breakdown

### 1. Tauri Launcher — The Desktop App

**What it's built with:**
- **Rust** (engine) — window management, system calls, launching MCBE, overlay, websocket client, Xbox auth
- **HTML + CSS + JavaScript** (UI) — all the visual screens and animations, using a framework like React or Svelte

**Screens & Features:**

| Screen | Description |
|--------|-------------|
| **Home / Auth** | Xbox Live sign-in, welcome, quick stats |
| **Queue** | Join/leave queue, estimated wait time, current Elo |
| **Match Found** | Full-screen seed category art + opponent info + countdown |
| **Loading Overlay** | Transparent, always-on-top window over MCBE during load |
| **In-Match HUD** | Small overlay showing opponent progress, timer |
| **Stats & History** | Match history, Elo graph, W/L record |
| **Settings** | MCBE path, overlay hotkey, auto-launch toggle |

**Auto-launch to server:**
```rust
// When match is found, launch MCBE and auto-connect
std::process::Command::new("cmd")
    .args(&["/c", "start", "minecraft://connect/?serverUrl=123.45.67.89&serverPort=25566"])
    .spawn()
    .unwrap();
```

**Loading overlay behavior:**
- Window is `alwaysOnTop: true`, `transparent: true`, `decorations: false`
- Sits on top of MCBE while it loads
- Shows full-screen seed category artwork (Shipwreck, Desert Temple, etc.)
- Shows opponent name, category name, loading animation
- Auto-dismisses after a timer (or when player presses a key)
- Can optionally shrink to a small corner HUD for in-match opponent progress

---

### 2. Go Backend — The Queen + Worker

**What it's built with:**
- **Go** — compiles to a single `mcsr-ranked.exe`
- **SQLite** — database for dev; schema designed for PostgreSQL migration
- **HTTP + WebSocket** — API for launcher, add-on events

**Queen Service:**
- Player accounts (Xbox identity)
- Queue management
- Match creation (pairs two players)
- Seed selection (category → seed)
- Rating system (Elo or similar)
- Match lifecycle (created → active → completed/cancelled)

**Worker Agent:**
- Registers with Queen, advertises available slots
- Manages BDS binary and pack cache
- Creates isolated BDS instances for each match
- Enforces timeouts and cleanup
- Reports player join/leave/splits/completion

---

### 3. MCBE Add-on — Progress Detection

**What it's built with:**
- **JavaScript** (Minecraft Script API)
- Runs on each Bedrock Dedicated Server instance
- No client-side injection needed

**What it detects:**
- Player joins/leaves the server
- Player enters Nether (dimension change event)
- Player obtains blaze rods, ender pearls (item acquisition events)
- Player enters End (dimension change)
- Player completes the run

**How it communicates:**
- Sends HTTP requests to the Go backend with progress events
- Displays opponent progress via boss bars / chat messages

---

## 🚀 How a Match Works End-to-End

```
1. User opens Tauri Launcher
2. Signs in with Xbox Live
3. Clicks "Queue Up"
4. Launcher sends queue request to Go backend via HTTP
   ─────────────────────────────────────────────────────
5. Backend matches them with another player
6. Backend selects: Version Profile → Seed Category → Seed
7. Backend reserves two BDS slots
8. Worker starts two BDS instances with the selected seed
9. Worker reports "servers ready" to Queen
   ─────────────────────────────────────────────────────
10. Launcher polls backend → "Match Found!"
11. Full-screen Match Found screen appears:
    ┌─────────────────────────────────────────────┐
    │  ⚔ Opponent: SpeedDemon45 (Elo 1180)        │
    │  🏝 Category: SHIPWRECK                      │
    │  🔗 Connecting to mc.mcsrranked.gg:25566...  │
    │                                              │
    │  [▶ Join Match]  [10... 9... 8...]          │
    └─────────────────────────────────────────────┘
12. User clicks Join (or auto-joins after countdown)
13. Launcher executes minecraft://connect URI
14. MCBE opens and auto-connects to the match server
15. Tauri overlay window appears ON TOP of MCBE:
    ┌─────────────────────────────────────────────┐
    │                                             │
    │           🏝 SHIPWRECK                      │
    │                                             │
    │  You vs SpeedDemon45                        │
    │  ████████░░░░░░░░ Loaded 40%                │
    │                                             │
    └─────────────────────────────────────────────┘
16. Once loaded, overlay fades away
17. Player speedruns the seed
18. Add-on detects progress, reports to backend
19. Launcher shows opponent progress (if enabled)
20. Run completes — ratings are updated
21. Player returns to launcher, can queue again
```

---

## 🎨 Seed Category Loading Screens

Each seed category gets a unique full-screen background design in the launcher:

| Category | Visual Theme |
|----------|-------------|
| Ruined Portal | Purple/orange Nether portal glow, obsidian frame |
| Stronghold | Stone brick, silverfish, end portal frame |
| Village | Warm plains village, hay bales, paths |
| Desert Temple | Sandy orange, terracotta, TNT trap |
| Shipwreck | Ocean blue, wooden hull, treasure maps |
| Bastion | Red/black Nether brick, piglin theme |
| Warped | Blue/teal warped forest aesthetic |

Each screen shows:
- Large category title with styled text
- Opponent name + Elo
- Match countdown (10 seconds)
- Animated decorative elements (particles, floating items, etc.)

---

## 📅 Revised Phase Plan

### ✅ Phase 0: Project Skeleton
- Go module and service layout
- Config loading + structured logging
- SQLite dev database
- Queen health endpoint
- Resource directory conventions

### ✅ Phase 1: Seed Category Pipeline
- Extract seeds from Fsg-Generator archive
- Map source files to categories
- Deduplicate, normalize as strings
- Output canonical JSON + manifest
- Random selection helper

### ✅ Phase 2: Accounts, Queue, Matchmaking
- Player table with Xbox XUID
- Dev fake-account mode
- Queue entries
- Match creation (2 players, seed selection)

### ✅ Phase 3: Local Worker Agent
- Worker registers with Queen
- Advertises slots and profiles
- Slot lifecycle states
- Fake worker mode for testing

### ✅ Phase 4: Real BDS Startup
- BDS binary + pack caches
- World templates
- Per-runner working folders with unique ports
- `debug.mcfunction` generation
- Staggered startup + cleanup

### ✅ Phase 5: Result Events
- Capture BDS logs + add-on event output
- Player join/leave, splits, completion
- Winner/loss/cancelled/forfeit logic
- Duplicate result rejection

### ✅ Phase 6: Ratings
- Rating model (Elo or similar)
- Provisional player handling
- Win/loss/draw/forfeit updates
- Transactional with match finalization

### 🆕 Phase 7: Tauri Launcher — Core
- **Tauri project setup** with React/Svelte frontend
- Home screen with Xbox sign-in (Microsoft OAuth)
- Queue screen (join/leave, wait time, Elo display)
- Poll backend for match status
- Match Found screen with seed category art backgrounds
- Auto-launch MCBE via `minecraft://connect` URI
- Settings screen (MCBE path, auto-launch toggle)

### 🆕 Phase 8: Tauri Overlay & Polish
- Transparent, always-on-top overlay window
- Loading screen with seed art + opponent info + countdown
- Auto-dismiss overlay after loading
- In-match corner HUD (opponent progress, timer)
- Hotkey to toggle overlay
- Window animations (fade in/out, transitions)
- System tray integration (minimize to tray)

### 🆕 Phase 9: MCBE Add-on
- JavaScript Script API add-on for BDS
- Track player join/leave
- Detect dimension changes (Overworld → Nether → End)
- Detect item acquisitions (blaze rods, ender pearls)
- Report progress via HTTP to backend
- Display opponent progress via boss bars or chat
- Handle disconnects and timeouts
- **In-match chat commands** — see sub-section below.

### 💬 In-Match Chat Commands *(sub-feature: chat input lives in Phase 9; state machine in Phase 5; ratings in Phase 6)*

Player-issued commands typed in MCBE chat during a live match. Chat input is detected by the MCBE add-on; the persistent state machine is owned by the queen (Phase 5); Elo updates come from Phase 6. All commands use a `/` prefix so they don't clash with normal chat.

| Command | Agreement | Expiry | Cap | Result on agreement |
|---------|-----------|--------|-----|---------------------|
| **`/forfeit`** | None — single-player | n/a — instant | None | Match ends as a Loss for the typist / Win for the opponent (Phase 6 ratings). Any pending request is cancelled. |
| **`/seedchange`** | Other player must also type `/seedchange` | 30 seconds from the first request | Max 3 *agreed* changes per match (`matches.seed_change_count`) | Both players kicked; fresh seed picked from the same category; world regenerated; in-game timer + splits reset to 0:00. |
| **`/draw`** | Other player must also type `/draw` | 30 seconds from the first request | None — a match finalizes via draw at most once | Match ends immediately as `completed_draw`; both Elo updated per Phase 6 draw formula. |
| **`/report`** | None — single-player | n/a — instant | One per player per match | Sends a report to queen moderators. The report contains: match ID, reporting player, reported player, a freeform reason string, and an optional `evidence` blob (Phase 7+ could attach screenshots/video). No gameplay effect — does not pause, forfeit, or alter the match. |

**State machine**

```
            (any state) ─add /forfeit──▶ [Resolved: forfeit]
                  │
[Idle] ─add /seedchange──▶ [Pending] ─30s elapses──▶ [Expired]
   │                      │           │
   │                      │           └─ other agrees within 30s ─▶ [Agreed]
   │                      │                                  │
   └─ add /draw──────────┘                                  ├─ /seedchange → regen world, reset timer
                                                            └─ /draw      → finalize as draw

        * Only ONE active request per match at a time.
```

**New / extended data (Phase 5)**

- **New table** `match_state_requests` (`id`, `match_id`, `request_type` ∈ {`seedchange`, `draw`}, `requested_by` (player_id), `requested_at`, `expires_at`, `status` ∈ {`pending`, `agreed`, `expired`, `cancelled`})
- **Extend `matches`:** add `seed_change_count INTEGER NOT NULL DEFAULT 0`
- **New table** `reports` (`id`, `match_id` REFERENCES matches(id), `reported_by` (player_id), `reported_player` (player_id), `reason` TEXT NOT NULL, `evidence` TEXT, `created_at` INTEGER NOT NULL DEFAULT (unixepoch())). One `UNIQUE(match_id, reported_by)` constraint so each player can report each match at most once.
- **Extend `match_events`:** add event types `seed_change_requested`, `seed_change_agreed`, `seed_change_expired`, `seed_change_completed`, `draw_requested`, `draw_agreed`, `forfeit`, `report_submitted`. The `seed_change_completed` event carries `{ old_seed, new_seed, agreed_by }`; the `report_submitted` event carries `{ reported_by, reported_player, reason }`.

**Constraints / edge cases**

- Only one *active* `match_state_requests` row per match at a time. While one row is `pending`, a different command of a different type is rejected with 409 `request_in_progress`. The same player re-typing the same command is **idempotent** (no duplicate rows). `/report` is exempt from the one-active-request rule — it can be issued at any time, including while a `/seedchange` or `/draw` is pending.
- A 4th `/seedchange` request is rejected with 409 `seed_change_cap_reached` once `seed_change_count == 3`. `/forfeit` and `/draw` are uncapped.
- Pending requests auto-cancel on `/forfeit` (status → `cancelled`). No further resolution steps after that.
- Pending requests expire naturally via `expires_at`. Expiry checks run on a Phase 5 background tick (or piggybacked onto the existing matchmaker loop).
- Disconnects do **not** auto-cancel a pending request — it expires via the 30s window. (Easy to revisit.)

**Mid-match `/seedchange` flow (touches Phase 4)**

1. Both players agree → queen increments `seed_change_count`, picks fresh seed (same category).
2. Queen emits `seed_change_completed` event with `{ old_seed, new_seed, agreed_by }`.
3. Worker (Phase 3 agent) instructs the local BDS (Phase 4) to kick both runners, dispose the instance, and lay a new world with the new seed; players reconnect.
4. Active in-game timer + splits reset to 0:00. Prior splits stay in `match_events` for audit; a new "run start" anchor is recorded.
5. Recommend: in-game timer displays the *current-seed* start (not the match-anchor start) — split data is meaningful per seed. **Open decision — confirm during build.**

**Client surfaces**

- **MCBE add-on (Phase 9):** `tellraw` on request creation + on expiry/cancellation; action-bar / boss-bar countdown of remaining seconds.
- **Tauri Overlay (Phase 8):** visual countdown during the 30s window; auto-dismiss when resolved.
- **Tauri Launcher (Phase 7):** surfaces the outcome — forfeit banner, "rejoining with new seed…" for `/seedchange`, draw banner for `/draw`.

**Penalty for `/forfeit`** — standard loss per Phase 6 (no extra penalty beyond the Elo hit). Easy to revisit.
- Worker auth tokens
- Remote worker registration
- Health checks and dead worker handling
- Capacity-aware matchmaking

> **Note:** Phases marked ✅ are unchanged from the original plan. Phases marked 🆕 are new to this revised architecture.

---

## 🔧 Tech Stack Summary

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Backend** | Go | Matchmaking, ratings, worker management, API |
| **Database** | SQLite → PostgreSQL | Player data, matches, ratings |
| **Desktop App** | Tauri (Rust + HTML/CSS/JS) | Launcher, queue UI, overlay, auto-connect |
| **UI Framework** | React or Svelte | Visual interface inside Tauri |
| **Add-on** | JavaScript (Script API) | Progress detection on BDS |
| **Server** | Bedrock Dedicated Server | Minecraft world hosting for each match |
| **Communication** | HTTP + WebSocket | Launcher ↔ Backend, Add-on ↔ Backend |

---

## 📁 Resource Layout

```
resources/
  source/
    Ranked1.3.mcaddon
    Fsg-Generator-MCBE-main.zip
  seeds/
    raw/
    categories/
    manifest.json
  packs/
    source/
    generated/
  templates/
    bedrock_1_18_plus/
  bds/
launcher/           ← Tauri project lives here
  src-tauri/        ← Rust code
  src/              ← HTML/CSS/JS UI
  public/
    backgrounds/    ← Seed category art assets
pkg/
  queen/            ← Go queen service
  worker/           ← Go worker agent
addon/              ← MCBE Script API add-on
  behavior_pack/
  resource_pack/
```

---

## 🪟 Windows-First Development

- **Phase 7 & 8** target Windows only initially
- `minecraft://connect` URIs are Windows-specific
- Always-on-top overlay uses Windows-specific Tauri APIs
- Xbox Live OAuth works on Windows natively
- **Mac version** will be added later (requires different deep-link approach and Mac-specific overlay APIs)

---

## 📊 Database Schema (Core Tables)

```
players
  id, xuid, gamertag, elo, provisional, created_at, updated_at

queue_entries
  id, player_id, queued_at, status

matches
  id, player1_id, player2_id, version_profile_id,
  seed_category, seed_value, status, created_at

match_events
  id, match_id, player_id, event_type, event_data, timestamp

ratings
  id, player_id, match_id, old_elo, new_elo, delta, created_at

runner_slots
  id, worker_id, slot_index, status, match_id,
  port, profile_id, started_at, cleanup_at
```
