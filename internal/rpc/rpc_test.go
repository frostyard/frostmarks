package rpc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"connectrpc.com/connect"

	"github.com/frostyard/hearth/authz"

	"github.com/frostyard/frostmarks/marks"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
)

// testApp returns the app on a database in a temporary directory.
func testApp(t *testing.T) *marks.Service {
	t.Helper()
	db, err := marks.Open(t.Context(), filepath.Join(t.TempDir(), "frostmarks.db"))
	if err != nil {
		t.Fatalf("marks.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	return &marks.Service{DB: db}
}

// local returns the context of the app's own user.
func local(t *testing.T) context.Context {
	t.Helper()
	return authz.WithCaller(t.Context(), authz.LocalCaller())
}

// mustCreateFolder makes 1 folder as the local user.
func mustCreateFolder(t *testing.T, app *marks.Service, path string) {
	t.Helper()
	if _, err := app.API().CreateFolder(local(t), &marksv1.CreateFolderRequest{Path: path}); err != nil {
		t.Fatalf("CreateFolder(%q): %v", path, err)
	}
}

// shortDir returns a temporary directory whose path is short enough for a Unix
// socket, and it removes that exact path when the test ends.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fm")
	if err != nil {
		t.Fatalf("make the socket's directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove the socket's directory: %v", err)
		}
	})
	return dir
}

// serve serves the API to the local user on a private socket, and it stops the
// server when the test ends.
func serve(t *testing.T, api marksv1.MarksAPI) string {
	t.Helper()
	return serveOn(t, api, os.Getuid())
}

// serveOn serves the API to the processes of uid on a private socket, and it
// stops the server when the test ends.
func serveOn(t *testing.T, api marksv1.MarksAPI, uid int) string {
	t.Helper()
	socket := filepath.Join(shortDir(t), "s.sock")
	listener, err := listen(socket, uid)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, listener, Handler(api))
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	return socket
}

// closeSocket makes 1 socket file that no listener holds: the listener is closed
// with its unlink turned off, so the path stays and the next dial is refused.
func closeSocket(t *testing.T, path string) {
	t.Helper()
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("make the socket %s: %v", path, err)
	}
	unixListener, ok := listener.(*net.UnixListener)
	if !ok {
		t.Fatalf("the listener of %s is a %T, and not a *net.UnixListener", path, listener)
	}
	unixListener.SetUnlinkOnClose(false)
	if err := unixListener.Close(); err != nil {
		t.Fatalf("close the socket %s: %v", path, err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("the closed socket %s stays: %v", path, err)
	}
}

// unixTransport returns the transport that dials 1 Unix socket.
func unixTransport(socket string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
	}
}

// headerTransport adds headers to every request of 1 client.
type headerTransport struct {
	// base is the transport that sends the request.
	base http.RoundTripper
	// headers are the headers that every request carries.
	headers map[string]string
}

// RoundTrip sends 1 request with the headers of the transport.
func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for name, value := range t.headers {
		clone.Header.Set(name, value)
	}
	return t.base.RoundTrip(clone)
}

// TestPrivateSocketIgnoresForgedHeaders checks that a forged Lodge-Principal and
// Lodge-Grant header on the private socket reaches no handler: the call acts as
// the local user, and it passes.
func TestPrivateSocketIgnoresForgedHeaders(t *testing.T) {
	app := testApp(t)
	mustCreateFolder(t, app, "/work")
	socket := serve(t, app.API())
	client := marksv1connect.NewMarksClient(&http.Client{Transport: headerTransport{
		base: unixTransport(socket),
		headers: map[string]string{
			"Lodge-Principal": "chani",
			"Lodge-Grant":     "not-a-grant",
		},
	}}, "http://frostmarks")

	response, err := client.CreateBookmark(t.Context(), &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/a",
		Folder: "/work",
	})
	if err != nil {
		t.Fatalf("CreateBookmark with a forged header: %v", err)
	}
	if response.GetBookmark().GetId() == "" {
		t.Error("the saved bookmark has no id")
	}
	listed, err := client.ListBookmarks(t.Context(), &marksv1.ListBookmarksRequest{Folder: "/", Recursive: true})
	if err != nil {
		t.Fatalf("ListBookmarks of the root with a forged header: %v", err)
	}
	if len(listed.GetBookmarks()) != 1 {
		t.Errorf("the list of the root = %d bookmarks, want 1", len(listed.GetBookmarks()))
	}
}

// failingAPI returns 1 error from ListBookmarks, without running the app's hooks.
type failingAPI struct {
	// MarksAPI supplies the other RPCs, which this test does not call.
	marksv1.MarksAPI
	// err is the error that ListBookmarks returns.
	err error
}

// ListBookmarks returns the error under test.
func (api failingAPI) ListBookmarks(context.Context, *marksv1.ListBookmarksRequest) (*marksv1.ListBookmarksResponse, error) {
	return nil, api.err
}

// TestHandlerErrors checks the interceptor through the socket and NewClient:
// each error of the app and of hearth arrives as its Connect code and text.
func TestHandlerErrors(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// err is the error of the call.
		err error
		// code is the Connect code of the result.
		code connect.Code
		// text is the text of the result.
		text string
	}{
		{
			name: "the app's own error",
			err:  connect.NewError(connect.CodeNotFound, errors.New("no bookmark abc")),
			code: connect.CodeNotFound,
			text: "no bookmark abc",
		},
		{
			name: "a refused value",
			err:  &authz.DeniedError{Cap: "marks:write", Selector: "folder", Path: "/home"},
			code: connect.CodePermissionDenied,
			text: "permission denied: needs marks:write on folder=/home",
		},
		{
			name: "a wrapped refusal",
			err:  fmt.Errorf("the hook: %w", &authz.DeniedError{Cap: "marks:read"}),
			code: connect.CodePermissionDenied,
			text: "the hook: permission denied: needs marks:read",
		},
		{
			name: "no caller",
			err:  authz.ErrNoCaller,
			code: connect.CodePermissionDenied,
			text: "permission denied: no caller",
		},
		{
			name: "an invalid path",
			err:  &authz.PathError{Selector: "folder", Path: "work", Reason: `it must start with "/"`},
			code: connect.CodeInvalidArgument,
			text: `invalid folder path "work": it must start with "/"`,
		},
		{
			name: "a cancelled call",
			err:  context.Canceled,
			code: connect.CodeCanceled,
			text: context.Canceled.Error(),
		},
		{
			name: "a deadline",
			err:  context.DeadlineExceeded,
			code: connect.CodeDeadlineExceeded,
			text: context.DeadlineExceeded.Error(),
		},
		{
			name: "another error",
			err:  errors.New("the disk is full"),
			code: connect.CodeInternal,
			text: "the disk is full",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			socket := serve(t, failingAPI{err: test.err})
			client := NewClient(socket)
			_, got := client.ListBookmarks(t.Context(), &marksv1.ListBookmarksRequest{})
			if code := connect.CodeOf(got); code != test.code {
				t.Fatalf("the code = %s, want %s: %v", code, test.code, got)
			}
			var connectErr *connect.Error
			if !errors.As(got, &connectErr) {
				t.Fatalf("the error %v (%T) is no *connect.Error", got, got)
			}
			if connectErr.Message() != test.text {
				t.Errorf("the text = %q, want %q", connectErr.Message(), test.text)
			}
		})
	}
}

// restricted is the app's API with 1 restricted grant as the caller of every
// call: it stands in for a caller of the private socket that holds less than the
// local user.
type restricted struct {
	// MarksAPI is the app's API under the test.
	marksv1.MarksAPI
	// grant is the grant of the caller.
	grant authz.Grant
}

// caller returns the context of the restricted caller.
func (r restricted) caller(ctx context.Context) context.Context {
	return authz.WithCaller(ctx, authz.Caller{Principal: "chani", Grant: r.grant})
}

// CreateBookmark calls the app's API as the restricted caller.
func (r restricted) CreateBookmark(ctx context.Context, req *marksv1.CreateBookmarkRequest) (*marksv1.CreateBookmarkResponse, error) {
	return r.MarksAPI.CreateBookmark(r.caller(ctx), req)
}

// ListBookmarks calls the app's API as the restricted caller.
func (r restricted) ListBookmarks(ctx context.Context, req *marksv1.ListBookmarksRequest) (*marksv1.ListBookmarksResponse, error) {
	return r.MarksAPI.ListBookmarks(r.caller(ctx), req)
}

// TestHandlerDenied checks the error mapping through the handler: a refused call
// is permission_denied with hearth's own text, and an invalid path is
// invalid_argument.
func TestHandlerDenied(t *testing.T) {
	app := testApp(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/home")
	grant, err := authz.NewGrant(authz.Item{Cap: "marks:read", Sel: map[string]string{"folder": "/work"}})
	if err != nil {
		t.Fatalf("authz.NewGrant: %v", err)
	}
	socket := serve(t, restricted{MarksAPI: app.API(), grant: grant})
	client := marksv1connect.NewMarksClient(&http.Client{Transport: unixTransport(socket)}, "http://frostmarks")

	_, err = client.ListBookmarks(t.Context(), &marksv1.ListBookmarksRequest{Folder: "/home"})
	if code := connect.CodeOf(err); code != connect.CodePermissionDenied {
		t.Fatalf("the code of the refused list = %s, want permission_denied: %v", code, err)
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("the error %v (%T) is no *connect.Error", err, err)
	}
	if want := "permission denied: needs marks:read on folder=/home"; connectErr.Message() != want {
		t.Errorf("the refusal = %q, want %q", connectErr.Message(), want)
	}

	_, err = client.CreateBookmark(t.Context(), &marksv1.CreateBookmarkRequest{Url: "https://example.com/a", Folder: "work"})
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("the code of the invalid folder = %s, want invalid_argument: %v", code, err)
	}
}

// TestListen checks the socket's rules: a live socket is another daemon, a stale
// socket is replaced, and another kind of file is a refusal. The socket's
// directory is made, and the socket belongs to the local user only.
func TestListen(t *testing.T) {
	t.Run("the directory is made and the socket belongs to the user", func(t *testing.T) {
		dir := shortDir(t)
		socket := filepath.Join(dir, "state", "s.sock")
		listener, err := Listen(socket)
		if err != nil {
			t.Fatalf("Listen: %v", err)
		}
		t.Cleanup(func() {
			_ = listener.Close()
		})
		info, err := os.Stat(socket)
		if err != nil {
			t.Fatalf("stat the socket: %v", err)
		}
		if got := info.Mode().Perm(); got != socketPerm {
			t.Errorf("the socket's permission = %#o, want %#o", got, socketPerm)
		}
		dirInfo, err := os.Stat(filepath.Dir(socket))
		if err != nil {
			t.Fatalf("stat the socket's directory: %v", err)
		}
		if got := dirInfo.Mode().Perm(); got != dirPerm {
			t.Errorf("the directory's permission = %#o, want %#o", got, dirPerm)
		}
	})

	t.Run("a live socket is another daemon", func(t *testing.T) {
		socket := filepath.Join(shortDir(t), "s.sock")
		listener, err := Listen(socket)
		if err != nil {
			t.Fatalf("Listen: %v", err)
		}
		t.Cleanup(func() {
			_ = listener.Close()
		})
		_, err = Listen(socket)
		if err == nil {
			t.Fatal("the second Listen passed, and another daemon serves the socket")
		}
		if want := "another frostmarksd serves " + socket; err.Error() != want {
			t.Errorf("the second Listen = %q, want %q", err.Error(), want)
		}
	})

	t.Run("a stale socket is replaced", func(t *testing.T) {
		socket := filepath.Join(shortDir(t), "s.sock")
		closeSocket(t, socket)
		listener, err := Listen(socket)
		if err != nil {
			t.Fatalf("Listen over the stale socket: %v", err)
		}
		t.Cleanup(func() {
			_ = listener.Close()
		})
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatalf("dial the new socket: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Errorf("close the connection: %v", err)
		}
	})

	t.Run("another kind of file is a refusal", func(t *testing.T) {
		path := filepath.Join(shortDir(t), "s.sock")
		if err := os.WriteFile(path, []byte("not a socket\n"), 0o600); err != nil {
			t.Fatalf("write the file: %v", err)
		}
		if _, err := Listen(path); err == nil {
			t.Fatalf("Listen over a regular file passed, and it must fail")
		}
	})
}

// TestNewClient checks that the client reaches the app over the socket, with the
// local user as the caller, and that a refused call carries the code and the text
// of the app's error.
func TestNewClient(t *testing.T) {
	app := testApp(t)
	mustCreateFolder(t, app, "/work")
	socket := serve(t, app.API())
	client := NewClient(socket)

	saved, err := client.CreateBookmark(t.Context(), &marksv1.CreateBookmarkRequest{Url: "https://example.com/a", Folder: "/work"})
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}
	if got := saved.GetBookmark().GetFolder(); got != "/work" {
		t.Errorf("the bookmark's folder = %q, want /work", got)
	}

	_, err = client.CreateBookmark(t.Context(), &marksv1.CreateBookmarkRequest{Url: "https://example.com/a", Folder: "/work"})
	if code := connect.CodeOf(err); code != connect.CodeAlreadyExists {
		t.Fatalf("the code of the duplicate = %s, want already_exists: %v", code, err)
	}

	_, err = client.GetBookmark(t.Context(), &marksv1.GetBookmarkRequest{Id: "no-such-id"})
	if code := connect.CodeOf(err); code != connect.CodeNotFound {
		t.Fatalf("the code of the missing bookmark = %s, want not_found: %v", code, err)
	}
}

// TestListenPeerUID checks that a socket serves only the daemon's own user: the
// listener closes every other peer before the server reads a byte, so such a peer
// gets no response at all. The socket's permission is the second guard, and it
// comes a moment after the socket exists.
func TestListenPeerUID(t *testing.T) {
	app := testApp(t)
	mustCreateFolder(t, app, "/work")

	t.Run("the daemon's own user is served", func(t *testing.T) {
		socket := serve(t, app.API())
		client := NewClient(socket)
		listed, err := client.ListBookmarks(t.Context(), &marksv1.ListBookmarksRequest{Folder: "/work"})
		if err != nil {
			t.Fatalf("ListBookmarks: %v", err)
		}
		if len(listed.GetBookmarks()) != 0 {
			t.Errorf("the list = %d bookmarks, want none", len(listed.GetBookmarks()))
		}
	})

	t.Run("another user is closed", func(t *testing.T) {
		socket := serveOn(t, app.API(), os.Getuid()+1)
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatalf("dial the socket: %v", err)
		}
		t.Cleanup(func() {
			_ = conn.Close()
		})
		// The peer writes 1 request, and it reads: the listener closes the
		// connection, so the write may fail, and no byte of a response comes.
		if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: frostmarks\r\n\r\n")); err != nil {
			t.Logf("the write of the refused peer: %v", err)
		}
		buffer := make([]byte, 512)
		n, err := conn.Read(buffer)
		if n != 0 {
			t.Fatalf("the refused peer read %d bytes, want none: %q", n, buffer[:n])
		}
		if err == nil {
			t.Fatal("the refused peer's read passed, want an error")
		}
	})
}

// TestClearPathDialErrors checks the rule of a socket path that holds a socket:
// only the kernel's refusal means that nothing listens there, so only a refusal
// removes the path.
func TestClearPathDialErrors(t *testing.T) {
	refused := func(string) (net.Conn, error) {
		return nil, fmt.Errorf("dial unix: %w", syscall.ECONNREFUSED)
	}
	slow := func(string) (net.Conn, error) {
		return nil, fmt.Errorf("dial unix: %w", os.ErrDeadlineExceeded)
	}
	forbidden := func(string) (net.Conn, error) {
		return nil, fmt.Errorf("dial unix: %w", fs.ErrPermission)
	}

	t.Run("a refusal removes the stale socket", func(t *testing.T) {
		path := filepath.Join(shortDir(t), "s.sock")
		closeSocket(t, path)
		if err := clearPath(path, refused); err != nil {
			t.Fatalf("clearPath: %v", err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the stale socket %s is there, and the refusal removes it", path)
		}
	})

	kept := []struct {
		// name names the case.
		name string
		// dial is the dial that fails.
		dial dialSocket
		// err is the error that the dial wraps.
		err error
	}{
		{name: "a timeout keeps the socket", dial: slow, err: os.ErrDeadlineExceeded},
		{name: "a permission error keeps the socket", dial: forbidden, err: fs.ErrPermission},
	}
	for _, test := range kept {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(shortDir(t), "s.sock")
			closeSocket(t, path)
			err := clearPath(path, test.dial)
			if err == nil {
				t.Fatalf("clearPath passed, and it must fail")
			}
			if !errors.Is(err, test.err) {
				t.Errorf("the error = %v, want the %v of the dial", err, test.err)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Errorf("the socket %s is gone, and a failed dial keeps it: %v", path, err)
			}
		})
	}

	t.Run("a live socket is another daemon", func(t *testing.T) {
		path := filepath.Join(shortDir(t), "s.sock")
		closeSocket(t, path)
		conn, peer := net.Pipe()
		t.Cleanup(func() {
			_ = conn.Close()
			_ = peer.Close()
		})
		err := clearPath(path, func(string) (net.Conn, error) { return conn, nil })
		if err == nil {
			t.Fatalf("clearPath passed, and another daemon serves the socket")
		}
		if want := "another frostmarksd serves " + path; err.Error() != want {
			t.Errorf("the error = %q, want %q", err.Error(), want)
		}
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("the socket %s is gone, and a live socket stays: %v", path, err)
		}
	})
}
