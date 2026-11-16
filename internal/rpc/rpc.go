// Package rpc serves frostmarks' API over Connect on the app's private Unix
// socket, and it gives the app's CLI and its MCP server a client of that socket.
//
// The socket belongs to the local user: the handler deletes every Lodge-* header
// of a request and sets the local user as the caller (hearth's
// lodge.PrivateHandler), so a forged header reaches no handler. The interceptor
// turns the app's and hearth's errors into Connect codes, so that the CLI and an
// agent read 1 code and 1 text.
package rpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"

	"github.com/frostyard/hearth/authz"
	"github.com/frostyard/hearth/lodge"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
)

// The socket's limits and its permissions.
const (
	// readHeaderTimeout is how long the server waits for a request's headers.
	readHeaderTimeout = 10 * time.Second
	// shutdownTimeout is how long a stop waits for the calls in flight.
	shutdownTimeout = 5 * time.Second
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
	path, handler := marksv1connect.NewMarksHandler(api, connect.WithInterceptors(errorInterceptor{}))
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	return lodge.PrivateHandler(mux)
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

// Serve serves h on the listener until ctx ends, and it returns nil then.
//
// A call in flight has shutdownTimeout to finish. A server that fails before
// then returns its error.
func Serve(ctx context.Context, listener net.Listener, h http.Handler) error {
	server := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	failed := make(chan error, 1)
	go func() {
		failed <- server.Serve(listener)
	}()
	select {
	case err := <-failed:
		return fmt.Errorf("serve the socket: %w", err)
	case <-ctx.Done():
		// The stop is the caller's, not the call's: shutting down needs a
		// context of its own, after the caller's ended.
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("stop the server: %w", err)
		}
		return nil
	}
}

// NewClient returns the API's client on the private socket.
func NewClient(socket string) marksv1connect.MarksClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
	return marksv1connect.NewMarksClient(&http.Client{Transport: transport}, "http://frostmarks")
}

// errorInterceptor maps the errors of the app and of hearth to Connect codes, and
// it keeps the app's own Connect errors as they are.
type errorInterceptor struct{}

// WrapUnary maps the error of 1 unary call.
func (errorInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		response, err := next(ctx, req)
		if err == nil {
			return response, nil
		}
		return nil, mapError(err)
	}
}

// WrapStreamingClient passes 1 streaming client through: the service has no
// streaming RPC.
func (errorInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler passes 1 streaming handler through: the service has no
// streaming RPC.
func (errorInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// mapError returns the Connect error of 1 error of a call:
//
//   - the app's own *connect.Error is kept as it is;
//   - a refused permission, hearth's *authz.DeniedError or authz.ErrNoCaller,
//     is permission_denied with the same text;
//   - an invalid path, hearth's *authz.PathError, is invalid_argument;
//   - a cancelled call is canceled;
//   - every other error is internal.
func mapError(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return err
	}
	var denied *authz.DeniedError
	var pathErr *authz.PathError
	switch {
	case errors.As(err, &denied):
		return connect.NewError(connect.CodePermissionDenied, errors.New(denied.Error()))
	case errors.Is(err, authz.ErrNoCaller):
		return connect.NewError(connect.CodePermissionDenied, errors.New(authz.ErrNoCaller.Error()))
	case errors.As(err, &pathErr):
		return connect.NewError(connect.CodeInvalidArgument, errors.New(pathErr.Error()))
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, errors.New(err.Error()))
	}
	return connect.NewError(connect.CodeInternal, errors.New(err.Error()))
}
