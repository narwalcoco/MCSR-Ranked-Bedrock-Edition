package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/db"
	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/matchmaker"
	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
	"github.com/mcsr-ranked-bedrock/pkg/shared/seeds"
	"github.com/mcsr-ranked-bedrock/pkg/shared/version"
)

// Server bundles HTTP handlers with shared dependencies.
type Server struct {
	db         *db.DB
	store      *store.Store
	seeds      *seeds.Manifest
	matchmaker *matchmaker.Matchmaker
	start      time.Time
	info       version.Info
	mux        *http.ServeMux
}

// New constructs a Server wired with the given dependencies.
// Any dependency may be nil — handlers defensively report unavailable
// instead of nil-dereferencing, so dev runs with partial config still work.
func New(database *db.DB, store *store.Store, mm *matchmaker.Matchmaker, manifest *seeds.Manifest, info version.Info) *Server {
	s := &Server{
		db:         database,
		store:      store,
		seeds:      manifest,
		matchmaker: mm,
		start:      time.Now(),
		info:       info,
		mux:        http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the configured http.Handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// requireStore returns false (and writes a 503) when the store isn't wired.
func (s *Server) requireStore(w http.ResponseWriter) bool {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not configured", nil)
		return false
	}
	return true
}

// routes registers every HTTP handler. Go 1.22+ pattern syntax with
// method-qualified prefixes lets us GET/POST/DELETE without per-method maps.
func (s *Server) routes() {
	// Phase 0/1: identity, version, health, seed categories.
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("GET /seeds/categories", s.handleSeedCategories)

	// Phase 2: players + queue + matches.
	s.mux.HandleFunc("GET /me", s.handleMe)
	s.mux.HandleFunc("GET /me/match", s.handleMyMatch)
	s.mux.HandleFunc("POST /players", s.handleUpsertPlayer)
	s.mux.HandleFunc("GET /players/{xuid}", s.handleGetPlayer)
	s.mux.HandleFunc("POST /queue", s.handleJoinQueue)
	s.mux.HandleFunc("DELETE /queue", s.handleSelfLeave)
	s.mux.HandleFunc("DELETE /queue/{xuid}", s.handleLeaveQueue)
	s.mux.HandleFunc("GET /queue", s.handleListQueue)
	s.mux.HandleFunc("GET /matches/{id}", s.handleGetMatch)

	// Silence unused-import warning for context when its only consumer
	// (a future handler) hasn't landed yet.
	_ = context.TODO
}

// HealthResponse describes service liveness.
type HealthResponse struct {
	Status      string `json:"status"`
	Service     string `json:"service"`
	Uptime      string `json:"uptime"`
	DBReachable bool   `json:"db_reachable"`
	DBVersion   int    `json:"db_version"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	QueueCount  int    `json:"queue_count"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	resp := HealthResponse{
		Service: "queen",
		Uptime:  time.Since(s.start).Round(time.Second).String(),
		Version: s.info.Version,
		Commit:  s.info.Commit,
	}

	schema, err := s.db.SchemaVersion(ctx)
	if err != nil {
		resp.Status = "degraded"
		resp.DBReachable = false
		slog.Warn("health: db unreachable", "err", err)
	} else {
		resp.DBReachable = true
		resp.DBVersion = schema
		resp.Status = "ok"
	}

	if s.store != nil {
		if entries, err := s.store.ListWaiting(ctx, 1000); err == nil {
			resp.QueueCount = len(entries)
		}
	}

	status := http.StatusOK
	if resp.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.info)
}

// writeJSON writes body as indented JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		slog.Error("write json", "err", err)
	}
}
