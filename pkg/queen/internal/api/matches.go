package api

import (
	"errors"
	"net/http"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
)

// MatchResponse mirrors store.Match minus the private Elo columns.
// Phase 6 will populate them at match finalize; Phase 2 leaves them nil.
type MatchResponse struct {
	ID        int64  `json:"id"`
	Player1ID int64  `json:"player1_id"`
	Player2ID int64  `json:"player2_id"`
	Category  string `json:"category"`
	SeedValue int64  `json:"seed_value"`
	Status    string `json:"status"`
	WinnerID  *int64 `json:"winner_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

func toMatchResponse(m *store.Match) *MatchResponse {
	if m == nil {
		return nil
	}
	return &MatchResponse{
		ID:        m.ID,
		Player1ID: m.Player1ID,
		Player2ID: m.Player2ID,
		Category:  m.Category,
		SeedValue: m.SeedValue,
		Status:    m.Status,
		WinnerID:  m.WinnerID,
		CreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// GET /matches/{id}
func (s *Server) handleGetMatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, err := parseInt64(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid match id", err)
		return
	}
	m, err := s.store.GetMatch(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "match not found", err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toMatchResponse(m))
}

// GET /me/match — current player's most recent pending/active match.
func (s *Server) handleMyMatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	p, err := s.identify(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "identify failed", err)
		return
	}
	m, err := s.store.FindLatestActiveMatch(r.Context(), p.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{
			"match":  nil,
			"player": toPlayerResponse(p),
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"match":  toMatchResponse(m),
		"player": toPlayerResponse(p),
	})
}
