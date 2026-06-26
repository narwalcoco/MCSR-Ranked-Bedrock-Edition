// Command seedgen extracts categorized seeds from the raw FSG JSON archives
// and writes per-category JSON arrays + a meta manifest.
//
// Usage:
//
//	seedgen \
//	  -raw=resources/seeds/raw \
//	  -out=resources/seeds/categories \
//	  -manifest=resources/seeds/manifest.json \
//	  -source='Fsg-Generator-MCBE-main (ranked-*.json in v1)'
//
// Only filenames present in `categoryMap` are extracted; everything else is
// left in -raw/ for manual inspection. Output is deterministic: seeds within
// each category are sorted as decimal strings before being written.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// version of the manifest schema this tool emits.
const manifestVersion = 1

// categoryMap maps raw filename → canonical category id.
var categoryMap = map[string]string{
	"ranked-bastion.json":       "bastion",
	"ranked-desert-temple.json": "desert_temple",
	"ranked-ruined-portal.json": "ruined_portal",
	"ranked-stronghold.json":    "stronghold",
	"ranked-village.json":       "village",
	"ranked-warped.json":        "warped",
}

// CategorySummary is one entry in manifest.json. Producers avoid storing the
// raw seeds here — those live in categories/<id>.json for sane git diffs.
type CategorySummary struct {
	ID         string `json:"id"`
	Count      int    `json:"count"`
	SourceFile string `json:"source_file"`
	// FirstSeed and LastSeed are the smallest / largest values for quick QA.
	// Empty when Count == 0.
	FirstSeed string `json:"first_seed,omitempty"`
	LastSeed  string `json:"last_seed,omitempty"`
}

// Manifest is the full manifest written to manifest.json.
type Manifest struct {
	Version     int              `json:"version"`
	GeneratedAt string           `json:"generated_at"`
	Source      string           `json:"source"`
	ToolVersion string           `json:"tool_version"`
	Categories  []CategorySummary `json:"categories"`
	Notes       string           `json:"notes,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seedgen:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		rawDir    = flag.String("raw", "resources/seeds/raw", "directory of input JSON arrays")
		outDir    = flag.String("out", "resources/seeds/categories", "directory for per-category outputs")
		manifestP = flag.String("manifest", "resources/seeds/manifest.json", "manifest output path")
		source    = flag.String("source", "Fsg-Generator-MCBE-main (ranked-*.json in v1)", "human-readable source label")
		notes     = flag.String("notes", "Only filenames mapped in seedgen.categoryMap are extracted. Unmapped files (classic.json, dt.json, packshvil.json) are reserved for manual review.", "notes appended to manifest")
	)
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir out: %w", err)
	}

	manifest := Manifest{
		Version:     manifestVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      *source,
		ToolVersion: "seedgen/0.1.0",
		Notes:       *notes,
		Categories:  []CategorySummary{},
	}

	// Process each mapped raw file. Stable order so the manifest is diff-friendly.
	srcNames := make([]string, 0, len(categoryMap))
	for n := range categoryMap {
		srcNames = append(srcNames, n)
	}
	sort.Strings(srcNames)

	for _, srcName := range srcNames {
		categoryID := categoryMap[srcName]
		srcPath := filepath.Join(*rawDir, srcName)

		rawSeeds, err := readSeedArray(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcPath, err)
		}
		deduped := dedupAndSort(rawSeeds)
		if len(deduped) == 0 {
			slog.Warn("no seeds after dedup", "source", srcName)
		}

		outPath := filepath.Join(*outDir, categoryID+".json")
		out, err := json.MarshalIndent(deduped, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, append(out, '\n'), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}

		sum := CategorySummary{
			ID:         categoryID,
			Count:      len(deduped),
			SourceFile: srcName,
		}
		if len(deduped) > 0 {
			sum.FirstSeed = deduped[0]
			sum.LastSeed = deduped[len(deduped)-1]
		}
		manifest.Categories = append(manifest.Categories, sum)

		slog.Info("extracted",
			"category", categoryID,
			"source", srcName,
			"raw_count", len(rawSeeds),
			"unique", len(deduped),
			"out", outPath,
		)
	}

	// Stable, alphabetical category order in the manifest.
	sort.Slice(manifest.Categories, func(i, j int) bool {
		return manifest.Categories[i].ID < manifest.Categories[j].ID
	})

	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(*manifestP, append(mb, '\n'), 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	slog.Info("manifest written", "path", *manifestP, "categories", len(manifest.Categories))

	return nil
}

// readSeedArray reads a JSON file that is just an array of numbers.
// Numbers are decoded as decimal strings to preserve precision.
func readSeedArray(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.UseNumber() // preserve numeric precision
	var raw []json.Number
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	out := make([]string, 0, len(raw))
	for _, n := range raw {
		// json.Number is always a valid integer literal in this dataset.
		s := n.String()
		// Trim a leading '+' (rare, but valid in some tooling).
		if len(s) > 0 && s[0] == '+' {
			s = s[1:]
		}
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// dedupAndSort removes duplicates and returns a sorted []string.
// Empty strings are dropped. Sort is by lexicographic order on decimal string,
// which is the same as numeric order for unsigned decimals (and even for
// signed ints when we explicitly handle '-' first — see below).
func dedupAndSort(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sortSeeds(out)
	return out
}

// sortSeeds sorts seeds as signed decimal numbers. This is what MCBE actually
// cares about: e.g. "-17" < "1" < "11".
func sortSeeds(s []string) {
	sort.Slice(s, func(i, j int) bool {
		return compareDecimal(s[i], s[j]) < 0
	})
}

// compareDecimal returns -1, 0, or +1 in numeric order.
func compareDecimal(a, b string) int {
	aNeg := len(a) > 0 && a[0] == '-'
	bNeg := len(b) > 0 && b[0] == '-'
	if aNeg != bNeg {
		if aNeg {
			return -1
		}
		return 1
	}
	// Same sign → compare absolute values as unsigned decimal, accounting for
	// length because Go sort treats strings lexicographically (-17 < -9 is false).
	ai := a
	bi := b
	if aNeg {
		ai = ai[1:]
	}
	if bNeg {
		bi = bi[1:]
	}
	if len(ai) != len(bi) {
		if len(ai) < len(bi) {
			return negIf(aNeg, -1)
		}
		return negIf(aNeg, +1)
	}
	if ai < bi {
		return negIf(aNeg, -1)
	}
	if ai > bi {
		return negIf(aNeg, +1)
	}
	return 0
}

func negIf(neg bool, v int) int {
	if neg {
		return -v
	}
	return v
}
