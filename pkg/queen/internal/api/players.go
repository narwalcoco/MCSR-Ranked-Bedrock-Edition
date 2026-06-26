package api

import (
	"errors"
	"net/http"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
)

// PlayerResponse is the JSON shape returned by player endpoints.
type PlayerResponse struct {
	ID          int64  `json:"id"`
	XUID        string `json:"xuid"`
	Gamertag    string `json:"gamertag"`
	Elo         int    `json:"elo"`
	Provisional bool   `json:"provisional"`
}

func toPlayerResponse(p *store.Player) PlayerResponse {
	return PlayerResponse{
		ID:          p.ID,
		XUID:        p.XUID,
		Gamertag:    p.Gamertag,
		Elo:         p.Elo,
		Provisional: p.Provisional,
	}
}

// POST /players — body {"xuid":"...","gamertag":"..."}
// Upserts a player. Useful for the launcher "register me" flow.
func (s *Server) handleUpsertPlayer(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	type body struct {
		XUID     string `json:"xuid"`
		Gamertag string `json:"gamertag"`
	}
	var b body
	if err := decodeJSON(r, &b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body", err)
		return
	}
	if b.XUID == "" {
		writeError(w, http.StatusBadRequest, "xuid is required", nil)
		return
	}
	if b.Gamertag == "" {
		b.Gamertag = "Player-" + b.XUID
	}
	p, err := s.store.UpsertPlayer(r.Context(), b.XUID, b.Gamertag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upsert failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toPlayerResponse(p))
}

// GET /players/{xuid}
func (s *Server) handleGetPlayer(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	xuid := r.PathValue("xuid")
	if xuid == "" {
		writeError(w, http.StatusBadRequest, "missing xuid", nil)
		return
	}
	p, err := s.store.GetPlayerByXUID(r.Context(), xuid)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "player not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toPlayerResponse(p))
}

// GET /me — returns the current player resolved from headers plus their
// latest pending/active match (so the Tauri launcher can react to match
// found! events via polling for now).
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	p, err := s.identify(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "identify failed", err)
		return
	}
	resp := struct {
		Player PlayerResponse `json:"player"`
		Match  *MatchResponse `json:"match,omitempty"`
	}{Player: toPlayerResponse(p)}

	if match, err := s.store.FindLatestActiveMatch(r.Context(), p.ID); err == nil {
		resp.Match = toMatchResponse(match)
	} else if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "match lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
