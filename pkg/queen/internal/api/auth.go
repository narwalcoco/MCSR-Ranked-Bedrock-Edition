package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
)

// identityHeaders are the dev-mode auth headers expected on the request.
// In production the launcher replaces these with a real Xbox identity
// token (Phase 7+ Tauri).
const (
	headerPlayerXUID     = "X-Player-XUID"
	headerPlayerGamertag = "X-Player-Gamertag"
)

// identify resolves the current player from request headers and upserts
// them into the players table. It is intentionally permissive in Phase 2:
// any XUID is accepted. Real Xbox OAuth lands in Phase 7.
func (s *Server) identify(ctx context.Context, r *http.Request) (*store.Player, error) {
	xuid := r.Header.Get(headerPlayerXUID)
	if xuid == "" {
		return nil, errors.New("missing " + headerPlayerXUID + " header (dev-mode auth)")
	}
	gamertag := r.Header.Get(headerPlayerGamertag)
	if gamertag == "" {
		// Fall back to a stable-ish display name for first-time derivers.
		gamertag = "Player-" + xuid
		if len(gamertag) > 32 {
			gamertag = gamertag[:32]
		}
	}
	return s.store.UpsertPlayer(ctx, xuid, gamertag)
}
