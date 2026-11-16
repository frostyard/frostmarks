// Package e2e tests frostmarks' 2 binaries together: the daemon on its private
// socket, and the CLI with 1 command for each RPC and its stdio MCP server.
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// TestBinaries builds the 2 binaries, runs the daemon on a private socket, and
// calls every command and every MCP tool of the CLI.
func TestBinaries(t *testing.T) {
	root := moduleRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	build(t, root, binDir)
	ctlPath := filepath.Join(binDir, "marksctl")
	socket := filepath.Join(shortDir(t), "s.sock")
	daemon, stopped := startDaemon(t, filepath.Join(binDir, "frostmarksd"), filepath.Join(t.TempDir(), "state"), socket)

	t.Run("the commands", func(t *testing.T) {
		created := createFolder(t, ctlPath, socket, "/work")
		if got := created.GetFolder().GetPath(); got != "/work" {
			t.Errorf("create-folder = %q, want /work", got)
		}
		createFolder(t, ctlPath, socket, "/work/reading")
		folders := listFolders(t, ctlPath, socket, "/work")
		if len(folders.GetFolders()) != 1 || folders.GetFolders()[0].GetPath() != "/work/reading" {
			t.Errorf("list-folders of /work = %v, want the 1 /work/reading", folders.GetFolders())
		}

		saved := createBookmark(t, ctlPath, socket)
		if got := saved.GetFolder(); got != "/work" {
			t.Errorf("the saved bookmark's folder = %q, want /work", got)
		}
		if got := saved.GetTags(); len(got) != 1 || got[0] != "reading" {
			t.Errorf("the saved bookmark's tags = %v, want the 1 reading", got)
		}

		got := getBookmark(t, ctlPath, socket, saved.GetId())
		if got.GetId() != saved.GetId() {
			t.Errorf("get-bookmark = %q, want %q", got.GetId(), saved.GetId())
		}

		updated := updateBookmark(t, ctlPath, socket, saved.GetId())
		if got := updated.GetTitle(); got != "A new title" {
			t.Errorf("update-bookmark's title = %q, want \"A new title\"", got)
		}
		if got := updated.GetTags(); len(got) != 2 || got[0] != "go" || got[1] != "reading" {
			t.Errorf("update-bookmark's tags = %v, want [go reading]", got)
		}

		moved := moveBookmark(t, ctlPath, socket, saved.GetId())
		if got := moved.GetFolder(); got != "/work/reading" {
			t.Errorf("move-bookmark's folder = %q, want /work/reading", got)
		}

		listed := listBookmarks(t, ctlPath, socket)
		if len(listed.GetBookmarks()) != 1 || listed.GetBookmarks()[0].GetId() != saved.GetId() {
			t.Errorf("list-bookmarks = %v, want the 1 bookmark %s", listed.GetBookmarks(), saved.GetId())
		}

		found := searchBookmarks(t, ctlPath, socket)
		if len(found.GetBookmarks()) != 1 || found.GetBookmarks()[0].GetId() != saved.GetId() {
			t.Errorf("search-bookmarks = %v, want the 1 bookmark %s", found.GetBookmarks(), saved.GetId())
		}

		renamed := renameFolder(t, ctlPath, socket, "/work/reading", "/work/read")
		if got := renamed.GetFolder().GetPath(); got != "/work/read" {
			t.Errorf("rename-folder = %q, want /work/read", got)
		}

		// The 2 commands that print nothing, and their JSON form as {}.
		for _, args := range [][]string{
			{"-json", "delete-bookmark", saved.GetId()},
			{"-json", "delete-folder", "/work/read"},
			{"-json", "delete-folder", "/work"},
		} {
			stdout, stderr, code := ctl(t, ctlPath, socket, args...)
			if code != 0 {
				t.Fatalf("marksctl %v: exit %d, stderr %q", args, code, stderr)
			}
			if got := strings.TrimSpace(stdout); got != "{}" {
				t.Errorf("marksctl %v printed %q, want {}", args, got)
			}
		}
	})

	t.Run("the text output and the errors", func(t *testing.T) {
		created := createFolder(t, ctlPath, socket, "/text")
		bookmark := createBookmarkIn(t, ctlPath, socket, "/text")
		if got := created.GetFolder().GetPath(); got != "/text" {
			t.Fatalf("create-folder = %q, want /text", got)
		}

		stdout, stderr, code := ctl(t, ctlPath, socket, "list-bookmarks", "-folder", "/text")
		if code != 0 {
			t.Fatalf("list-bookmarks: exit %d, stderr %q", code, stderr)
		}
		want := bookmark.GetId() + "\t/text\thttps://example.com/text\tA text page\n"
		if stdout != want {
			t.Errorf("the text output = %q, want %q", stdout, want)
		}

		// The same URL in the same folder is already_exists, and it exits 1.
		_, stderr, code = ctl(t, ctlPath, socket, "-json", "create-bookmark", "-url", "https://example.com/text", "-folder", "/text")
		if code != 1 {
			t.Fatalf("the duplicate create-bookmark = %d, want 1 (stderr %q)", code, stderr)
		}
		if !strings.Contains(stderr, "already_exists") {
			t.Errorf("the duplicate's stderr = %q, and it holds no already_exists", stderr)
		}

		// An unknown command is a usage error, and it exits 2.
		stdout, stderr, code = ctl(t, ctlPath, socket, "nonsense")
		if code != 2 {
			t.Fatalf("the unknown command = %d, want 2 (stdout %q, stderr %q)", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "usage: marksctl") {
			t.Errorf("the unknown command's stderr = %q, and it holds no usage", stderr)
		}

		// The MCP server's own checks follow, and they delete what they made.
	})

	t.Run("the MCP tools", func(t *testing.T) {
		callTools(t, ctlPath, socket)
	})

	// The daemon stops on SIGTERM, with exit code 0, and it removes its socket.
	if err := daemon.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal the daemon: %v", err)
	}
	code := waitDaemon(t, daemon)
	stopped()
	if code != 0 {
		t.Errorf("the daemon exited with %d, want 0", code)
	}
	if _, err := os.Stat(socket); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket %s is there after the stop, and the daemon removes it", socket)
	}
}

// callTools calls each MCP tool of the CLI's stdio server 1 time, and 1
// duplicate create_bookmark that fails.
func callTools(t *testing.T, ctlPath, socket string) {
	t.Helper()
	command := exec.Command(ctlPath, "-socket", socket, "mcp")
	client := mcp.NewClient(&mcp.Implementation{Name: "frostmarks-e2e", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect the MCP server: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close the MCP session: %v", err)
		}
	})

	listed, err := session.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	want := []string{
		"create_bookmark", "get_bookmark", "update_bookmark", "move_bookmark", "delete_bookmark",
		"list_bookmarks", "search_bookmarks", "create_folder", "rename_folder", "delete_folder", "list_folders",
	}
	if len(names) != len(want) {
		t.Fatalf("the tools = %v, want the 11 %v", names, want)
	}
	for _, name := range want {
		if !slicesContains(names, name) {
			t.Errorf("the tools hold no %s: %v", name, names)
		}
	}

	// Each tool 1 time with valid arguments.
	call := func(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("CallTool(%s): %v", name, err)
		}
		if result.IsError {
			t.Fatalf("CallTool(%s) = an error: %s", name, resultText(result))
		}
		return result
	}

	call(t, "create_folder", map[string]any{"path": "/mcp"})
	saved := call(t, "create_bookmark", map[string]any{
		"url":    "https://example.com/mcp",
		"title":  "An MCP page",
		"tags":   []any{"mcp"},
		"folder": "/mcp",
	})
	id := structuredID(t, saved)
	call(t, "get_bookmark", map[string]any{"id": id})
	call(t, "update_bookmark", map[string]any{"id": id, "title": "A changed page"})
	call(t, "move_bookmark", map[string]any{"id": id, "folder": "/"})
	call(t, "list_bookmarks", map[string]any{"folder": "/", "recursive": true})
	call(t, "search_bookmarks", map[string]any{"query": "mcp", "folder": "/"})
	call(t, "list_folders", map[string]any{"folder": "/", "recursive": true})

	// The duplicate call fails, and its text names the code.
	duplicate, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "create_bookmark",
		Arguments: map[string]any{"url": "https://example.com/mcp", "folder": "/"},
	})
	if err != nil {
		t.Fatalf("the duplicate CallTool: %v", err)
	}
	if !duplicate.IsError {
		t.Fatalf("the duplicate create_bookmark passed, and it must fail: %s", resultText(duplicate))
	}
	if text := resultText(duplicate); !strings.Contains(text, "already_exists") {
		t.Errorf("the duplicate's text = %q, and it holds no already_exists", text)
	}

	call(t, "rename_folder", map[string]any{"path": "/mcp", "newPath": "/mcp2"})
	call(t, "delete_bookmark", map[string]any{"id": id})
	call(t, "delete_folder", map[string]any{"path": "/mcp2"})
}

// structuredID returns the id of a create_bookmark result.
func structuredID(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var response marksv1.CreateBookmarkResponse
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("read the result: %v", err)
	}
	if err := protojson.Unmarshal(raw, &response); err != nil {
		t.Fatalf("read the bookmark of the result: %v", err)
	}
	id := response.GetBookmark().GetId()
	if id == "" {
		t.Fatalf("the created bookmark has no id: %s", raw)
	}
	return id
}

// resultText returns the text content of 1 tool result.
func resultText(result *mcp.CallToolResult) string {
	var out strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			out.WriteString(text.Text)
		}
	}
	return out.String()
}

// moduleRoot returns the repository's root: the directory of its go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		t.Fatalf("the test runs outside a Go module: GOMOD is %q", gomod)
	}
	return filepath.Dir(gomod)
}

// build builds the 2 binaries into dir.
func build(t *testing.T, root, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("make the binary directory: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", dir+string(filepath.Separator), "./cmd/frostmarksd", "./cmd/marksctl")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
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

// startDaemon starts the daemon, and it returns the process and the function
// that marks the stop as done. It waits for the line that the daemon writes when
// its socket accepts.
func startDaemon(t *testing.T, binary, stateDir, socket string) (*exec.Cmd, func()) {
	t.Helper()
	cmd := exec.Command(binary, "-state-dir", stateDir, "-socket", socket)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("the daemon's stderr: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the daemon: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		if err := cmd.Process.Kill(); err != nil {
			t.Errorf("kill the daemon: %v", err)
		}
	})
	lines := readLines(stderr)
	waitLine(t, lines, "frostmarksd: serving on "+socket)
	return cmd, func() { stopped = true }
}

// waitDaemon waits for the daemon to exit, and it returns its exit code.
func waitDaemon(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait for the daemon: %v", err)
	}
	return exitErr.ExitCode()
}

// readLines reads the lines of a stream, in a goroutine.
func readLines(r io.Reader) <-chan string {
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return lines
}

// waitLine waits for the first line that holds want.
func waitLine(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	var seen []string
	for line := range lines {
		if strings.Contains(line, want) {
			return
		}
		seen = append(seen, line)
	}
	t.Fatalf("the stream ended before the line %q: %v", want, seen)
}

// ctl runs marksctl and returns its standard output, its standard error and its
// exit code.
func ctl(t *testing.T, ctlPath, socket string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(ctlPath, append([]string{"-socket", socket}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run marksctl %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

// ctlJSON runs marksctl with -json and reads its response into response.
func ctlJSON(t *testing.T, ctlPath, socket string, response proto.Message, args ...string) {
	t.Helper()
	stdout, stderr, code := ctl(t, ctlPath, socket, append([]string{"-json"}, args...)...)
	if code != 0 {
		t.Fatalf("marksctl %v: exit %d, stderr %q", args, code, stderr)
	}
	if err := protojson.Unmarshal([]byte(stdout), response); err != nil {
		t.Fatalf("read the response of marksctl %v: %v: %q", args, err, stdout)
	}
}

// createFolder runs create-folder.
func createFolder(t *testing.T, ctlPath, socket, path string) *marksv1.CreateFolderResponse {
	t.Helper()
	response := &marksv1.CreateFolderResponse{}
	ctlJSON(t, ctlPath, socket, response, "create-folder", path)
	return response
}

// createBookmark runs create-bookmark in /work.
func createBookmark(t *testing.T, ctlPath, socket string) *marksv1.Bookmark {
	t.Helper()
	response := &marksv1.CreateBookmarkResponse{}
	ctlJSON(t, ctlPath, socket, response,
		"create-bookmark", "-url", "https://example.com/article#section", "-title", "A page", "-tag", "Reading", "-folder", "/work")
	return response.GetBookmark()
}

// createBookmarkIn runs create-bookmark in 1 folder.
func createBookmarkIn(t *testing.T, ctlPath, socket, folder string) *marksv1.Bookmark {
	t.Helper()
	response := &marksv1.CreateBookmarkResponse{}
	ctlJSON(t, ctlPath, socket, response,
		"create-bookmark", "-url", "https://example.com/text", "-title", "A text page", "-folder", folder)
	return response.GetBookmark()
}

// getBookmark runs get-bookmark.
func getBookmark(t *testing.T, ctlPath, socket, id string) *marksv1.Bookmark {
	t.Helper()
	response := &marksv1.GetBookmarkResponse{}
	ctlJSON(t, ctlPath, socket, response, "get-bookmark", id)
	return response.GetBookmark()
}

// updateBookmark runs update-bookmark with a title and a tag.
func updateBookmark(t *testing.T, ctlPath, socket, id string) *marksv1.Bookmark {
	t.Helper()
	response := &marksv1.UpdateBookmarkResponse{}
	ctlJSON(t, ctlPath, socket, response, "update-bookmark", id, "-title", "A new title", "-add-tag", "go")
	return response.GetBookmark()
}

// moveBookmark runs move-bookmark to /work/reading.
func moveBookmark(t *testing.T, ctlPath, socket, id string) *marksv1.Bookmark {
	t.Helper()
	response := &marksv1.MoveBookmarkResponse{}
	ctlJSON(t, ctlPath, socket, response, "move-bookmark", id, "-folder", "/work/reading")
	return response.GetBookmark()
}

// listBookmarks runs list-bookmarks of /work and its folders, with the tag go.
func listBookmarks(t *testing.T, ctlPath, socket string) *marksv1.ListBookmarksResponse {
	t.Helper()
	response := &marksv1.ListBookmarksResponse{}
	ctlJSON(t, ctlPath, socket, response, "list-bookmarks", "-folder", "/work", "-recursive", "-tag", "go")
	return response
}

// searchBookmarks runs search-bookmarks in /work, with 2 words.
func searchBookmarks(t *testing.T, ctlPath, socket string) *marksv1.SearchBookmarksResponse {
	t.Helper()
	response := &marksv1.SearchBookmarksResponse{}
	ctlJSON(t, ctlPath, socket, response, "search-bookmarks", "-folder", "/work", "example", "article")
	return response
}

// renameFolder runs rename-folder.
func renameFolder(t *testing.T, ctlPath, socket, path, newPath string) *marksv1.RenameFolderResponse {
	t.Helper()
	response := &marksv1.RenameFolderResponse{}
	ctlJSON(t, ctlPath, socket, response, "rename-folder", path, newPath)
	return response
}

// listFolders runs list-folders of 1 folder.
func listFolders(t *testing.T, ctlPath, socket, folder string) *marksv1.ListFoldersResponse {
	t.Helper()
	response := &marksv1.ListFoldersResponse{}
	ctlJSON(t, ctlPath, socket, response, "list-folders", "-folder", folder)
	return response
}

// slicesContains reports whether a list holds 1 value.
func slicesContains(values []string, want string) bool {
	return slices.Contains(values, want)
}
