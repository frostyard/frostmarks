package e2e

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/internal/importer"
)

// importJSON runs marksctl -json import with the given arguments, and it reads
// the result.
func importJSON(t *testing.T, ctlPath, socket string, args ...string) *importer.Result {
	t.Helper()
	stdout, stderr, code := ctl(t, ctlPath, socket, append([]string{"-json", "import"}, args...)...)
	if code != 0 {
		t.Fatalf("marksctl import %v: exit %d, stderr %q", args, code, stderr)
	}
	result := &importer.Result{}
	if err := json.Unmarshal([]byte(stdout), result); err != nil {
		t.Fatalf("read the result of marksctl import %v: %v: %q", args, err, stdout)
	}
	return result
}

// counts returns the 4 counts of a result.
func counts(result *importer.Result) [4]int {
	return [4]int{result.FoldersMade, result.BookmarksAdded, result.SkippedDuplicates, result.Refused}
}

// TestImport builds the 2 binaries, runs the daemon on a private socket, and
// imports the Firefox and Pinboard fixtures: a dry run that writes nothing, the
// import itself, and a second import that adds nothing.
func TestImport(t *testing.T) {
	root := moduleRoot(t)
	binDir := filepath.Join(t.TempDir(), "bin")
	build(t, root, binDir)
	ctlPath := filepath.Join(binDir, "marksctl")
	socket := filepath.Join(shortDir(t), "s.sock")
	daemon := startDaemon(t, filepath.Join(binDir, "frostmarksd"), filepath.Join(t.TempDir(), "state"), socket)
	firefox := filepath.Join(root, "internal", "netscape", "testdata", "firefox.html")
	pinboard := filepath.Join(root, "internal", "netscape", "testdata", "pinboard.html")

	// The dry run: a plan, and no write.
	dry := importJSON(t, ctlPath, socket, firefox, "-folder", "/imported", "-dry-run")
	if !dry.DryRun || dry.Folder != "/imported" {
		t.Errorf("the dry run = %+v, want a dry run of /imported", dry)
	}
	// The target, and 4 folders of the file; 8 links, 1 of them a duplicate of
	// its folder; and the place: link.
	if got, want := counts(dry), [4]int{5, 7, 1, 1}; got != want {
		t.Errorf("the dry run's counts = %v, want %v", got, want)
	}
	if _, stderr, code := ctl(t, ctlPath, socket, "list-folders", "-folder", "/imported"); code != 1 || !strings.Contains(stderr, "not_found") {
		t.Errorf("list-folders of /imported after the dry run: exit %d, stderr %q, want exit 1 and not_found", code, stderr)
	}
	listed := &marksv1.ListFoldersResponse{}
	ctlJSON(t, ctlPath, socket, listed, "list-folders", "-recursive")
	if len(listed.GetFolders()) != 0 {
		t.Errorf("the dry run made the folders %v", listed.GetFolders())
	}
	books := &marksv1.ListBookmarksResponse{}
	ctlJSON(t, ctlPath, socket, books, "list-bookmarks", "-recursive")
	if len(books.GetBookmarks()) != 0 {
		t.Errorf("the dry run added the bookmarks %v", books.GetBookmarks())
	}

	// The import: the same counts, the folders and the bookmarks.
	first := importJSON(t, ctlPath, socket, firefox, "-folder", "/imported")
	if first.DryRun {
		t.Errorf("the import reports a dry run")
	}
	if counts(first) != counts(dry) {
		t.Errorf("the import's counts = %v, want the dry run's %v", counts(first), counts(dry))
	}
	if want := []string{"/imported", "/imported/Reading", "/imported/Reading/Later", "/imported/Bookmarks Toolbar", "/imported/Other Bookmarks"}; !slices.Equal(first.Folders, want) {
		t.Errorf("the folders = %v, want %v", first.Folders, want)
	}
	for _, added := range first.Added {
		if added.ID == "" {
			t.Errorf("the added bookmark %s has no id", added.URL)
		}
	}
	if len(first.Refusals) != 1 || !strings.HasPrefix(first.Refusals[0].URL, "place:") {
		t.Errorf("the refusals = %+v, want the place: link", first.Refusals)
	}

	ctlJSON(t, ctlPath, socket, listed, "list-folders", "-recursive", "-folder", "/imported")
	if len(listed.GetFolders()) != 4 {
		t.Errorf("list-folders = %v, want the 4 folders of the file below /imported", listed.GetFolders())
	}
	books = &marksv1.ListBookmarksResponse{}
	ctlJSON(t, ctlPath, socket, books, "list-bookmarks", "-recursive", "-folder", "/imported")
	if len(books.GetBookmarks()) != 7 {
		t.Fatalf("list-bookmarks has %d bookmarks, want 7: %v", len(books.GetBookmarks()), books.GetBookmarks())
	}
	byTitle := map[string]*marksv1.Bookmark{}
	for _, bookmark := range books.GetBookmarks() {
		byTitle[bookmark.GetTitle()] = bookmark
	}
	article := byTitle["An article"]
	if article == nil {
		t.Fatalf("the bookmarks hold no \"An article\": %v", books.GetBookmarks())
	}
	if article.GetFolder() != "/imported/Reading" || article.GetUrl() != "https://example.com/article" ||
		article.GetNotes() != "Read this on the train." || !slices.Equal(article.GetTags(), []string{"go", "reading-list"}) {
		t.Errorf("the article = %v, want its folder, URL, notes and tags", article)
	}
	if got := byTitle["A & B"]; got == nil || got.GetUrl() != "https://example.org/a?x=1&y=2" {
		t.Errorf("the bookmark of the entity = %v, want the title A & B", got)
	}
	if got := byTitle["Read later"]; got == nil || got.GetFolder() != "/imported/Reading/Later" || got.GetUrl() != "https://example.net/Later" {
		t.Errorf("the bookmark of the nested folder = %v", got)
	}
	if got := byTitle["A menu link"]; got == nil || got.GetFolder() != "/imported" {
		t.Errorf("the bookmark of the menu = %v, want it in /imported", got)
	}
	if got := byTitle["Example again"]; got != nil {
		t.Errorf("the duplicate was added: %v", got)
	}

	// The second import: nothing is added, and every earlier bookmark is a skip.
	second := importJSON(t, ctlPath, socket, firefox, "-folder", "/imported")
	if got, want := counts(second), [4]int{0, 0, first.BookmarksAdded + first.SkippedDuplicates, first.Refused}; got != want {
		t.Errorf("the second import's counts = %v, want %v", got, want)
	}
	if !slices.Equal(refusalURLs(second), refusalURLs(first)) {
		t.Errorf("the second import's refusals = %v, want %v", refusalURLs(second), refusalURLs(first))
	}

	// The Pinboard file into the root, with the text output.
	stdout, stderr, code := ctl(t, ctlPath, socket, "import", pinboard, "-dry-run")
	if code != 0 {
		t.Fatalf("marksctl import (dry run): exit %d, stderr %q", code, stderr)
	}
	if !strings.HasSuffix(stdout, "dry run: folders made: 0, bookmarks added: 3, skipped as duplicates: 2, refused: 0\n") {
		t.Errorf("the dry run's output = %q, want the counts line with its prefix", stdout)
	}
	stdout, stderr, code = ctl(t, ctlPath, socket, "import", pinboard)
	if code != 0 {
		t.Fatalf("marksctl import: exit %d, stderr %q", code, stderr)
	}
	if want := "folders made: 0, bookmarks added: 3, skipped as duplicates: 2, refused: 0\n"; !strings.HasSuffix(stdout, "\n"+want) {
		t.Errorf("the output = %q, want the counts line %q", stdout, want)
	}
	for _, line := range []string{
		"add\t/\thttps://example.com/go\tGo and SQLite\n",
		"skip\t/\thttps://example.com/go\tThe same page, in capitals\n",
		"skip\t/\thttps://example.net/page\tA page, without its fragment\n",
	} {
		if !strings.Contains(stdout, line) {
			t.Errorf("the output = %q, want the line %q", stdout, line)
		}
	}
	books = &marksv1.ListBookmarksResponse{}
	ctlJSON(t, ctlPath, socket, books, "list-bookmarks")
	if len(books.GetBookmarks()) != 3 {
		t.Errorf("the root holds %d bookmarks, want the 3 of the Pinboard file: %v", len(books.GetBookmarks()), books.GetBookmarks())
	}

	// A file that is no bookmark file exits 1, with nothing on the standard output.
	stdout, stderr, code = ctl(t, ctlPath, socket, "import", filepath.Join(root, "go.mod"))
	if code != 1 || stdout != "" || !strings.Contains(stderr, "is no Netscape bookmark file") {
		t.Errorf("import of go.mod: exit %d, stdout %q, stderr %q; want exit 1 and the error of the file", code, stdout, stderr)
	}

	daemon.signal(t, syscall.SIGTERM)
	if code := daemon.wait(t); code != 0 {
		t.Errorf("the daemon exited with %d, want 0", code)
	}
}

// refusalURLs returns the URLs of the refusals of a result.
func refusalURLs(result *importer.Result) []string {
	var urls []string
	for _, refusal := range result.Refusals {
		urls = append(urls, refusal.URL)
	}
	return urls
}
