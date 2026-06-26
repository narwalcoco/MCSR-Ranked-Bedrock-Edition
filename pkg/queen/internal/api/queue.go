package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
)

// JoinResponse is the JSON returned by POST /queue.
type JoinResponse struct {
	Player PlayerResponse    `json:"player"`
	Queue  *store.QueueEntry `json:"queue,omitempty"`
	Match  *MatchResponse    `json:"match,omitempty"`
}

// POST /queue — current player joins the matchmaking queue.
func (s *Server) handleJoinQueue(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	p, err := s.identify(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "identify failed", err)
		return
	}
	qe, err := s.store.JoinQueue(r.Context(), p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "join failed", err)
		return
	}

	// Kick the matchmaker immediately so the user doesn't wait for the
	// background tick. Notify is non-blocking and coalescing.
	if s.matchmaker != nil {
		go s.matchmaker.Notify()
	}

	resp := JoinResponse{Player: toPlayerResponse(p), Queue: qe}
	if match, err := s.store.FindLatestActiveMatch(r.Context(), p.ID); err == nil {
		resp.Match = toMatchResponse(match)
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// DELETE /queue/{xuid} — admin-grade leave (others leave themselves via DELETE /queue).
func (s *Server) handleLeaveQueue(w http.ResponseWriter, r *http.Request) {
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
	ok, err := s.store.LeaveQueueByPlayerID(r.Context(), p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "leave failed", err)
		return
	}
	status := "no_change"
	if ok {
		status = "left"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status, "xuid": xuid})
}

// DELETE /queue — current player leaves.
func (s *Server) handleSelfLeave(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	p, err := s.identify(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "identify failed", err)
		return
	}
	ok, err := s.store.LeaveQueueByPlayerID(r.Context(), p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "leave failed", err)
		return
	}
	status := "no_change"
	if ok {
		status = "left"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status, "xuid": p.XUID})
}

// GET /queue — admin view of the waiting pool.
func (s *Server) handleListQueue(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	entries, err := s.store.ListWaiting(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := struct {
		Count   int                `json:"count"`
		Waiting []store.QueueEntry `json:"waiting"`
	}{Count: len(entries), Waiting: entries}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode queue list", "err", err)
	}
}
