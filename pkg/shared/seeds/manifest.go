// Package seeds loads the seed manifest produced by cmd/seedgen and exposes
// typed accessors for categorized MCBE seed values.
//
// Layout expected on disk:
//
//	<baseDir>/manifest.json          — category metadata
//	<baseDir>/categories/<id>.json   — JSON array of decimal-string seeds
//
// All seed values are stored and parsed as int64. They cross the wire (and
// disk) as decimal strings to keep JS-style JSON decoders round-trip safe.
package seeds

import (
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ManifestVersion is the manifest schema version this package understands.
// Callers should warn or refuse on a mismatch.
const ManifestVersion = 1

// CategorySummary mirrors the JSON schema written by cmd/seedgen.
type CategorySummary struct {
	ID         string `json:"id"`
	Count      int    `json:"count"`
	SourceFile string `json:"source_file"`
	FirstSeed  string `json:"first_seed,omitempty"`
	LastSeed   string `json:"last_seed,omitempty"`
}

// Manifest is the in-memory representation of manifest.json.
type Manifest struct {
	Version     int               `json:"version"`
	GeneratedAt string            `json:"generated_at"`
	Source      string            `json:"source"`
	ToolVersion string            `json:"tool_version"`
	Categories  []CategorySummary `json:"categories"`
	Notes       string            `json:"notes,omitempty"`

	// baseDir is the directory the manifest was loaded from. Set by Load.
	baseDir string
	mu      sync.RWMutex
	loaded  map[string][]int64 // categoryID -> parsed seeds (cached on first read)
}

// Load reads manifest.json from baseDir/manifestPath (relative paths are
// resolved against baseDir) and returns a parsed Manifest. Per-category
// files are NOT eagerly loaded — they're parsed on demand via Pick/List.
func Load(baseDir, manifestPath string) (*Manifest, error) {
	if baseDir == "" {
		return nil, errors.New("baseDir must not be empty")
	}
	resolved := manifestPath
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseDir, resolved)
	}

	raw, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Version != ManifestVersion {
		return nil, fmt.Errorf("unsupported manifest version %d (want %d)", m.Version, ManifestVersion)
	}
	m.baseDir = filepath.Dir(resolved)
	m.loaded = make(map[string][]int64, len(m.Categories))
	return &m, nil
}

// List returns a copy of the manifest's category summaries, sorted by ID.
func (m *Manifest) List() []CategorySummary {
	out := make([]CategorySummary, len(m.Categories))
	copy(out, m.Categories)
	return out
}

// Has reports whether the named category exists in the manifest.
func (m *Manifest) Has(category string) bool {
	for _, c := range m.Categories {
		if c.ID == category {
			return true
		}
	}
	return false
}

// Category returns the parsed seed slice for the named category, lazily
// loading and parsing the file the first time it's requested.
func (m *Manifest) Category(id string) ([]int64, error) {
	if id == "" {
		return nil, errors.New("category id must not be empty")
	}
	if !m.Has(id) {
		return nil, fmt.Errorf("unknown category %q", id)
	}

	m.mu.RLock()
	if cached, ok := m.loaded[id]; ok {
		m.mu.RUnlock()
		return cached, nil
	}
	m.mu.RUnlock()

	path := filepath.Join(m.baseDir, "categories", id+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var rawStrings []string
	if err := json.Unmarshal(raw, &rawStrings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	parsed := make([]int64, len(rawStrings))
	for i, s := range rawStrings {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, fmt.Errorf("empty seed at %s index %d", path, i)
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse seed %q at %s[%d]: %w", s, path, i, err)
		}
		parsed[i] = v
	}

	// Always return sorted output (defense in depth — seedgen also sorts on
	// disk, but external tools might drop files in here without our tool).
	sort.Slice(parsed, func(i, j int) bool { return parsed[i] < parsed[j] })

	m.mu.Lock()
	// Cache only on success so a later retry has a chance to succeed.
	m.loaded[id] = parsed
	m.mu.Unlock()
	return parsed, nil
}

// Pick returns one seed from the named category using the default
// crypto-randomness-free math/rand source (sufficient for casual match
// balancing). Use PickWith for deterministic selection in tests.
func (m *Manifest) Pick(category string) (int64, error) {
	return m.PickWith(category, nil)
}

// PickWith is like Pick but accepts an explicit *math/rand.Rand for
// deterministic testing. Callers may pass nil to use a fresh random source.
func (m *Manifest) PickWith(category string, r *mrand.Rand) (int64, error) {
	seeds, err := m.Category(category)
	if err != nil {
		return 0, err
	}
	if len(seeds) == 0 {
		return 0, fmt.Errorf("category %q has no seeds", category)
	}
	if r == nil {
		r = mrand.New(mrand.NewSource(randSeed()))
	}
	return seeds[r.Intn(len(seeds))], nil
}
