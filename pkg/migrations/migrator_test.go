package migrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigrationsFromFS(t *testing.T) {
	tmp := t.TempDir()

	upName := "001_create_table.up.sql"
	downName := "001_create_table.down.sql"

	upPath := filepath.Join(tmp, upName)
	downPath := filepath.Join(tmp, downName)

	upSQL := `CREATE TABLE IF NOT EXISTS test_table (id INT PRIMARY KEY);`
	downSQL := `DROP TABLE IF EXISTS test_table;`

	if err := os.WriteFile(upPath, []byte(upSQL), 0644); err != nil {
		t.Fatalf("failed to write up migration: %v", err)
	}
	if err := os.WriteFile(downPath, []byte(downSQL), 0644); err != nil {
		t.Fatalf("failed to write down migration: %v", err)
	}

	// Use os.DirFS to provide an fs.FS rooted at the temp directory
	fsys := os.DirFS(tmp)

	migs, err := LoadMigrationsFromFS(fsys, ".")
	if err != nil {
		t.Fatalf("LoadMigrationsFromFS returned error: %v", err)
	}

	if len(migs) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migs))
	}

	m := migs[0]
	if m.Version() != "001" {
		t.Fatalf("expected version 001, got %s", m.Version())
	}
	if m.Description() != "create_table" {
		t.Fatalf("expected description create_table, got %s", m.Description())
	}

	// Ensure the migration implements Migration and Up/Down methods exist (can't execute without DB)
	// Also ensure sorting does not error when multiple migrations present
	// Create an extra migration file with higher version and ensure order
	upName2 := "002_add_index.up.sql"
	upPath2 := filepath.Join(tmp, upName2)
	if err := os.WriteFile(upPath2, []byte("CREATE INDEX idx_test ON test_table (id);"), 0644); err != nil {
		t.Fatalf("failed to write second up migration: %v", err)
	}

	migs2, err := LoadMigrationsFromFS(fsys, ".")
	if err != nil {
		t.Fatalf("LoadMigrationsFromFS returned error on second load: %v", err)
	}
	if len(migs2) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migs2))
	}
	// sorted by version ascending
	if migs2[0].Version() != "001" || migs2[1].Version() != "002" {
		t.Fatalf("expected ordered versions 001,002 got %s,%s", migs2[0].Version(), migs2[1].Version())
	}
}
