package marks

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenTwice checks that the schema is made once, that a second Open of the
// same database is a no-op migration, and that the schema version is the 1 this
// build knows.
func TestOpenTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frostmarks.db")
	first, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("the first Open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close the first database: %v", err)
	}
	second, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("the second Open: %v", err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	var version int
	if err := second.QueryRowContext(t.Context(), `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read the schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("the schema version = %d, want %d", version, schemaVersion)
	}
	for _, table := range []string{"folders", "bookmarks", "bookmark_tags", "bookmarks_fts", "hearth_outbox"} {
		var name string
		err := second.QueryRowContext(t.Context(), `SELECT name FROM sqlite_master WHERE name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("the table %s is not in the database: %v", table, err)
		}
	}
	var fileFormat string
	if err := second.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&fileFormat); err != nil {
		t.Fatalf("read the journal mode: %v", err)
	}
	if fileFormat != "wal" {
		t.Errorf("the journal mode = %q, want wal", fileFormat)
	}
}

// TestOpenEscapedPath checks the database's URI: a directory whose name holds the
// characters of a URI's own syntax opens, and the database file is at that exact
// path.
func TestOpenEscapedPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a?b#c%41 d")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("make the directory: %v", err)
	}
	path := filepath.Join(dir, "frostmarks.db")
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the database file at %q: %v", path, err)
	}
	var version int
	if err := db.QueryRowContext(t.Context(), `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read the schema version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("the schema version = %d, want %d", version, schemaVersion)
	}
	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Errorf("the write-ahead log of the database at %q: %v", path, err)
	}
}
