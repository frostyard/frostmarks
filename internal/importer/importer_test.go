package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
	"github.com/frostyard/frostmarks/internal/netscape"
)

// fakeClient is an in-memory daemon of the 4 RPCs that the import calls. It
// embeds the client interface, so that a call of another RPC panics, and it
// records each call that it answers.
type fakeClient struct {
	marksv1connect.MarksClient
	// folders holds the stored folders; the root is never stored.
	folders map[string]bool
	// bookmarks are the stored bookmarks.
	bookmarks []*marksv1.Bookmark
	// calls are the calls in order, as "Name argument".
	calls []string
	// bookmarkErr gives the error of a CreateBookmark call by the URL, or nil.
	bookmarkErr map[string]error
	// folderErr gives the error of a CreateFolder call by the path, or nil.
	folderErr map[string]error
	// raced holds the folders that another client makes after the import's read:
	// ListFolders does not show them, and CreateFolder makes them and answers
	// already_exists.
	raced map[string]bool
	// listErr is the error of every ListFolders call of the path it names.
	listErr map[string]error
}

// newFake returns a fake daemon that holds the given folders.
func newFake(folders ...string) *fakeClient {
	f := &fakeClient{folders: map[string]bool{}, bookmarkErr: map[string]error{}, folderErr: map[string]error{}, listErr: map[string]error{}, raced: map[string]bool{}}
	for _, folder := range folders {
		f.folders[folder] = true
	}
	return f
}

// exists reports whether a folder is there: the root always is.
func (f *fakeClient) exists(path string) bool {
	return path == "/" || f.folders[path]
}

// below reports whether a path is below a folder.
func below(path, folder string) bool {
	return path != folder && (folder == "/" || strings.HasPrefix(path, folder+"/"))
}

// ListFolders lists the folders below 1 folder.
func (f *fakeClient) ListFolders(_ context.Context, req *marksv1.ListFoldersRequest) (*marksv1.ListFoldersResponse, error) {
	f.calls = append(f.calls, fmt.Sprintf("ListFolders %s recursive=%t", req.GetFolder(), req.GetRecursive()))
	if err := f.listErr[req.GetFolder()]; err != nil {
		return nil, err
	}
	if !f.exists(req.GetFolder()) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no folder %s", req.GetFolder()))
	}
	response := &marksv1.ListFoldersResponse{}
	var paths []string
	for path := range f.folders {
		if below(path, req.GetFolder()) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		response.Folders = append(response.Folders, &marksv1.Folder{Path: path})
	}
	return response, nil
}

// ListBookmarks lists the bookmarks of 1 folder and the folders below it.
func (f *fakeClient) ListBookmarks(_ context.Context, req *marksv1.ListBookmarksRequest) (*marksv1.ListBookmarksResponse, error) {
	f.calls = append(f.calls, fmt.Sprintf("ListBookmarks %s recursive=%t", req.GetFolder(), req.GetRecursive()))
	if !f.exists(req.GetFolder()) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no folder %s", req.GetFolder()))
	}
	response := &marksv1.ListBookmarksResponse{}
	for _, bookmark := range f.bookmarks {
		if bookmark.GetFolder() == req.GetFolder() || below(bookmark.GetFolder(), req.GetFolder()) {
			response.Bookmarks = append(response.Bookmarks, bookmark)
		}
	}
	return response, nil
}

// CreateFolder makes 1 folder.
func (f *fakeClient) CreateFolder(_ context.Context, req *marksv1.CreateFolderRequest) (*marksv1.CreateFolderResponse, error) {
	f.calls = append(f.calls, "CreateFolder "+req.GetPath())
	if err := f.folderErr[req.GetPath()]; err != nil {
		return nil, err
	}
	if f.raced[req.GetPath()] {
		f.folders[req.GetPath()] = true
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("the folder %s exists", req.GetPath()))
	}
	if !f.exists(parentOf(req.GetPath())) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no folder %s", parentOf(req.GetPath())))
	}
	if f.folders[req.GetPath()] {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("the folder %s exists", req.GetPath()))
	}
	f.folders[req.GetPath()] = true
	return &marksv1.CreateFolderResponse{Folder: &marksv1.Folder{Path: req.GetPath()}}, nil
}

// CreateBookmark adds 1 bookmark.
func (f *fakeClient) CreateBookmark(_ context.Context, req *marksv1.CreateBookmarkRequest) (*marksv1.CreateBookmarkResponse, error) {
	f.calls = append(f.calls, "CreateBookmark "+req.GetFolder()+" "+req.GetUrl())
	if err := f.bookmarkErr[req.GetUrl()]; err != nil {
		return nil, err
	}
	if !f.exists(req.GetFolder()) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no folder %s", req.GetFolder()))
	}
	for _, stored := range f.bookmarks {
		if stored.GetFolder() == req.GetFolder() && stored.GetUrl() == req.GetUrl() {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("the URL is already bookmarked"))
		}
	}
	bookmark := &marksv1.Bookmark{
		Id: fmt.Sprintf("id-%d", len(f.bookmarks)+1), Url: req.GetUrl(), Title: req.GetTitle(),
		Notes: req.GetNotes(), Tags: req.GetTags(), Folder: req.GetFolder(),
	}
	f.bookmarks = append(f.bookmarks, bookmark)
	return &marksv1.CreateBookmarkResponse{Bookmark: bookmark}, nil
}

// writes returns the calls that write.
func (f *fakeClient) writes() []string {
	var out []string
	for _, call := range f.calls {
		if strings.HasPrefix(call, "Create") {
			out = append(out, call)
		}
	}
	return out
}

// link returns 1 bookmark of a document, in the folder at 1 index (-1 is the
// target folder).
func link(folder int, url, title string) netscape.Bookmark {
	return netscape.Bookmark{Folder: folder, URL: url, Title: title, Notes: "n " + title, Tags: []string{"t"}}
}

// sample is a document of 2 folders, 3 bookmarks, 1 duplicate in the file and 1
// refusal of the parser.
func sample() *netscape.Document {
	return &netscape.Document{
		Folders: []netscape.Folder{{Name: "Reading", Parent: -1}, {Name: "Later", Parent: 0}},
		Bookmarks: []netscape.Bookmark{
			link(0, "https://example.com/a", "A"),
			link(1, "https://example.com/b", "B"),
			link(-1, "https://example.com/c", "C"),
			link(0, "https://example.com/a", "A again"),
		},
		Refusals: []netscape.Refusal{
			{Folder: 0, URL: "javascript:x", Title: "X", Reason: `the URL "javascript:x" must start with http:// or https://`},
		},
	}
}

// run runs the import, and it fails the test on an error.
func run(t *testing.T, f *fakeClient, doc *netscape.Document, target string, dryRun bool) *Result {
	t.Helper()
	result, err := Run(t.Context(), f, doc, target, dryRun)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return result
}

// TestDryRun checks that a dry run calls only the 2 reads, and that its plan is
// the plan of the real import.
func TestDryRun(t *testing.T) {
	f := newFake("/imp")
	dry := run(t, f, sample(), "/imp", true)
	if got := f.writes(); len(got) != 0 {
		t.Errorf("the dry run wrote: %v", got)
	}
	if want := []string{"ListFolders /imp recursive=true", "ListBookmarks /imp recursive=true"}; !slices.Equal(f.calls, want) {
		t.Errorf("the calls = %v, want %v", f.calls, want)
	}
	if !dry.DryRun || dry.Folder != "/imp" {
		t.Errorf("the result = %+v, want a dry run of /imp", dry)
	}
	if want := []string{"/imp/Reading", "/imp/Reading/Later"}; !slices.Equal(dry.Folders, want) {
		t.Errorf("the folders = %v, want %v", dry.Folders, want)
	}
	for _, added := range dry.Added {
		if added.ID != "" {
			t.Errorf("the dry run gives the id %q to %s", added.ID, added.URL)
		}
	}

	real := run(t, newFake("/imp"), sample(), "/imp", false)
	if dry.FoldersMade != real.FoldersMade || dry.BookmarksAdded != real.BookmarksAdded ||
		dry.SkippedDuplicates != real.SkippedDuplicates || dry.Refused != real.Refused {
		t.Errorf("the dry run's counts %d/%d/%d/%d differ from the import's %d/%d/%d/%d",
			dry.FoldersMade, dry.BookmarksAdded, dry.SkippedDuplicates, dry.Refused,
			real.FoldersMade, real.BookmarksAdded, real.SkippedDuplicates, real.Refused)
	}
}

// TestApply checks the writes of a real import: the folders first, parents first,
// then the bookmarks in the order of the file, and the result.
func TestApply(t *testing.T) {
	f := newFake("/imp")
	result := run(t, f, sample(), "/imp", false)
	wantWrites := []string{
		"CreateFolder /imp/Reading",
		"CreateFolder /imp/Reading/Later",
		"CreateBookmark /imp/Reading https://example.com/a",
		"CreateBookmark /imp/Reading/Later https://example.com/b",
		"CreateBookmark /imp https://example.com/c",
	}
	if got := f.writes(); !slices.Equal(got, wantWrites) {
		t.Errorf("the writes = %v, want %v", got, wantWrites)
	}
	if result.DryRun || result.FoldersMade != 2 || result.BookmarksAdded != 3 || result.SkippedDuplicates != 1 || result.Refused != 1 {
		t.Errorf("the result = %+v, want 2 folders, 3 added, 1 skipped, 1 refused", result)
	}
	wantAdded := []Added{
		{ID: "id-1", Folder: "/imp/Reading", URL: "https://example.com/a", Title: "A"},
		{ID: "id-2", Folder: "/imp/Reading/Later", URL: "https://example.com/b", Title: "B"},
		{ID: "id-3", Folder: "/imp", URL: "https://example.com/c", Title: "C"},
	}
	if !reflect.DeepEqual(result.Added, wantAdded) {
		t.Errorf("the added = %+v, want %+v", result.Added, wantAdded)
	}
	wantSkipped := []Skipped{{Folder: "/imp/Reading", URL: "https://example.com/a", Title: "A again"}}
	if !reflect.DeepEqual(result.Skipped, wantSkipped) {
		t.Errorf("the skipped = %+v, want %+v", result.Skipped, wantSkipped)
	}
	wantRefusals := []Refusal{{Folder: "/imp/Reading", URL: "javascript:x", Title: "X", Reason: `the URL "javascript:x" must start with http:// or https://`}}
	if !reflect.DeepEqual(result.Refusals, wantRefusals) {
		t.Errorf("the refusals = %+v, want %+v", result.Refusals, wantRefusals)
	}
	stored := f.bookmarks[0]
	if stored.GetNotes() != "n A" || !slices.Equal(stored.GetTags(), []string{"t"}) {
		t.Errorf("the stored bookmark = %+v, want its notes and tags", stored)
	}

	// The second import adds nothing: every bookmark is a duplicate.
	again := run(t, f, sample(), "/imp", false)
	if again.FoldersMade != 0 || again.BookmarksAdded != 0 || again.SkippedDuplicates != 4 || again.Refused != 1 {
		t.Errorf("the second import = %d/%d/%d/%d, want 0 folders, 0 added, 4 skipped, 1 refused",
			again.FoldersMade, again.BookmarksAdded, again.SkippedDuplicates, again.Refused)
	}
}

// TestMissingTarget checks the target that is not there, and its missing parents.
func TestMissingTarget(t *testing.T) {
	f := newFake("/a")
	doc := &netscape.Document{
		Folders:   []netscape.Folder{{Name: "X", Parent: -1}},
		Bookmarks: []netscape.Bookmark{link(0, "https://example.com/x", "X")},
	}
	dry := run(t, f, doc, "/a/b/c", true)
	wantCalls := []string{"ListFolders /a/b/c recursive=true", "ListFolders /a/b recursive=false", "ListFolders /a recursive=false"}
	if !slices.Equal(f.calls, wantCalls) {
		t.Errorf("the calls = %v, want %v (no ListBookmarks of a missing target)", f.calls, wantCalls)
	}
	want := []string{"/a/b", "/a/b/c", "/a/b/c/X"}
	if !slices.Equal(dry.Folders, want) {
		t.Errorf("the folders = %v, want %v", dry.Folders, want)
	}

	f = newFake("/a")
	result := run(t, f, doc, "/a/b/c", false)
	wantWrites := []string{
		"CreateFolder /a/b", "CreateFolder /a/b/c", "CreateFolder /a/b/c/X",
		"CreateBookmark /a/b/c/X https://example.com/x",
	}
	if got := f.writes(); !slices.Equal(got, wantWrites) {
		t.Errorf("the writes = %v, want %v", got, wantWrites)
	}
	if result.FoldersMade != 3 || result.BookmarksAdded != 1 {
		t.Errorf("the result = %+v, want 3 folders and 1 added", result)
	}

	// A target that is not there and whose parents are all missing: the walk
	// stops at the root.
	f = newFake()
	if got := run(t, f, &netscape.Document{}, "/p/q", true).Folders; !slices.Equal(got, []string{"/p", "/p/q"}) {
		t.Errorf("the folders = %v, want [/p /p/q]", got)
	}
}

// TestExistingFolders checks that a folder that is there is not made and not
// counted.
func TestExistingFolders(t *testing.T) {
	f := newFake("/imp", "/imp/Reading")
	result := run(t, f, sample(), "/imp", false)
	if want := []string{"/imp/Reading/Later"}; !slices.Equal(result.Folders, want) {
		t.Errorf("the folders = %v, want %v", result.Folders, want)
	}
	if got := f.writes(); slices.Contains(got, "CreateFolder /imp/Reading") {
		t.Errorf("the import made the folder that is there: %v", got)
	}
}

// TestDuplicates checks a duplicate of the store and a duplicate of the file.
func TestDuplicates(t *testing.T) {
	f := newFake("/imp", "/imp/Reading")
	f.bookmarks = []*marksv1.Bookmark{
		{Id: "old-1", Url: "https://example.com/a", Folder: "/imp/Reading"},
		// The same URL in another folder is no duplicate.
		{Id: "old-2", Url: "https://example.com/c", Folder: "/imp/Other"},
	}
	doc := sample()
	// A bookmark of the file in the root of the target, a repeat in the file.
	doc.Bookmarks = append(doc.Bookmarks, link(-1, "https://example.com/c", "C again"))
	result := run(t, f, doc, "/imp", true)
	wantSkipped := []string{"A", "A again", "C again"}
	var got []string
	for _, skipped := range result.Skipped {
		got = append(got, skipped.Title)
	}
	if !slices.Equal(got, wantSkipped) {
		t.Errorf("the skipped = %v, want %v", got, wantSkipped)
	}
	if result.BookmarksAdded != 2 {
		t.Errorf("the added = %+v, want 2 (B and C)", result.Added)
	}
}

// TestCreateBookmarkErrors checks the answers of the daemon to a write: already_exists
// is a duplicate, invalid_argument is a refusal, and any other error stops.
func TestCreateBookmarkErrors(t *testing.T) {
	doc := &netscape.Document{Bookmarks: []netscape.Bookmark{
		link(-1, "https://example.com/1", "One"),
		link(-1, "https://example.com/2", "Two"),
		link(-1, "https://example.com/3", "Three"),
	}}

	f := newFake()
	f.bookmarkErr["https://example.com/1"] = connect.NewError(connect.CodeAlreadyExists, errors.New("the URL is already bookmarked"))
	f.bookmarkErr["https://example.com/2"] = connect.NewError(connect.CodeInvalidArgument, errors.New("the title is 3000 bytes, and the limit is 2048"))
	result := run(t, f, doc, "", false)
	if result.BookmarksAdded != 1 || result.SkippedDuplicates != 1 || result.Refused != 1 {
		t.Fatalf("the result = %+v, want 1 added, 1 skipped, 1 refused", result)
	}
	if got, want := result.Skipped[0].Title, "One"; got != want {
		t.Errorf("the skipped title = %q, want %q", got, want)
	}
	wantRefusal := Refusal{Folder: "/", URL: "https://example.com/2", Title: "Two", Reason: "the title is 3000 bytes, and the limit is 2048"}
	if result.Refusals[0] != wantRefusal {
		t.Errorf("the refusal = %+v, want %+v", result.Refusals[0], wantRefusal)
	}

	f = newFake()
	f.bookmarkErr["https://example.com/2"] = connect.NewError(connect.CodeInternal, errors.New("the database failed"))
	result, err := Run(t.Context(), f, doc, "/", false)
	if err == nil || connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("Run = %v, %v; want the internal error", result, err)
	}
	if result != nil {
		t.Errorf("Run returned the result %+v with an error", result)
	}
	if got := f.writes(); len(got) != 2 {
		t.Errorf("the writes = %v, want 2 (the import stops at the error)", got)
	}
}

// TestCallErrors checks that an error of the other calls stops the import.
func TestCallErrors(t *testing.T) {
	denied := connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	tests := []struct {
		// name names the case.
		name string
		// target is the target folder.
		target string
		// set sets the failure.
		set func(*fakeClient)
	}{
		{name: "the list of the target", target: "/imp", set: func(f *fakeClient) { f.listErr["/imp"] = denied }},
		{name: "the list of a parent", target: "/x/y", set: func(f *fakeClient) { f.listErr["/x"] = denied }},
		{name: "the make of a folder", target: "/imp", set: func(f *fakeClient) { f.folderErr["/imp/Reading"] = denied }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFake("/imp")
			test.set(f)
			result, err := Run(t.Context(), f, sample(), test.target, false)
			if err == nil || result != nil {
				t.Errorf("Run = %v, %v; want an error and no result", result, err)
			}
		})
	}

}

// TestFolderRace checks a folder that another client made after the read: the
// list does not show it, and CreateFolder answers already_exists. The import goes
// on, adds the bookmarks of the folder, and counts no folder made for it.
func TestFolderRace(t *testing.T) {
	f := newFake("/imp")
	f.raced["/imp/Reading"] = true
	result := run(t, f, sample(), "/imp", false)
	if !slices.Contains(f.calls, "CreateFolder /imp/Reading") {
		t.Errorf("the calls = %v, want the call CreateFolder /imp/Reading", f.calls)
	}
	if want := []string{"/imp/Reading/Later"}; !slices.Equal(result.Folders, want) {
		t.Errorf("the folders = %v, want %v", result.Folders, want)
	}
	if result.FoldersMade != 1 {
		t.Errorf("the folders made = %d, want 1 (the raced folder is not counted)", result.FoldersMade)
	}
	if result.BookmarksAdded != 3 {
		t.Errorf("the added = %+v, want the 3 bookmarks, with the 1 of the raced folder", result.Added)
	}
	if !slices.Contains(f.calls, "CreateBookmark /imp/Reading https://example.com/a") {
		t.Errorf("the calls = %v, want the bookmark of the raced folder", f.calls)
	}
}

// TestRefusedFolder checks that a folder that breaks a rule is refused with its
// bookmarks and its subfolders, and that no folder is made for it.
func TestRefusedFolder(t *testing.T) {
	long := strings.Repeat("a", 1100)
	// 400 characters of 2 bytes are a short path, and a topic that is far too long.
	wide := strings.Repeat("é", 400)
	doc := &netscape.Document{
		Folders: []netscape.Folder{{Name: long, Parent: -1}, {Name: "Sub", Parent: 0}, {Name: "Fine", Parent: -1}, {Name: wide, Parent: -1}},
		Bookmarks: []netscape.Bookmark{
			link(0, "https://example.com/1", "One"),
			link(1, "https://example.com/2", "Two"),
			link(2, "https://example.com/3", "Three"),
			link(3, "https://example.com/4", "Four"),
		},
		Refusals: []netscape.Refusal{{Folder: 0, URL: "javascript:x", Title: "X", Reason: "bad"}},
	}
	f := newFake()
	result := run(t, f, doc, "/", false)
	for _, call := range f.writes() {
		if strings.Contains(call, long) || strings.Contains(call, wide) {
			t.Errorf("the import wrote for a refused folder: %.80s", call)
		}
	}
	if want := []string{"/Fine"}; !slices.Equal(result.Folders, want) {
		t.Errorf("the folders = %v, want %v", result.Folders, want)
	}
	if result.BookmarksAdded != 1 || result.Added[0].Title != "Three" {
		t.Errorf("the added = %+v, want only Three", result.Added)
	}
	// 2 refused folders, then the parser's refusal, then 3 bookmarks.
	if result.Refused != 6 {
		t.Fatalf("the refusals = %d, want 6: %+v", result.Refused, result.Refusals)
	}
	first, second := result.Refusals[0], result.Refusals[1]
	if first.Folder != "/"+long || first.URL != "" || first.Title != "" ||
		first.Reason != fmt.Sprintf("the folder %q is 1101 bytes, and the limit is 1024", "/"+long) {
		t.Errorf("the refusal of the long folder = %.200q", fmt.Sprintf("%+v", first))
	}
	if second.Folder != "/"+wide || second.URL != "" || !strings.Contains(second.Reason, "topic") {
		t.Errorf("the refusal of the wide folder = %.200q", fmt.Sprintf("%+v", second))
	}
	if got := result.Refusals[2]; got.URL != "javascript:x" || got.Reason != "bad" || got.Folder != "/"+long {
		t.Errorf("the parser's refusal = %.200q", fmt.Sprintf("%+v", got))
	}
	for i, want := range []struct{ url, refusedBy string }{
		{"https://example.com/1", "/" + long},
		{"https://example.com/2", "/" + long},
		{"https://example.com/4", "/" + wide},
	} {
		got := result.Refusals[3+i]
		if got.URL != want.url || got.Reason != fmt.Sprintf("the folder %s is refused", want.refusedBy) {
			t.Errorf("the refusal of %s = %.200q", want.url, fmt.Sprintf("%+v", got))
		}
	}
	// A folder below a refused folder has no path of its own: its bookmark's
	// refusal names the refused folder.
	if got := result.Refusals[4].Folder; got != "/"+long {
		t.Errorf("the folder of the bookmark below the refused folder = %.60q, want the path of the refused folder", got)
	}
}

// TestRefusedParentManyChildren checks a parent whose name is too long, with many
// children, each with 1 bookmark: 1 refusal of the folder, 1 refusal for each
// bookmark with the refused folder's path, and no write.
func TestRefusedParentManyChildren(t *testing.T) {
	const children = 1000
	name := strings.Repeat("a", netscape.MaxNameLen)
	doc := &netscape.Document{Folders: []netscape.Folder{{Name: name, Parent: -1}}}
	for i := range children {
		doc.Folders = append(doc.Folders, netscape.Folder{Name: fmt.Sprintf("child %d", i), Parent: 0})
		doc.Bookmarks = append(doc.Bookmarks, link(i+1, fmt.Sprintf("https://example.com/%d", i), "B"))
	}
	f := newFake()
	result := run(t, f, doc, "/", false)
	if got := f.writes(); len(got) != 0 {
		t.Errorf("the import wrote for a refused folder: %d calls, the first %.80s", len(got), got[0])
	}
	if result.Refused != 1+children || result.FoldersMade != 0 || result.BookmarksAdded != 0 {
		t.Fatalf("the counts = %d refused, %d folders, %d added; want %d, 0, 0", result.Refused, result.FoldersMade, result.BookmarksAdded, 1+children)
	}
	parent := "/" + name
	if got := result.Refusals[0]; got.Folder != parent || got.URL != "" ||
		got.Reason != fmt.Sprintf("the folder %q is %d bytes, and the limit is 1024", parent, len(parent)) {
		t.Errorf("the refusal of the folder = %.200q", fmt.Sprintf("%+v", got))
	}
	wantReason := "the folder " + parent + " is refused"
	for i, got := range result.Refusals[1:] {
		if got.Folder != parent || got.Reason != wantReason || got.URL != fmt.Sprintf("https://example.com/%d", i) {
			t.Fatalf("the refusal %d = %.200q", i, fmt.Sprintf("%+v", got))
		}
	}
}

// TestTargets checks the canonical form of the target, and the targets that are
// refused.
func TestTargets(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// target is the -folder value.
		target string
		// want is the canonical target.
		want string
		// invalid is true for a target that is an error.
		invalid bool
	}{
		{name: "empty", target: "", want: "/"},
		{name: "the root", target: "/", want: "/"},
		{name: "a folder", target: "/a/b", want: "/a/b"},
		{name: "a composed form", target: "/e\u0301", want: "/\u00e9"},
		{name: "relative", target: "a", invalid: true},
		{name: "a trailing slash", target: "/a/", invalid: true},
		{name: "a dot segment", target: "/a/../b", invalid: true},
		{name: "a control character", target: "/a\tb", invalid: true},
		{name: "too long", target: "/" + strings.Repeat("a", 1100), invalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFake()
			for _, folder := range []string{"/a", "/a/b", "/\u00e9"} {
				f.folders[folder] = true
			}
			result, err := Run(t.Context(), f, &netscape.Document{}, test.target, true)
			if test.invalid {
				if err == nil {
					t.Fatalf("Run(%q) passed, want an error", test.target)
				}
				if len(f.calls) != 0 {
					t.Errorf("an invalid target made the calls %v", f.calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run(%q): %v", test.target, err)
			}
			if result.Folder != test.want {
				t.Errorf("the folder = %q, want %q", result.Folder, test.want)
			}
		})
	}
}

// TestCanonicalFolders checks that 2 names that differ only in their Unicode form
// are 1 folder.
func TestCanonicalFolders(t *testing.T) {
	doc := &netscape.Document{
		Folders: []netscape.Folder{{Name: "\u00e9", Parent: -1}, {Name: "e\u0301", Parent: -1}},
		Bookmarks: []netscape.Bookmark{
			link(0, "https://example.com/1", "One"),
			link(1, "https://example.com/1", "Two"),
		},
	}
	result := run(t, newFake(), doc, "/", true)
	if want := []string{"/\u00e9"}; !slices.Equal(result.Folders, want) {
		t.Errorf("the folders = %q, want %q", result.Folders, want)
	}
	if result.BookmarksAdded != 1 || result.SkippedDuplicates != 1 {
		t.Errorf("the result = %+v, want 1 added and 1 skipped", result)
	}
}

// TestJSON checks the JSON form of a result: the keys of docs/specs/cli.md, each
// empty list as [], and the id only in an added bookmark of a real import.
func TestJSON(t *testing.T) {
	wantKeys := []string{"added", "bookmarksAdded", "dryRun", "folder", "folders", "foldersMade", "refusals", "refused", "skipped", "skippedDuplicates"}

	empty := run(t, newFake(), &netscape.Document{}, "", true)
	data, err := json.MarshalIndent(empty, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if !slices.Equal(keys, wantKeys) {
		t.Errorf("the keys = %v, want %v", keys, wantKeys)
	}
	for _, key := range []string{"folders", "added", "skipped", "refusals"} {
		if list, ok := decoded[key].([]any); !ok || len(list) != 0 {
			t.Errorf("%s = %#v, want []", key, decoded[key])
		}
	}
	if !strings.Contains(string(data), `"folders": []`) {
		t.Errorf("the JSON = %s, want an empty list as []", data)
	}
	if decoded["dryRun"] != true || decoded["folder"] != "/" {
		t.Errorf("the JSON = %s, want dryRun true and folder /", data)
	}

	doc := sample()
	for _, dryRun := range []bool{true, false} {
		result := run(t, newFake("/imp"), doc, "/imp", dryRun)
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var raw struct {
			Added    []map[string]any `json:"added"`
			Skipped  []map[string]any `json:"skipped"`
			Refusals []map[string]any `json:"refusals"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		for _, added := range raw.Added {
			_, hasID := added["id"]
			if hasID == dryRun {
				t.Errorf("dryRun %t: the added bookmark %v has the id: %t", dryRun, added, hasID)
			}
			if len(added) != map[bool]int{true: 3, false: 4}[dryRun] {
				t.Errorf("dryRun %t: the added bookmark has the keys %v", dryRun, added)
			}
		}
		if len(raw.Skipped) != 1 || len(raw.Skipped[0]) != 3 {
			t.Errorf("the skipped = %v, want 1 item with 3 keys (folder, url, title)", raw.Skipped)
		}
		if len(raw.Refusals) != 1 || len(raw.Refusals[0]) != 4 {
			t.Errorf("the refusals = %v, want 1 item with 4 keys (folder, url, title, reason)", raw.Refusals)
		}
	}
}
