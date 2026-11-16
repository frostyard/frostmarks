package rpc

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostmarks/marks"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// marksService is the path of the app's Connect service: 1 RPC per method
// below it.
const marksService = "frostyard.frostmarks.v1.Marks/"

// serveLodge serves the app on a lodge socket for the processes of uid, and it
// stops the server when the test ends.
func serveLodge(t *testing.T, api marksv1.MarksAPI, uid int, health func(context.Context) Health) string {
	t.Helper()
	socket := filepath.Join(shortDir(t), "l.sock")
	listener, err := ListenLodge(socket, uid)
	if err != nil {
		t.Fatalf("ListenLodge: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- ServeLodge(ctx, listener, LodgeHandler(api, health))
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("ServeLodge: %v", err)
		}
	})
	return socket
}

// lodgeClient returns the HTTP client of a lodge socket.
func lodgeClient(socket string) *http.Client {
	return &http.Client{Transport: unixTransport(socket)}
}

// grantHeader returns the Lodge-Grant value of 1 grant's JSON: base64url without
// padding.
func grantHeader(t *testing.T, grant string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(grant))
}

// doLodge sends 1 request to the lodge socket and returns its status and its
// body.
func doLodge(t *testing.T, socket, method, path string, headers map[string]string, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, "http://lodge"+path, reader)
	if err != nil {
		t.Fatalf("make the request %s %s: %v", method, path, err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := lodgeClient(socket).Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close the response: %v", err)
		}
	}()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the answer of %s %s: %v", method, path, err)
	}
	return response.StatusCode, string(answer)
}

// createFolderCall posts 1 CreateFolder call as lodge forwards it: the caller
// kind, a principal and the grant's 2 headers.
func createFolderCall(t *testing.T, socket, path, principal, grant string) (int, string) {
	t.Helper()
	headers := map[string]string{callerHeader: principalCaller}
	if principal != "" {
		headers["Lodge-Principal"] = principal
	}
	if grant != "" {
		headers["Lodge-Grant"] = grantHeader(t, grant)
	}
	return doLodge(t, socket, http.MethodPost, connectPrefix+marksService+"CreateFolder",
		headers, `{"path":`+quoted(path)+`}`)
}

// quoted returns 1 text as a JSON string.
func quoted(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `\"`) + `"`
}

// folderCount returns how many folders the store holds, read as the app's own
// user.
func folderCount(t *testing.T, api marksv1.MarksAPI) int {
	t.Helper()
	listed, err := api.ListFolders(local(t), &marksv1.ListFoldersRequest{Folder: "/", Recursive: true})
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	return len(listed.GetFolders())
}

// TestLodgeHealth checks the answer of GET /health: the version and the
// protocol while the store answers, and the reason while it does not.
func TestLodgeHealth(t *testing.T) {
	db, err := marks.Open(t.Context(), filepath.Join(t.TempDir(), "frostmarks.db"))
	if err != nil {
		t.Fatalf("marks.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the database: %v", err)
		}
	})
	service := &marks.Service{DB: db}
	health := func(context.Context) Health {
		return Health{Version: "1.2.3", Protocol: 1, Ping: db.PingContext}
	}
	socket := serveLodge(t, service.API(), os.Getuid(), health)

	t.Run("the app is ok", func(t *testing.T) {
		status, body := doLodge(t, socket, http.MethodGet, healthPath, nil, "")
		if status != http.StatusOK {
			t.Fatalf("GET /health = %d, want 200: %s", status, body)
		}
		if want := `{"ok":true,"version":"1.2.3","protocol":1}` + "\n"; body != want {
			t.Errorf("GET /health = %q, want %q", body, want)
		}
	})

	t.Run("HEAD answers too", func(t *testing.T) {
		status, body := doLodge(t, socket, http.MethodHead, healthPath, nil, "")
		if status != http.StatusOK {
			t.Fatalf("HEAD /health = %d, want 200: %s", status, body)
		}
		if body != "" {
			t.Errorf("HEAD /health answered %q, want no body", body)
		}
	})

	t.Run("another method is 405", func(t *testing.T) {
		status, body := doLodge(t, socket, http.MethodPost, healthPath, nil, "{}")
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("POST /health = %d, want 405: %s", status, body)
		}
	})

	t.Run("a store that does not answer is not ok", func(t *testing.T) {
		if err := db.Close(); err != nil {
			t.Fatalf("close the database: %v", err)
		}
		status, body := doLodge(t, socket, http.MethodGet, healthPath, nil, "")
		if status != http.StatusOK {
			t.Fatalf("GET /health = %d, want 200: %s", status, body)
		}
		if !strings.Contains(body, `"ok":false`) || !strings.Contains(body, `"reason":"`) {
			t.Errorf("GET /health = %q, want an answer that is not ok and says why", body)
		}
	})
}

// TestLodgeConnect checks the Connect face of the lodge socket: the caller kind
// that it admits, the grant that the adapter reads, and the paths that it
// refuses.
func TestLodgeConnect(t *testing.T) {
	app := testApp(t)
	health := func(context.Context) Health { return Health{Version: "dev", Protocol: 1, Ping: app.DB.PingContext} }
	socket := serveLodge(t, app.API(), os.Getuid(), health)

	writeGrant := `[{"cap":"marks:write","sel":{"folder":"/work"}}]`
	readGrant := `[{"cap":"marks:read","sel":{"folder":"/work"}}]`

	t.Run("a grant on the folder creates the folder below it", func(t *testing.T) {
		status, body := createFolderCall(t, socket, "/work", "chani", writeGrant)
		if status != http.StatusOK {
			t.Fatalf("CreateFolder(/work) = %d, want 200: %s", status, body)
		}
		if !strings.Contains(body, `"path":"/work"`) {
			t.Errorf("CreateFolder(/work) = %q, want the folder /work", body)
		}
		status, body = createFolderCall(t, socket, "/work/a", "chani", writeGrant)
		if status != http.StatusOK {
			t.Fatalf("CreateFolder(/work/a) = %d, want 200: %s", status, body)
		}
	})

	t.Run("a grant on another folder is permission_denied", func(t *testing.T) {
		before := folderCount(t, app.API())
		status, body := createFolderCall(t, socket, "/home/a", "chani", writeGrant)
		if status != http.StatusForbidden {
			t.Fatalf("CreateFolder(/home/a) = %d, want 403: %s", status, body)
		}
		if !strings.Contains(body, "permission_denied") {
			t.Errorf("CreateFolder(/home/a) = %q, want permission_denied", body)
		}
		if after := folderCount(t, app.API()); after != before {
			t.Errorf("the store holds %d folders after the refused call, want %d", after, before)
		}
	})

	t.Run("a read grant cannot create", func(t *testing.T) {
		before := folderCount(t, app.API())
		status, body := createFolderCall(t, socket, "/work/b", "chani", readGrant)
		if status != http.StatusForbidden {
			t.Fatalf("CreateFolder(/work/b) with a read grant = %d, want 403: %s", status, body)
		}
		if !strings.Contains(body, "permission_denied") {
			t.Errorf("CreateFolder(/work/b) with a read grant = %q, want permission_denied", body)
		}
		if after := folderCount(t, app.API()); after != before {
			t.Errorf("the store holds %d folders after the refused call, want %d", after, before)
		}
	})

	t.Run("a caller kind that is not principal is 403", func(t *testing.T) {
		for _, kind := range []string{"", "session", "agent"} {
			before := folderCount(t, app.API())
			headers := map[string]string{
				"Lodge-Principal": "chani",
				"Lodge-Grant":     grantHeader(t, writeGrant),
			}
			if kind != "" {
				headers[callerHeader] = kind
			}
			status, body := doLodge(t, socket, http.MethodPost, connectPrefix+marksService+"CreateFolder",
				headers, `{"path":"/work/c"}`)
			if status != http.StatusForbidden {
				t.Fatalf("a call with the kind %q = %d, want 403: %s", kind, status, body)
			}
			if after := folderCount(t, app.API()); after != before {
				t.Errorf("the store holds %d folders after the call with the kind %q, want %d", after, kind, before)
			}
		}
	})

	t.Run("a path outside the contract is 403", func(t *testing.T) {
		for _, path := range []string{"/mcp", "/api", "/x/a", "/connect"} {
			status, body := doLodge(t, socket, http.MethodGet, path, nil, "")
			if status != http.StatusForbidden {
				t.Errorf("GET %s = %d, want 403: %s", path, status, body)
			}
		}
	})

	t.Run("a peer that is not lodge is closed", func(t *testing.T) {
		other := serveLodge(t, app.API(), os.Getuid()+1, health)
		conn, err := net.Dial("unix", other)
		if err != nil {
			t.Fatalf("dial the socket: %v", err)
		}
		t.Cleanup(func() {
			_ = conn.Close()
		})
		// The listener closes the connection before the server reads a byte, so
		// the write may fail, and no byte of a response comes.
		if _, err := conn.Write([]byte("GET /health HTTP/1.1\r\nHost: lodge\r\n\r\n")); err != nil {
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

// TestListenLodge checks the socket's own rules: its directory and its mode, and
// the error of a path that holds something else.
func TestListenLodge(t *testing.T) {
	path := filepath.Join(shortDir(t), "run", "lodge", "apps", "marks.sock")
	listener, err := ListenLodge(path, os.Getuid())
	if err != nil {
		t.Fatalf("ListenLodge: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the socket: %v", err)
	}
	if got := info.Mode().Perm(); got != lodgeSocketPerm {
		t.Errorf("the socket's permission = %#o, want %#o", got, lodgeSocketPerm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat the socket's directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != lodgeDirPerm {
		t.Errorf("the directory's permission = %#o, want %#o", got, lodgeDirPerm)
	}

	if _, err := ListenLodge(path, os.Getuid()); err == nil {
		t.Error("the second ListenLodge passed, and another daemon serves the socket")
	}
}
