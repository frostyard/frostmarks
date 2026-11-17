// Package rpc serves frostmarks' API over Connect on the app's private and lodge
// Unix sockets, and it gives the app's CLI and its MCP server a private client.
//
// Hearth's connectserver provides the handlers, the error mapping, the servers
// and the Unix client. The private socket acts as the local user and ignores
// forged Lodge-* headers. The lodge socket admits calls forwarded by lodge with
// the principal and grant of the caller. This package owns the socket files and
// the app's health route.
package rpc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/frostyard/hearth/connectserver"
	"github.com/frostyard/hearth/lodge"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
)

// The socket's limits and its permissions.
const (
	// dialTimeout is how long the test of an existing socket path waits for a
	// connection.
	dialTimeout = time.Second
	// dirPerm is the permission of the socket's directory.
	dirPerm = 0o700
	// socketPerm is the permission of the socket: the local user only.
	socketPerm = 0o600
)

// Handler returns the HTTP handler of the app's API on the private socket.
func Handler(api marksv1.MarksAPI) http.Handler {
	path, h := marksv1connect.NewMarksHandler(api, connectserver.HandlerOptions()...)
	return connectserver.Private(path, h)
}

// Listen listens on the private socket at path, and it serves only the processes
// of the local user: another user's connection is closed before the server reads
// a byte of it.
//
// It makes the socket's directory (0700) when it is missing. A path that holds a
// socket that accepts a connection is another frostmarksd: the call fails. A path
// that holds a socket that the kernel refuses is stale: it is removed. A path that
// holds something else, or a socket that fails for another reason, fails the call
// and stays. The socket's own permission is 0600.
func Listen(path string) (net.Listener, error) {
	return listen(path, os.Getuid())
}

// listen listens on the private socket at path, and it serves only the processes
// of uid.
//
// The uid is the seam of the tests: the daemon passes its own user's id, so a
// test can pass another id and prove that such a peer is closed with no response.
// The socket's permission is the second guard, and the weaker 1: the socket file
// exists a moment before its permission is set, so the peer check, which the
// kernel makes before the server reads a byte, is what keeps another user out.
func listen(path string, uid int) (net.Listener, error) {
	if dir := parentDir(path); dir != "" {
		if err := makeDir(dir); err != nil {
			return nil, err
		}
	}
	if err := clearPath(path, dialTimeoutSocket); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := chmod(path, socketPerm); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return lodge.Listener(listener, uid), nil
}

// Serve serves h on the listener until ctx ends, and it returns nil after a clean
// stop. A call in flight has the time that hearth's connectserver.Serve gives it
// to finish; a server error closes its connections and returns the error.
func Serve(ctx context.Context, listener net.Listener, h http.Handler) error {
	if err := connectserver.Serve(ctx, listener, h); err != nil {
		return fmt.Errorf("serve the socket: %w", err)
	}
	return nil
}

// NewClient returns the API's client on the private socket.
func NewClient(socket string) marksv1connect.MarksClient {
	return marksv1connect.NewMarksClient(connectserver.UnixClient(socket), "http://frostmarks")
}
