// Package matchmaker pairs queued players and creates Match rows.
//
// Phase 2 intentionally pairs the two oldest waiting players and picks a
// random category + seed. Phase 6 will layer rating-based pairing and
// Phase 7+ (Tauri) may add category preference.
package matchmaker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
	"github.com/mcsr-ranked-bedrock/pkg/shared/seeds"
)

// ErrNoManifest is returned by Tick when no seed manifest is loaded.
var ErrNoManifest = errors.New("matchmaker: no seed manifest")

// Matchmaker runs in the background. Call Run from a goroutine; it
// blocks until ctx is cancelled.
type Matchmaker struct {
	store    *store.Store
	seeds    *seeds.Manifest
	interval time.Duration
	logger   *slog.Logger

	mu      sync.Mutex
	notify  chan struct{}
	stopped bool
}

// New constructs a matchmaker. The interval is the fallback poll period;
// on-demand ticks (via Notify) run synchronously without waiting.
func New(s *store.Store, m *seeds.Manifest, interval time.Duration, logger *slog.Logger) *Matchmaker {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Matchmaker{
		store:    s,
		seeds:    m,
		interval: interval,
		logger:   logger,
		notify:   make(chan struct{}, 1),
	}
}

// Run loops until ctx is cancelled. It is safe to call from a goroutine.
func (m *Matchmaker) Run(ctx context.Context) error {
	t := time.NewTicker(m.interval)
	defer t.Stop()
	m.logger.Info("matchmaker started", "interval", m.interval)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("matchmaker stopping")
			return ctx.Err()
		case <-t.C:
			if _, err := m.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				m.logger.Warn("matchmaker tick failed", "err", err)
			}
		case <-m.notify:
			if _, err := m.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				m.logger.Warn("matchmaker on-demand tick failed", "err", err)
			}
		}
	}
}

// Notify requests an immediate match attempt without blocking. Safe from
// any goroutine. Subsequent calls before the tick fires are coalesced.
func (m *Matchmaker) Notify() {
	select {
	case m.notify <- struct{}{}:
	default:
		// Already pending — that's fine, the loop will coalesce.
	}
}

// Tick pairs as many queue entries as possible and creates matches. It
// returns the number of matches successfully created this tick.
//
// Tick is safe to call concurrently with Run — only one will actually
// talk to the DB at a time per process because the SQLite connection
// pool is size 1 (set in db.Open).
func (m *Matchmaker) Tick(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.seeds == nil {
		return 0, ErrNoManifest
	}

	const batchSize = 50
	waiting, err := m.store.ListWaiting(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("list waiting: %w", err)
	}
	if len(waiting) < 2 {
		return 0, nil
	}

	matched := 0
	for i := 0; i+1 < len(waiting); i += 2 {
		p1 := waiting[i]
		p2 := waiting[i+1]
		// Sanity: never pair a player with themselves (defense in depth
		// — the schema CHECK also enforces this).
		if p1.PlayerID == p2.PlayerID {
			continue
		}
		seed, category, err := m.pickSeed(ctx)
		if err != nil {
			// Stop pairing if we can't pick seeds; the next tick will retry.
			m.logger.Warn("seed pick failed", "err", err)
			return matched, err
		}
		match, err := m.store.CreateMatch(ctx, p1.PlayerID, p2.PlayerID, category, seed)
		if err != nil {
			m.logger.Warn("create match failed",
				"p1_xuid", p1.XUID, "p2_xuid", p2.XUID, "err", err)
			continue
		}
		matched++
		m.logger.Info("match created",
			"match_id", match.ID,
			"p1", p1.Gamertag, "p2", p2.Gamertag,
			"category", category, "seed", seed,
		)
	}
	return matched, nil
}

// pickSeed picks a random category then a random seed within it.
// Centralized here so future rating-aware logic lands in one place.
func (m *Matchmaker) pickSeed(ctx context.Context) (int64, string, error) {
	cats := m.seeds.List()
	if len(cats) == 0 {
		return 0, "", errors.New("no categories in manifest")
	}
	// math/rand in seeds.PickWith already, but here we also need the
	// category, so we do both here with a single local RNG.
	category := cats[seedRandomIndex(len(cats))].ID
	seed, err := m.seeds.Pick(category)
	if err != nil {
		return 0, "", err
	}
	return seed, category, nil
}
