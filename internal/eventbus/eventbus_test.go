package eventbus

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/frostyard/hearth/events"
)

// fakeConn is 1 connection of a test: how many events it published, and whether
// it is closed.
type fakeConn struct {
	// published is how many events Publish took.
	published int
	// closed is set by Close.
	closed bool
	// err is the error of Publish.
	err error
}

// Publish records 1 event.
func (c *fakeConn) Publish(_ context.Context, _ events.Event) error {
	c.published++
	return c.err
}

// Close records the close.
func (c *fakeConn) Close() { c.closed = true }

// counter counts the dials of a test, and it gives 1 connection or 1 error.
type counter struct {
	// mu guards the fields.
	mu sync.Mutex
	// dials is how many dials the transport made.
	dials int
	// fail is how many of the first dials fail.
	fail int
	// conn is the connection of a successful dial.
	conn *fakeConn
}

// dial is the transport's dial function.
func (c *counter) dial() (Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dials++
	if c.dials <= c.fail {
		return nil, errors.New("the broker's socket is not there")
	}
	return c.conn, nil
}

// dialsMade returns how many dials the transport made.
func (c *counter) dialsMade() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dials
}

// TestTransportDialsOnPublish checks the transport's 2 rules: a dial that fails
// returns its error and is tried again at the next publish, and a dial that
// succeeds happens 1 time only.
func TestTransportDialsOnPublish(t *testing.T) {
	event := events.Event{ID: "019a8b6e-6a00-7000-8000-000000000001", Topic: "marks.bookmark.created.folder"}

	t.Run("a failed dial is tried again", func(t *testing.T) {
		counts := &counter{fail: 1, conn: &fakeConn{}}
		transport := newTransport(counts.dial)

		if err := transport.Publish(t.Context(), event); err == nil {
			t.Fatal("the publish over a failed dial passed, and it must fail")
		}
		if got := counts.dialsMade(); got != 1 {
			t.Fatalf("the transport dialed %d times, want 1", got)
		}
		if err := transport.Publish(t.Context(), event); err != nil {
			t.Fatalf("the publish after the failed dial: %v", err)
		}
		if got := counts.dialsMade(); got != 2 {
			t.Errorf("the transport dialed %d times, want 2: a failed dial is tried again", got)
		}
		if got := counts.conn.published; got != 1 {
			t.Errorf("the connection published %d events, want 1", got)
		}
	})

	t.Run("a successful dial happens 1 time", func(t *testing.T) {
		counts := &counter{conn: &fakeConn{}}
		transport := newTransport(counts.dial)
		for range 3 {
			if err := transport.Publish(t.Context(), event); err != nil {
				t.Fatalf("the publish: %v", err)
			}
		}
		if got := counts.dialsMade(); got != 1 {
			t.Errorf("the transport dialed %d times, want 1", got)
		}
		if got := counts.conn.published; got != 3 {
			t.Errorf("the connection published %d events, want 3", got)
		}
	})

	t.Run("a closed transport dials again", func(t *testing.T) {
		counts := &counter{conn: &fakeConn{}}
		transport := newTransport(counts.dial)
		if err := transport.Publish(t.Context(), event); err != nil {
			t.Fatalf("the publish: %v", err)
		}
		transport.Close()
		if !counts.conn.closed {
			t.Error("Close left the connection open")
		}
		// A close before any publish is harmless, and the next publish dials.
		transport.Close()
		if err := transport.Publish(t.Context(), event); err != nil {
			t.Fatalf("the publish after the close: %v", err)
		}
		if got := counts.dialsMade(); got != 2 {
			t.Errorf("the transport dialed %d times, want 2", got)
		}
	})
}
