package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// SeedSummary is the per-category payload returned by /seeds/categories.
// We intentionally do NOT expose raw seed values here — those stay in the
// per-category files on disk and are fetched on demand for matchmaking.
type SeedSummary struct {
	ID         string `json:"id"`
	Count      int    `json:"count"`
	SourceFile string `json:"source_file"`
	FirstSeed  string `json:"first_seed,omitempty"`
	LastSeed   string `json:"last_seed,omitempty"`
}

// SeedCategoriesResponse is the JSON returned by GET /seeds/categories.
type SeedCategoriesResponse struct {
	ManifestVersion int           `json:"manifest_version"`
	Source          string        `json:"source"`
	GeneratedAt     string        `json:"generated_at"`
	TotalCategories int           `json:"total_categories"`
	TotalSeeds      int           `json:"total_seeds"`
	Categories      []SeedSummary `json:"categories"`
}

func (s *Server) handleSeedCategories(w http.ResponseWriter, r *http.Request) {
	if s.seeds == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "seeds manifest not loaded",
		})
		return
	}
	list := s.seeds.List()
	resp := SeedCategoriesResponse{
		ManifestVersion: s.seeds.Version,
		Source:          s.seeds.Source,
		GeneratedAt:     s.seeds.GeneratedAt,
		TotalCategories: len(list),
		Categories:      make([]SeedSummary, 0, len(list)),
	}
	for _, c := range list {
		resp.TotalSeeds += c.Count
		resp.Categories = append(resp.Categories, SeedSummary{
			ID:         c.ID,
			Count:      c.Count,
			SourceFile: c.SourceFile,
			FirstSeed:  c.FirstSeed,
			LastSeed:   c.LastSeed,
		})
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode seed categories", "err", err)
	}
}
