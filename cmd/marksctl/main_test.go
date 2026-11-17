package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostmarks/internal/importer"
	"github.com/frostyard/frostmarks/internal/netscape"
)

// noEnv is the environment of a CLI that has no variable: no socket and no state
// directory.
func noEnv(string) string {
	return ""
}

// TestUsageErrors checks that each usage error prints the usage and exits 2.
func TestUsageErrors(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// args are the CLI's arguments.
		args []string
	}{
		{name: "no command", args: nil},
		{name: "an unknown command", args: []string{"nonsense"}},
		{name: "no URL", args: []string{"create-bookmark"}},
		{name: "no id", args: []string{"get-bookmark"}},
		{name: "an extra argument", args: []string{"get-bookmark", "a", "b"}},
		{name: "a missing argument", args: []string{"rename-folder", "/a"}},
		{name: "a missing folder", args: []string{"move-bookmark", "an-id"}},
		{name: "a bad flag", args: []string{"list-bookmarks", "-bogus"}},
		{name: "no query", args: []string{"search-bookmarks"}},
		{name: "import with no file", args: []string{"import"}},
		{name: "import with 2 files", args: []string{"import", "a.html", "b.html"}},
		{name: "import with a bad flag", args: []string{"import", "a.html", "-bogus"}},
		{name: "an argument of mcp", args: []string{"mcp", "extra"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(test.args, strings.NewReader(""), &stdout, &stderr, noEnv)
			if code != exitUsage {
				t.Fatalf("run(%v) = %d, want %d (stderr %q)", test.args, code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), "usage: marksctl") {
				t.Errorf("the stderr = %q, and it holds no usage", stderr.String())
			}
		})
	}
}

// TestHelp checks that help exits 0 with the usage, and that it needs no
// environment and no socket.
func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help"}, strings.NewReader(""), &stdout, &stderr, noEnv); code != exitOK {
		t.Fatalf("run(help) = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stderr.String(), "usage: marksctl") {
		t.Errorf("the stderr = %q, and it holds no usage", stderr.String())
	}
}

// TestNoSocket checks that a command without a socket path prints the error of
// the path and exits 1.
func TestNoSocket(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"list-bookmarks"}, strings.NewReader(""), &stdout, &stderr, noEnv); code != exitError {
		t.Fatalf("run(list-bookmarks) = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "no socket path") {
		t.Errorf("the stderr = %q, and it holds no socket error", stderr.String())
	}
}

// TestImportFileErrors checks that a file that the import cannot use stops it
// with exit 1 before any call: a missing file, a file that is no Netscape
// bookmark file, and a file that is too large.
func TestImportFileErrors(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.html")
	if err := os.WriteFile(plain, []byte("<html><body>a page</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(dir, "large.html")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(netscape.MaxFileSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		// name names the case.
		name string
		// path is the file.
		path string
		// want is the text of the error.
		want string
	}{
		{name: "a missing file", path: filepath.Join(dir, "missing.html"), want: "missing.html: no such file"},
		{name: "no Netscape file", path: plain, want: "the file " + plain + " is no Netscape bookmark file"},
		{name: "a large file", path: large, want: "the file " + large + " is larger than 33554432 bytes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			// A socket is set, and nothing listens on it: the error is the file's.
			args := []string{"-socket", filepath.Join(dir, "none.sock"), "import", test.path}
			if code := run(args, strings.NewReader(""), &stdout, &stderr, noEnv); code != exitError {
				t.Fatalf("run(%v) = %d, want %d (stderr %q)", args, code, exitError, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("the stdout = %q, want nothing", stdout.String())
			}
			if !strings.HasPrefix(stderr.String(), "marksctl: ") || !strings.Contains(stderr.String(), test.want) {
				t.Errorf("the stderr = %q, want %q after marksctl:", stderr.String(), test.want)
			}
		})
	}
}

// TestImportNoSocket checks that a good file with no socket path exits 1 with the
// error of the path.
func TestImportNoSocket(t *testing.T) {
	file := filepath.Join(t.TempDir(), "marks.html")
	if err := os.WriteFile(file, []byte("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n<DL></DL>"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"import", file}, strings.NewReader(""), &stdout, &stderr, noEnv); code != exitError {
		t.Fatalf("run(import) = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "no socket path") {
		t.Errorf("the stderr = %q, and it holds no socket error", stderr.String())
	}
}

// TestPrintImport checks the text output of an import: a line for each item, with
// each control character as a space, and the counts line with its dry-run prefix.
func TestPrintImport(t *testing.T) {
	result := &importer.Result{
		Folder:            "/imp",
		FoldersMade:       1,
		BookmarksAdded:    1,
		SkippedDuplicates: 1,
		Refused:           2,
		Folders:           []string{"/imp/A"},
		Added:             []importer.Added{{ID: "id", Folder: "/imp/A", URL: "https://example.com/", Title: "a\tb"}},
		Skipped:           []importer.Skipped{{Folder: "/imp/A", URL: "https://example.com/", Title: "again"}},
		Refusals: []importer.Refusal{
			{Folder: "/imp", URL: "javascript:x", Title: "X", Reason: "must start\nwith http"},
			{Folder: "/imp/B", Reason: "the folder is too long"},
		},
	}
	want := "folder\t/imp/A\n" +
		"add\t/imp/A\thttps://example.com/\ta b\n" +
		"skip\t/imp/A\thttps://example.com/\tagain\n" +
		"refuse\t/imp\tjavascript:x\tmust start with http\n" +
		"refuse\t/imp/B\t\tthe folder is too long\n" +
		"folders made: 1, bookmarks added: 1, skipped as duplicates: 1, refused: 2\n"
	var out bytes.Buffer
	if err := printImport(&out, result); err != nil {
		t.Fatal(err)
	}
	if out.String() != want {
		t.Errorf("the output =\n%q\nwant\n%q", out.String(), want)
	}
	result.DryRun = true
	out.Reset()
	if err := printImport(&out, result); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "\ndry run: folders made: 1, bookmarks added: 1, skipped as duplicates: 1, refused: 2\n") {
		t.Errorf("the dry run's output = %q, want the counts line with the prefix", out.String())
	}
}
