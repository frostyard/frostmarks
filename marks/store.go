package marks

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/frostyard/hearth/events/outbox"
)

// schemaVersion is the version of the app's schema, in PRAGMA user_version.
const schemaVersion = 1

// pragmas are the pragmas of every connection: foreign keys on, a busy database
// that waits 5 s before it reports a lock, and WAL.
const pragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

// Open opens the app's database at path, it makes the schema, and it makes the
// outbox's table.
//
// The database holds 1 connection: SQLite writes 1 at a time, so the app's
// transactions and the relay's deletes wait for each other, and they never fail
// with a lock. Foreign keys are on, so a deleted bookmark takes its tags with
// it, and a busy database waits up to 5 s before it reports a lock.
//
// A second Open of the same database is a no-op: PRAGMA user_version tells it
// that the schema is there.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn, err := dataSourceName(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open the database %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := outbox.Init(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// dataSourceName returns the URI of the app's database at path, with the pragmas
// of every connection.
//
// The path is escaped as a URL path: a directory may hold any character, and a
// URI reads an unescaped "?" or "#" as its own syntax, which would open another
// file or none. A relative path is made absolute first, so that the URI names 1
// file whatever the process's directory is.
func dataSourceName(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("read the database's path %s: %w", path, err)
	}
	uri := &url.URL{Scheme: "file", Path: absolute, RawQuery: pragmas}
	return uri.String(), nil
}

// migrate runs the schema's steps up to schemaVersion, each in 1 transaction,
// and it refuses a database of a later version.
func migrate(ctx context.Context, db *sql.DB) error {
	steps := [][]string{
		{
			// A folder is 1 path, and the root path is never stored: it
			// always exists.
			`CREATE TABLE folders (
	path TEXT PRIMARY KEY
)`,
			// A bookmark is 1 URL in 1 folder. The times are Unix nanoseconds
			// in UTC. The same URL in the same folder is refused.
			`CREATE TABLE bookmarks (
	id          TEXT PRIMARY KEY,
	url         TEXT NOT NULL,
	title       TEXT NOT NULL,
	notes       TEXT NOT NULL,
	folder      TEXT NOT NULL,
	create_time INTEGER NOT NULL,
	update_time INTEGER NOT NULL,
	UNIQUE(folder, url)
)`,
			// A tag of 1 bookmark: lower-case, and 1 tag per bookmark.
			`CREATE TABLE bookmark_tags (
	bookmark_id TEXT NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
	tag         TEXT NOT NULL,
	PRIMARY KEY (bookmark_id, tag)
)`,
			// The search index of every bookmark, over its title, its URL, its
			// notes and its tags.
			`CREATE VIRTUAL TABLE bookmarks_fts USING fts5(
	id UNINDEXED,
	title,
	url,
	notes,
	tags
)`,
			`CREATE INDEX bookmark_tags_tag ON bookmark_tags (tag)`,
		},
	}
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read the schema version: %w", err)
	}
	switch {
	case version > schemaVersion:
		return fmt.Errorf("the database is at schema version %d, and this build knows version %d", version, schemaVersion)
	case version == schemaVersion:
		return nil
	}
	for i := version; i < schemaVersion; i++ {
		if err := applyStep(ctx, db, steps[i], i+1); err != nil {
			return err
		}
	}
	return nil
}

// applyStep runs the statements of 1 schema step and sets the schema version, in
// 1 transaction: a step is there wholly, or not at all.
func applyStep(ctx context.Context, db *sql.DB, statements []string, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin the transaction of schema version %d: %w", version, err)
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("make schema version %d: %w", version, err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("set schema version %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema version %d: %w", version, err)
	}
	return nil
}
