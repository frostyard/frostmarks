// Package marks is frostmarks' app core: bookmarks and the folders that hold
// them, stored in SQLite, with 2 capabilities (marks:read and marks:write) on
// the folder selector.
//
// The service writes each change and its event in 1 SQLite transaction (hearth's
// outbox), and it wakes the relay after the commit, so that no change is
// published without its event, and no event without its change. The permission
// checks are hearth's: the generated check of every RPC, the generated hook of
// each RPC that runs on an id, and the grant filter of the app's lists (see
// authorize.go).
package marks

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/frostyard/hearth/authz"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// The app's 2 capabilities: marks:read and marks:write, each on the folder
// selector.
const (
	// readCap reads 1 bookmark or 1 folder.
	readCap = "marks:read"
	// writeCap changes 1 bookmark or 1 folder.
	writeCap = "marks:write"
)

// Service is frostmarks' app core: the database, the clock and the id source of
// its changes, and the wake of the outbox relay.
type Service struct {
	// DB is the app's database, from Open.
	DB *sql.DB
	// Notify wakes the outbox relay after a change that wrote 1 event. nil is
	// no wake.
	Notify func()
	// Now is the clock of a bookmark's create_time and update_time. nil is
	// time.Now.
	Now func() time.Time
	// NewID makes 1 bookmark's id: a UUIDv7. nil is uuid.NewV7.
	NewID func() (string, error)
}

// API returns the app's API with the generated permission check of every RPC.
func (s *Service) API() marksv1.MarksAPI {
	return marksv1.NewMarksAuthz[*sql.Tx](s, s.inTx)
}

// inTx runs fn in 1 transaction, commits it, and then wakes the relay, which
// publishes the events that the transaction wrote. A rollback leaves neither
// the change nor its event.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin the transaction: %w", err)
	}
	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit the transaction: %w", err)
	}
	if s.Notify != nil {
		s.Notify()
	}
	return nil
}

// now returns the current time of the app's clock.
func (s *Service) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// newID returns the id of 1 new bookmark: a UUIDv7 of the app's id source.
func (s *Service) newID() (string, error) {
	if s.NewID != nil {
		return s.NewID()
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("make the bookmark's id: %w", err)
	}
	return id.String(), nil
}

// querier is what reads and writes the app's rows: the database, or 1
// transaction of it.
type querier interface {
	// QueryContext runs 1 query that returns rows.
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	// QueryRowContext runs 1 query that returns 1 row.
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	// ExecContext runs 1 statement.
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// transaction returns the transaction of 1 RPC that runs on an id: the
// generated hook of that RPC opened it.
func transaction(ctx context.Context) (*sql.Tx, error) {
	tx, ok := authz.TxFrom[*sql.Tx](ctx)
	if !ok {
		return nil, internal("the call runs outside the app's transaction")
	}
	return tx, nil
}
