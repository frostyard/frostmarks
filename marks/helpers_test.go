package marks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/frostyard/hearth/authz"
	"github.com/frostyard/hearth/events"
	"github.com/frostyard/hearth/events/inproc"
	"github.com/frostyard/hearth/events/outbox"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// testClock is the app's clock in a test: every read is 1 s later than the read
// before it, so that a change's time is always a new 1.
type testClock struct {
	// at is the time of the next read.
	at time.Time
}

// Now returns the clock's next time.
func (c *testClock) Now() time.Time {
	c.at = c.at.Add(time.Second)
	return c.at
}

// testIDs is the app's id source in a test: a UUIDv7-shaped id with a counter at
// its end.
type testIDs struct {
	// n is how many ids the source gave.
	n int
}

// New returns the source's next id.
func (i *testIDs) New() (string, error) {
	i.n++
	return fmt.Sprintf("019a8b6e-6a00-7000-8000-%012d", i.n), nil
}

// app returns the app on a database in a temporary directory, with 1 fixed clock
// and 1 fixed id source.
func app(t *testing.T) *Service {
	t.Helper()
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "frostmarks.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	clock := &testClock{at: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)}
	ids := &testIDs{}
	return &Service{DB: db, Now: clock.Now, NewID: ids.New}
}

// local returns the context of the app's own user, which every check passes.
func local(t *testing.T) context.Context {
	t.Helper()
	return authz.WithCaller(t.Context(), authz.LocalCaller())
}

// item returns 1 grant item on 1 folder.
func item(cap, folder string) authz.Item {
	return authz.Item{Cap: cap, Sel: map[string]string{"folder": folder}}
}

// caller returns the context of 1 principal with the grant of the items.
func caller(t *testing.T, items ...authz.Item) context.Context {
	t.Helper()
	grant, err := authz.NewGrant(items...)
	if err != nil {
		t.Fatalf("authz.NewGrant: %v", err)
	}
	return authz.WithCaller(t.Context(), authz.Caller{Principal: "chani", Grant: grant})
}

// readonly returns the context of 1 principal that may read and write nothing.
func readonly(t *testing.T) context.Context {
	t.Helper()
	grant, err := authz.NewGrant()
	if err != nil {
		t.Fatalf("authz.NewGrant: %v", err)
	}
	return authz.WithCaller(t.Context(), authz.Caller{Principal: "chani", Grant: grant})
}

// subscribe subscribes 1 channel to the filters of the bus.
func subscribe(t *testing.T, bus *inproc.Bus, filters ...string) <-chan events.Event {
	t.Helper()
	got := make(chan events.Event, 64)
	for _, filter := range filters {
		cancel, err := bus.Subscribe(filter, func(e events.Event) { got <- e })
		if err != nil {
			t.Fatalf("Subscribe(%q): %v", filter, err)
		}
		t.Cleanup(cancel)
	}
	return got
}

// drain publishes the outbox's rows to the bus.
func drain(t *testing.T, s *Service, bus *inproc.Bus) {
	t.Helper()
	if _, err := outbox.NewRelay(s.DB, bus, outbox.Options{}).Drain(t.Context()); err != nil {
		t.Fatalf("the relay's Drain: %v", err)
	}
}

// collect returns the events that the bus delivered and that no read took.
func collect(t *testing.T, got <-chan events.Event) []events.Event {
	t.Helper()
	var out []events.Event
	for {
		select {
		case event := <-got:
			out = append(out, event)
		default:
			return out
		}
	}
}

// count returns how many rows 1 table of the app's database holds.
func count(t *testing.T, s *Service, table string) int {
	t.Helper()
	var rows int
	if err := s.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&rows); err != nil {
		t.Fatalf("count the rows of %s: %v", table, err)
	}
	return rows
}

// wantCode fails the test unless err is 1 of the app's Connect errors of code.
func wantCode(t *testing.T, err error, code connect.Code) *connect.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("the call passed, and it must fail with %s", code)
	}
	if got := connect.CodeOf(err); got != code {
		t.Fatalf("the error code = %s, want %s: %v", got, code, err)
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("the error %v (%T) is no *connect.Error", err, err)
	}
	return connectErr
}

// wantText fails the test unless err's text is want.
func wantText(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("the call passed, and it must fail with %q", want)
	}
	if err.Error() != want {
		t.Fatalf("the error = %q, want %q", err.Error(), want)
	}
}

// mustCreateFolder makes 1 folder as the local user.
func mustCreateFolder(t *testing.T, s *Service, path string) {
	t.Helper()
	if _, err := s.API().CreateFolder(local(t), &marksv1.CreateFolderRequest{Path: path}); err != nil {
		t.Fatalf("CreateFolder(%q): %v", path, err)
	}
}

// mustCreateBookmark saves 1 bookmark as the local user.
func mustCreateBookmark(t *testing.T, s *Service, req *marksv1.CreateBookmarkRequest) *marksv1.Bookmark {
	t.Helper()
	response, err := s.API().CreateBookmark(local(t), req)
	if err != nil {
		t.Fatalf("CreateBookmark(%q in %q): %v", req.GetUrl(), req.GetFolder(), err)
	}
	return response.GetBookmark()
}

// inprocBus returns the bus of 1 test, and it gives the app its relay's wake, so
// that the app publishes like the daemon publishes.
func inprocBus(t *testing.T, s *Service) *inproc.Bus {
	t.Helper()
	bus := inproc.New()
	s.Notify = outbox.NewRelay(s.DB, bus, outbox.Options{}).Notify
	return bus
}
