// Package eventbus publishes the app's events to the broker that lodge embeds
// (lodge ADR-0022, and hearth's events/broker).
//
// The transport dials the broker on the first event, and it dials again after a
// dial that failed, so that frostmarksd starts and serves its API while the
// broker is not there yet: the outbox keeps each event, and the relay's next
// drain publishes it.
package eventbus

import (
	"context"
	"fmt"
	"sync"

	"github.com/frostyard/hearth/events"
	"github.com/frostyard/hearth/events/broker"
)

// Conn is a connection to the broker: hearth's broker transport, which closes
// its connection when the app stops.
type Conn interface {
	// Transport publishes 1 event, and it returns nil only once the broker
	// holds it.
	events.Transport
	// Close closes the connection.
	Close()
}

// Transport is the app's transport to lodge's broker. It dials the broker when
// it has the first event to publish, and not before.
type Transport struct {
	// mu guards conn, so that 2 publishes dial 1 time.
	mu sync.Mutex
	// dial makes the connection to the broker.
	dial func() (Conn, error)
	// conn is the connection, or nil while no publish has succeeded.
	conn Conn
}

// New returns the transport to the broker on the socket of opts.
func New(opts broker.Options) *Transport {
	return newTransport(func() (Conn, error) { return broker.Dial(opts) })
}

// newTransport returns the transport of a dial function, which is the seam of
// the tests.
func newTransport(dial func() (Conn, error)) *Transport {
	return &Transport{dial: dial}
}

// Publish publishes 1 event, and it dials the broker when the transport holds no
// connection.
//
// A dial that fails returns its error, so the relay keeps the event's row and
// tries again at its next drain.
func (t *Transport) Publish(ctx context.Context, e events.Event) error {
	t.mu.Lock()
	if t.conn == nil {
		conn, err := t.dial()
		if err != nil {
			t.mu.Unlock()
			return fmt.Errorf("eventbus: %w", err)
		}
		t.conn = conn
	}
	conn := t.conn
	t.mu.Unlock()
	return conn.Publish(ctx, e)
}

// Close closes the connection to the broker, when the transport holds 1. The
// broker holds every event that Publish acknowledged, so a close loses no event.
func (t *Transport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return
	}
	t.conn.Close()
	t.conn = nil
}
