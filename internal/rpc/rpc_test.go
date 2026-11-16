package rpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// serve serves the API on a private socket, and it stops the server when the
// test ends.
func serve(t *testing.T, api marksv1.MarksAPI) string {
	t.Helper()
	socket := filepath.Join(shortDir(t), "s.sock")
	listener, err := Listen(socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
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

// TestMapError checks the table of the interceptor: each error of the app and of
// hearth arrives as its Connect code and its own text.
func TestMapError(t *testing.T) {
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
			text: "permission denied: needs marks:read",
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
			name: "another error",
			err:  errors.New("the disk is full"),
			code: connect.CodeInternal,
			text: "the disk is full",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mapError(test.err)
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
	server := httptest.NewServer(Handler(restricted{MarksAPI: app.API(), grant: grant}))
	t.Cleanup(server.Close)
	client := marksv1connect.NewMarksClient(server.Client(), server.URL)

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
		stale, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatalf("make the stale socket: %v", err)
		}
		if err := stale.Close(); err != nil {
			t.Fatalf("close the stale socket: %v", err)
		}
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
