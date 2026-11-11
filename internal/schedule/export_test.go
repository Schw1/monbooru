package schedule

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/monbooru/monbooru/internal/config"
)

func TestPruneExportsKeepsNewestOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"main-2026-11-01_010000.db",
		"main-2026-11-02_010000.db",
		"main-2026-11-03_010000.db",
		"main-2026-11-04_010000.db",
		// Not this format, not this gallery, or not stamped: left alone.
		"main-2026-11-01_010000.json",
		"main-2026-11-01_010000-light.json",
		"other-2026-11-01_010000.db",
		"main-backup.db",
		"main-extra-2026-11-01_010000.db",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pruneExports(dir, "main", config.ExportFormatDB, 2)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{
		"main-2026-11-01_010000-light.json",
		"main-2026-11-01_010000.json",
		"main-2026-11-03_010000.db",
		"main-2026-11-04_010000.db",
		"main-backup.db",
		"main-extra-2026-11-01_010000.db",
		"other-2026-11-01_010000.db",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("after prune:\n got %v\nwant %v", got, want)
	}
}
