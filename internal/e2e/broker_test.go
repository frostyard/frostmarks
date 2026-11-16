// This file is the test broker of the lodge test: an embedded NATS server with
// JetStream, its Unix socket, and the hub's connection, which makes the app's
// stream and reads it (lodge ADR-0022, and docs/specs/lodge-app.md).
package e2e

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// The broker's shape in the tests: the app's stream, and the limits that lodge
// gives it.
const (
	// readyTimeout bounds the wait for the server's readiness.
	readyTimeout = 10 * time.Second
	// streamName is the app's stream: app_<id>.
	streamName = "app_marks"
	// streamSubject is what the app may publish: <id>.>.
	streamSubject = "marks.>"
	// streamMaxAge is how long the stream keeps a message.
	streamMaxAge = 7 * 24 * time.Hour
	// streamMaxBytes is how large the stream grows before it drops the oldest
	// message.
	streamMaxBytes = 1 << 30
	// duplicateWindow is how long the stream remembers a message's id.
	duplicateWindow = 2 * time.Minute
)

// broker is 1 test broker: its server, its Unix socket, and the hub's connection.
type broker struct {
	// socket is the path of the app's Unix socket.
	socket string
	// srv is the server.
	srv *server.Server
	// js is the hub's JetStream client: the stream's maker and its reader.
	js jetstream.JetStream
}

// startBroker starts the broker, and it serves the app on socket: every
// connection of the socket is spliced to a connection of the server itself.
func startBroker(t *testing.T, socket string) *broker {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		t.Fatalf("make the broker's directory: %v", err)
	}
	srv, err := server.NewServer(&server.Options{
		ServerName: "frostmarks-e2e",
		// The app reaches the server on a Unix socket whose connections are
		// spliced to connections of the server itself.
		DontListen: true,
		JetStream:  true,
		StoreDir:   t.TempDir(),
		NoSigs:     true,
	})
	if err != nil {
		t.Fatalf("make the broker: %v", err)
	}
	srv.SetLoggerV2(discardLogger{}, false, false, false)
	srv.Start()
	t.Cleanup(srv.Shutdown)
	if !srv.ReadyForConnections(readyTimeout) {
		t.Fatal("the broker is not ready")
	}
	b := &broker{socket: socket, srv: srv}

	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen on the broker's socket: %v", err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close the broker's socket: %v", err)
		}
	})
	go b.accept(listener)

	hub, err := nats.Connect("", nats.InProcessServer(srv), nats.Name("frostmarks-e2e-hub"), nats.Timeout(readyTimeout))
	if err != nil {
		t.Fatalf("connect the hub: %v", err)
	}
	t.Cleanup(hub.Close)
	b.js, err = jetstream.New(hub)
	if err != nil {
		t.Fatalf("connect the hub to JetStream: %v", err)
	}
	if _, err := b.js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:       streamName,
		Subjects:   []string{streamSubject},
		MaxAge:     streamMaxAge,
		MaxBytes:   streamMaxBytes,
		Discard:    jetstream.DiscardOld,
		Storage:    jetstream.FileStorage,
		Duplicates: duplicateWindow,
	}); err != nil {
		t.Fatalf("make the app's stream: %v", err)
	}
	return b
}

// accept splices each connection of the socket to a connection of the server
// itself: the server has no listener of its own.
func (b *broker) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		serverConn, err := b.srv.InProcessConn()
		if err != nil {
			_ = conn.Close()
			continue
		}
		go splice(conn, serverConn)
	}
}

// splice copies each direction of 1 connection until 1 of them ends, and then
// closes both.
func splice(app, broker net.Conn) {
	go func() {
		_, _ = io.Copy(app, broker)
		_ = app.Close()
		_ = broker.Close()
	}()
	_, _ = io.Copy(broker, app)
	_ = app.Close()
	_ = broker.Close()
}

// count returns how many messages the app's stream holds.
func (b *broker) count(t *testing.T) uint64 {
	t.Helper()
	stream, err := b.js.Stream(t.Context(), streamName)
	if err != nil {
		t.Fatalf("read the app's stream: %v", err)
	}
	info, err := stream.Info(t.Context())
	if err != nil {
		t.Fatalf("read the app's stream: %v", err)
	}
	return info.State.Msgs
}

// discardLogger is the server's log, which a test does not read.
type discardLogger struct{}

// Noticef discards 1 notice.
func (discardLogger) Noticef(string, ...any) {}

// Warnf discards 1 warning.
func (discardLogger) Warnf(string, ...any) {}

// Errorf discards 1 error.
func (discardLogger) Errorf(string, ...any) {}

// Fatalf discards 1 fatal error.
func (discardLogger) Fatalf(string, ...any) {}

// Debugf discards 1 debug line.
func (discardLogger) Debugf(string, ...any) {}

// Tracef discards 1 trace line.
func (discardLogger) Tracef(string, ...any) {}
