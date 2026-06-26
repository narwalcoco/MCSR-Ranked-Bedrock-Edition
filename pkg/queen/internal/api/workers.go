package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
)

// --- Worker registration --------------------------------------------------

// registerWorkerRequest is the JSON body for POST /workers.
type registerWorkerRequest struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	MaxSlots int    `json:"max_slots"`
}

func (s *Server) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	var req registerWorkerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body", err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", nil)
		return
	}
	if req.Host == "" {
		writeError(w, http.StatusBadRequest, "host is required", nil)
		return
	}
	if req.MaxSlots < 1 || req.MaxSlots > 64 {
		writeError(w, http.StatusBadRequest, "max_slots must be 1-64", nil)
		return
	}

	worker, err := s.store.RegisterWorker(r.Context(), req.Name, req.Host, req.Port, req.MaxSlots)
	if err != nil {
		slog.Error("register worker", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to register worker", err)
		return
	}

	slog.Info("worker registered",
		"id", worker.ID,
		"name", worker.Name,
		"host", worker.Host,
		"max_slots", worker.MaxSlots,
	)
	writeJSON(w, http.StatusCreated, worker)
}

// --- Worker heartbeat ------------------------------------------------------

func (s *Server) handleWorkerHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid worker id", err)
		return
	}

	if err := s.store.HeartbeatWorker(r.Context(), id); err != nil {
		slog.Warn("worker heartbeat failed", "id", id, "err", err)
		writeError(w, http.StatusNotFound, "worker not found", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- List workers ----------------------------------------------------------

func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	workers, err := s.store.ListWorkers(r.Context())
	if err != nil {
		slog.Error("list workers", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list workers", err)
		return
	}
	if workers == nil {
		workers = []store.Worker{}
	}
	writeJSON(w, http.StatusOK, workers)
}

// --- Get worker ------------------------------------------------------------

func (s *Server) handleGetWorker(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid worker id", err)
		return
	}

	worker, err := s.store.GetWorker(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "worker not found", err)
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

// --- Worker slots ----------------------------------------------------------

func (s *Server) handleListWorkerSlots(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid worker id", err)
		return
	}

	slots, err := s.store.ListWorkerSlots(r.Context(), id)
	if err != nil {
		slog.Error("list worker slots", "worker", id, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list slots", err)
		return
	}
	if slots == nil {
		slots = []store.RunnerSlot{}
	}
	writeJSON(w, http.StatusOK, slots)
}

// --- Free slots ------------------------------------------------------------

func (s *Server) handleListFreeSlots(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	slots, err := s.store.ListFreeSlots(r.Context())
	if err != nil {
		slog.Error("list free slots", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list free slots", err)
		return
	}
	if slots == nil {
		slots = []store.RunnerSlot{}
	}
	writeJSON(w, http.StatusOK, slots)
}

// --- Worker drain ----------------------------------------------------------

func (s *Server) handleDrainWorker(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid worker id", err)
		return
	}

	if err := s.store.DrainWorker(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "worker not found", err)
		return
	}
	slog.Info("worker draining", "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "draining"})
}

// Silence unused-import warning for context when its only consumer
// (a future handler) hasn't landed yet.
var _ = context.TODO
