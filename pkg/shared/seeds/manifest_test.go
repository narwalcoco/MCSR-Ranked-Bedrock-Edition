package seeds

import (
	mrand "math/rand"
	"os"
	"path/filepath"
	"testing"
)

const testManifest = `{
  "version": 1,
  "generated_at": "2024-01-01T00:00:00Z",
  "source": "test fixture",
  "tool_version": "seedgen/test",
  "categories": [
    {"id": "stronghold", "count": 3, "source_file": "x.json", "first_seed": "1", "last_seed": "100"},
    {"id": "bastion",    "count": 2, "source_file": "y.json", "first_seed": "-99", "last_seed": "5"},
    {"id": "empty",      "count": 0, "source_file": "z.json"}
  ]
}`

// writeFixture lays out a temp dir with manifest.json + a few category files.
func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "manifest.json"), testManifest)
	mustWrite(t, filepath.Join(dir, "categories", "stronghold.json"), `["100","1","42"]`)
	mustWrite(t, filepath.Join(dir, "categories", "bastion.json"), `["5","-99"]`)
	mustWrite(t, filepath.Join(dir, "categories", "empty.json"), `[]`)
	return dir
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoad(t *testing.T) {
	dir := writeFixture(t)
	m, err := Load(dir, "manifest.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Version != 1 {
		t.Fatalf("Version = %d, want 1", m.Version)
	}
	if got := len(m.Categories); got != 3 {
		t.Fatalf("Categories = %d, want 3", got)
	}
}

func TestList(t *testing.T) {
	dir := writeFixture(t)
	m, _ := Load(dir, "manifest.json")
	list := m.List()
	if len(list) != 3 {
		t.Fatalf("List len = %d", len(list))
	}
	want := map[string]bool{"stronghold": false, "bastion": false, "empty": false}
	for _, c := range list {
		if _, ok := want[c.ID]; ok {
			want[c.ID] = true
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("missing category %q in List()", id)
		}
	}
}

func TestCategory_ParseAndSort(t *testing.T) {
	dir := writeFixture(t)
	m, _ := Load(dir, "manifest.json")
	got, err := m.Category("stronghold")
	if err != nil {
		t.Fatalf("Category: %v", err)
	}
	want := []int64{1, 42, 100}
	if !equalInts(got, want) {
		t.Errorf("stronghold = %v, want %v", got, want)
	}

	// bastion has negatives — verify signed sort.
	got, err = m.Category("bastion")
	if err != nil {
		t.Fatalf("Category bastion: %v", err)
	}
	want = []int64{-99, 5}
	if !equalInts(got, want) {
		t.Errorf("bastion = %v, want %v", got, want)
	}
}

func TestCategory_UnknownErrors(t *testing.T) {
	dir := writeFixture(t)
	m, _ := Load(dir, "manifest.json")
	if _, err := m.Category("nope"); err == nil {
		t.Fatal("expected error for unknown category")
	}
}

func TestCategory_CachesResult(t *testing.T) {
	dir := writeFixture(t)
	m, _ := Load(dir, "manifest.json")
	first, err := m.Category("stronghold")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Mutate the file to a wrong value — should NOT affect the cached value.
	mustWrite(t, filepath.Join(dir, "categories", "stronghold.json"), `["999"]`)
	second, err := m.Category("stronghold")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !equalInts(first, second) {
		t.Errorf("expected cached result; got first=%v second=%v", first, second)
	}
}

func TestPick_DeterministicWithExplicitRand(t *testing.T) {
	dir := writeFixture(t)
	m, _ := Load(dir, "manifest.json")
	// Two identical sources must produce identical picks (Pick must be
	// deterministic given the same RNG state). We don’t pin a specific
	// expected value because math/rand’s Intn sequence is implementation-
	// defined and could change between Go versions.
	for i := 0; i < 5; i++ {
		r1 := mrand.New(mrand.NewSource(int64(i)))
		v1, err := m.PickWith("stronghold", r1)
		if err != nil {
			t.Fatalf("Pick #%d: %v", i, err)
		}
		r2 := mrand.New(mrand.NewSource(int64(i)))
		v2, err := m.PickWith("stronghold", r2)
		if err != nil {
			t.Fatalf("Pick #%d (second): %v", i, err)
		}
		if v1 != v2 {
			t.Errorf("Pick not deterministic at seed=%d: first=%d second=%d", i, v1, v2)
		}
		allowed := map[int64]bool{1: true, 42: true, 100: true}
		if !allowed[v1] {
			t.Errorf("Pick at seed=%d returned %d, not a fixture value {1,42,100}", i, v1)
		}
	}
}

func TestPick_EmptyCategoryErrors(t *testing.T) {
	dir := writeFixture(t)
	m, _ := Load(dir, "manifest.json")
	if _, err := m.Pick("empty"); err == nil {
		t.Fatal("expected error picking from empty category")
	}
}

func TestLoad_WrongVersionErrors(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "manifest.json"), `{"version":2,"categories":[]}`)
	if _, err := Load(dir, "manifest.json"); err == nil {
		t.Fatal("expected error for unsupported manifest version")
	}
}

func TestLoad_MissingFileErrors(t *testing.T) {
	if _, err := Load(t.TempDir(), "manifest.json"); err == nil {
		t.Fatal("expected error for missing manifest file")
	}
}

// equalInts is a small helper for table-style asserts.
func equalInts(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
