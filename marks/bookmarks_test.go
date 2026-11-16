package marks

import (
	"testing"

	"connectrpc.com/connect"

	"github.com/frostyard/hearth/events"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// TestDuplicateURLInFolder checks the unique rule of 1 folder: the same
// canonical URL in the same folder is already_exists with the id of the bookmark
// that holds it, in a create, an update and a move, and the same URL in another
// folder passes.
func TestDuplicateURLInFolder(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/home")

	first := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://Example.COM/a#x",
		Folder: "/work",
	})
	if first.GetUrl() != "https://example.com/a" {
		t.Fatalf("the saved URL = %q, want https://example.com/a", first.GetUrl())
	}

	_, err := app.API().CreateBookmark(local(t), &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/a",
		Folder: "/work",
	})
	connectErr := wantCode(t, err, connect.CodeAlreadyExists)
	want := "the URL https://example.com/a is already bookmarked in /work as " + first.GetId()
	if connectErr.Message() != want {
		t.Errorf("the refusal = %q, want %q", connectErr.Message(), want)
	}

	if _, err := app.API().CreateBookmark(local(t), &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/a",
		Folder: "/home",
	}); err != nil {
		t.Fatalf("the same URL in /home: %v", err)
	}

	other := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/other",
		Folder: "/work",
	})
	url := "https://example.com/a"
	_, err = app.API().UpdateBookmark(local(t), &marksv1.UpdateBookmarkRequest{Id: other.GetId(), Url: &url})
	connectErr = wantCode(t, err, connect.CodeAlreadyExists)
	if connectErr.Message() != want {
		t.Errorf("the update's refusal = %q, want %q", connectErr.Message(), want)
	}

	held := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/held",
		Folder: "/work",
	})
	home := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/held",
		Folder: "/home",
	})
	_, err = app.API().MoveBookmark(local(t), &marksv1.MoveBookmarkRequest{Id: home.GetId(), Folder: "/work"})
	connectErr = wantCode(t, err, connect.CodeAlreadyExists)
	heldWant := "the URL https://example.com/held is already bookmarked in /work as " + held.GetId()
	if connectErr.Message() != heldWant {
		t.Errorf("the move's refusal = %q, want %q", connectErr.Message(), heldWant)
	}
	if got := count(t, app, "bookmarks"); got != 5 {
		t.Errorf("the bookmarks table holds %d rows, want the 5 of the creates", got)
	}
}

// TestSearchBookmarks checks the full-text search: a word of the title, of the
// host part of the URL, of the path of the URL, of the notes and of a tag; 2
// words that must both match; a changed bookmark; a deleted bookmark; and the
// scope of the folder.
func TestSearchBookmarks(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/work/reading")
	mustCreateFolder(t, app, "/home")

	guide := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/storage/page-one",
		Title:  "A Guide To SQLite",
		Notes:  "the engine of the app",
		Tags:   []string{"database"},
		Folder: "/work",
	})
	search := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://docs.example.org/ranking",
		Title:  "Full text",
		Notes:  "rank and order",
		Tags:   []string{"search"},
		Folder: "/work/reading",
	})
	mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.net/other",
		Title:  "Something else",
		Notes:  "nothing to find",
		Tags:   []string{"misc"},
		Folder: "/home",
	})

	find := func(t *testing.T, query, folder string) []*marksv1.Bookmark {
		t.Helper()
		response, err := app.API().SearchBookmarks(local(t), &marksv1.SearchBookmarksRequest{Query: query, Folder: folder})
		if err != nil {
			t.Fatalf("SearchBookmarks(%q in %q): %v", query, folder, err)
		}
		return response.GetBookmarks()
	}
	only := func(t *testing.T, query, folder string, want *marksv1.Bookmark) {
		t.Helper()
		found := find(t, query, folder)
		if len(found) != 1 || found[0].GetId() != want.GetId() {
			t.Fatalf("SearchBookmarks(%q in %q) = %d bookmarks, want the 1 %s", query, folder, len(found), want.GetId())
		}
	}

	t.Run("a title word", func(t *testing.T) { only(t, "guide", "/work", guide) })
	t.Run("a host part", func(t *testing.T) { only(t, "docs", "/work", search) })
	t.Run("a path part", func(t *testing.T) { only(t, "ranking", "/work", search) })
	t.Run("a notes word", func(t *testing.T) { only(t, "engine", "/work", guide) })
	t.Run("a tag", func(t *testing.T) { only(t, "database", "/work", guide) })
	t.Run("the folder scope", func(t *testing.T) {
		if found := find(t, "ranking", "/work/reading"); len(found) != 1 {
			t.Errorf("SearchBookmarks(ranking in /work/reading) = %d bookmarks, want 1", len(found))
		}
		if found := find(t, "engine", "/work/reading"); len(found) != 0 {
			t.Errorf("SearchBookmarks(engine in /work/reading) = %d bookmarks, want none", len(found))
		}
	})

	t.Run("2 words must both match", func(t *testing.T) {
		only(t, "guide engine", "/work", guide)
		if found := find(t, "guide ranking", "/work"); len(found) != 0 {
			t.Errorf("SearchBookmarks(guide ranking) = %d bookmarks, want none", len(found))
		}
	})

	t.Run("a changed bookmark", func(t *testing.T) {
		notes := "a page of words"
		if _, err := app.API().UpdateBookmark(local(t), &marksv1.UpdateBookmarkRequest{Id: guide.GetId(), Notes: &notes}); err != nil {
			t.Fatalf("UpdateBookmark: %v", err)
		}
		if found := find(t, "engine", "/work"); len(found) != 0 {
			t.Errorf("SearchBookmarks(engine) after the change = %d bookmarks, want none", len(found))
		}
		only(t, "words", "/work", guide)
	})

	t.Run("a deleted bookmark", func(t *testing.T) {
		if _, err := app.API().DeleteBookmark(local(t), &marksv1.DeleteBookmarkRequest{Id: search.GetId()}); err != nil {
			t.Fatalf("DeleteBookmark: %v", err)
		}
		if found := find(t, "ranking", "/work"); len(found) != 0 {
			t.Errorf("SearchBookmarks(ranking) after the delete = %d bookmarks, want none", len(found))
		}
		if got := count(t, app, "bookmarks_fts"); got != 2 {
			t.Errorf("the search table holds %d rows, want the 2 of the 2 bookmarks", got)
		}
	})

	t.Run("a query with no word is refused", func(t *testing.T) {
		_, err := app.API().SearchBookmarks(local(t), &marksv1.SearchBookmarksRequest{Query: "   "})
		wantCode(t, err, connect.CodeInvalidArgument)
	})
	t.Run("a folder that is not there", func(t *testing.T) {
		_, err := app.API().SearchBookmarks(local(t), &marksv1.SearchBookmarksRequest{Query: "words", Folder: "/missing"})
		wantCode(t, err, connect.CodeNotFound)
	})
}

// TestBookmarkCreatedEvent checks that 1 created bookmark publishes its created
// event on its folder, with the bookmark's id.
func TestBookmarkCreatedEvent(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	bus := inprocBus(t, app)
	got := subscribe(t, bus, "marks.>")

	bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/a",
		Folder: "/work",
	})
	drain(t, app, bus)
	checkEvents(t, collect(t, got), wantEvent{
		kind:     marksv1.EventBookmarkCreated,
		subject:  bookmark.GetId(),
		selector: "/work",
		author:   "local",
	})
}

// TestBookmarkUpdatedEvent checks that 1 changed bookmark publishes its updated
// event, and that a call that changes nothing publishes no event.
func TestBookmarkUpdatedEvent(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	bus := inprocBus(t, app)
	got := subscribe(t, bus, "marks.>")

	bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/a",
		Title:  "first",
		Folder: "/work",
	})
	drain(t, app, bus)
	collect(t, got)

	title := "second"
	if _, err := app.API().UpdateBookmark(local(t), &marksv1.UpdateBookmarkRequest{Id: bookmark.GetId(), Title: &title}); err != nil {
		t.Fatalf("UpdateBookmark: %v", err)
	}
	drain(t, app, bus)
	checkEvents(t, collect(t, got), wantEvent{
		kind:     marksv1.EventBookmarkUpdated,
		subject:  bookmark.GetId(),
		selector: "/work",
		author:   "local",
	})

	same := "second"
	if _, err := app.API().UpdateBookmark(local(t), &marksv1.UpdateBookmarkRequest{Id: bookmark.GetId(), Title: &same}); err != nil {
		t.Fatalf("the second UpdateBookmark: %v", err)
	}
	if got := count(t, app, "hearth_outbox"); got != 0 {
		t.Errorf("the outbox holds %d rows after a call that changed nothing, want none", got)
	}
}

// TestBookmarkMovedEvents checks that 1 moved bookmark publishes its moved_out
// event on the folder it leaves, and then its moved_in event on the folder it
// arrives in, both with the bookmark's id.
func TestBookmarkMovedEvents(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/home")
	bus := inprocBus(t, app)
	got := subscribe(t, bus, "marks.>")

	bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/a",
		Folder: "/work",
	})
	drain(t, app, bus)
	collect(t, got)

	response, err := app.API().MoveBookmark(local(t), &marksv1.MoveBookmarkRequest{Id: bookmark.GetId(), Folder: "/home"})
	if err != nil {
		t.Fatalf("MoveBookmark: %v", err)
	}
	if got := response.GetBookmark().GetFolder(); got != "/home" {
		t.Errorf("the moved bookmark's folder = %q, want /home", got)
	}
	drain(t, app, bus)
	checkEvents(t, collect(t, got),
		wantEvent{kind: marksv1.EventBookmarkMovedOut, subject: bookmark.GetId(), selector: "/work", author: "local"},
		wantEvent{kind: marksv1.EventBookmarkMovedIn, subject: bookmark.GetId(), selector: "/home", author: "local"},
	)
}

// TestFolderRenameEvents checks that 1 folder's rename publishes the moved_out
// and moved_in events of each bookmark of the folder and of every folder below
// it, with the old and the new path, and none for a bookmark outside it.
func TestFolderRenameEvents(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/work/sub")
	mustCreateFolder(t, app, "/home")
	bus := inprocBus(t, app)
	got := subscribe(t, bus, "marks.>")

	outer := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/1", Folder: "/work"})
	inner := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/2", Folder: "/work/sub"})
	outside := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/3", Folder: "/home"})
	drain(t, app, bus)
	collect(t, got)

	if _, err := app.API().RenameFolder(local(t), &marksv1.RenameFolderRequest{Path: "/work", NewPath: "/archive"}); err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	drain(t, app, bus)
	checkEvents(t, collect(t, got),
		wantEvent{kind: marksv1.EventBookmarkMovedOut, subject: outer.GetId(), selector: "/work", author: "local"},
		wantEvent{kind: marksv1.EventBookmarkMovedIn, subject: outer.GetId(), selector: "/archive", author: "local"},
		wantEvent{kind: marksv1.EventBookmarkMovedOut, subject: inner.GetId(), selector: "/work/sub", author: "local"},
		wantEvent{kind: marksv1.EventBookmarkMovedIn, subject: inner.GetId(), selector: "/archive/sub", author: "local"},
	)

	moved, err := app.API().GetBookmark(local(t), &marksv1.GetBookmarkRequest{Id: inner.GetId()})
	if err != nil {
		t.Fatalf("GetBookmark: %v", err)
	}
	if got := moved.GetBookmark().GetFolder(); got != "/archive/sub" {
		t.Errorf("the bookmark's folder after the rename = %q, want /archive/sub", got)
	}
	untouched, err := app.API().GetBookmark(local(t), &marksv1.GetBookmarkRequest{Id: outside.GetId()})
	if err != nil {
		t.Fatalf("GetBookmark: %v", err)
	}
	if got := untouched.GetBookmark().GetFolder(); got != "/home" {
		t.Errorf("the bookmark outside the rename is in %q, want /home", got)
	}
	if got, want := untouched.GetBookmark().GetUpdateTime().AsTime(), outside.GetUpdateTime().AsTime(); !got.Equal(want) {
		t.Errorf("the bookmark outside the rename has the update time %v, want %v", got, want)
	}
}

// TestBookmarkDeletedEvent checks that 1 deleted bookmark publishes its deleted
// event on the folder it was filed in.
func TestBookmarkDeletedEvent(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/home")
	bus := inprocBus(t, app)
	got := subscribe(t, bus, "marks.>")

	bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
		Url:    "https://example.com/a",
		Folder: "/home",
	})
	drain(t, app, bus)
	collect(t, got)

	if _, err := app.API().DeleteBookmark(local(t), &marksv1.DeleteBookmarkRequest{Id: bookmark.GetId()}); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}
	drain(t, app, bus)
	checkEvents(t, collect(t, got), wantEvent{
		kind:     marksv1.EventBookmarkDeleted,
		subject:  bookmark.GetId(),
		selector: "/home",
		author:   "local",
	})
	if got := count(t, app, "bookmark_tags"); got != 0 {
		t.Errorf("the bookmark_tags table holds %d rows after the delete, want none", got)
	}
	if got := count(t, app, "bookmarks_fts"); got != 0 {
		t.Errorf("the search table holds %d rows after the delete, want none", got)
	}
}

// wantEvent is 1 event that a test expects.
type wantEvent struct {
	// kind is the event's spec.
	kind events.Spec
	// subject is the object's id.
	subject string
	// selector is the object's folder.
	selector string
	// author is the caller's principal.
	author string
}

// checkEvents checks that got holds exactly the events that want names, in
// order, with their type, subject, selector, author and topic.
func checkEvents(t *testing.T, got []events.Event, want ...wantEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d events, want %d: %+v", len(got), len(want), got)
	}
	for i, event := range got {
		expected := want[i]
		if event.Type != expected.kind.Type {
			t.Errorf("the event %d's type = %q, want %q", i, event.Type, expected.kind.Type)
		}
		if event.Subject != expected.subject {
			t.Errorf("the event %d's subject = %q, want %q", i, event.Subject, expected.subject)
		}
		if event.Selector != expected.selector {
			t.Errorf("the event %d's selector = %q, want %q", i, event.Selector, expected.selector)
		}
		if event.Author != expected.author {
			t.Errorf("the event %d's author = %q, want %q", i, event.Author, expected.author)
		}
		topic, _, err := events.TopicOf(expected.kind, expected.selector)
		if err != nil {
			t.Fatalf("events.TopicOf: %v", err)
		}
		if event.Topic != topic {
			t.Errorf("the event %d's topic = %q, want %q", i, event.Topic, topic)
		}
	}
}

// TestRootFolder checks the root folder: a bookmark in it, the list of it, the
// list of it with its folders, and its delete.
func TestRootFolder(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	root := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/root"})
	if got := root.GetFolder(); got != "/" {
		t.Fatalf("the bookmark's folder = %q, want /", got)
	}
	work := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/work", Folder: "/work"})

	listed, err := app.API().ListBookmarks(local(t), &marksv1.ListBookmarksRequest{})
	if err != nil {
		t.Fatalf("ListBookmarks of the root: %v", err)
	}
	if len(listed.GetBookmarks()) != 1 || listed.GetBookmarks()[0].GetId() != root.GetId() {
		t.Errorf("the list of the root = %v, want the 1 bookmark %s", listed.GetBookmarks(), root.GetId())
	}

	all, err := app.API().ListBookmarks(local(t), &marksv1.ListBookmarksRequest{Folder: "/", Recursive: true})
	if err != nil {
		t.Fatalf("ListBookmarks of the root, recursively: %v", err)
	}
	if len(all.GetBookmarks()) != 2 {
		t.Errorf("the recursive list of the root = %d bookmarks, want 2", len(all.GetBookmarks()))
	}

	if _, err := app.API().DeleteBookmark(local(t), &marksv1.DeleteBookmarkRequest{Id: work.GetId()}); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}
	if got := count(t, app, "bookmarks"); got != 1 {
		t.Errorf("the bookmarks table holds %d rows, want the 1 of the root", got)
	}
}
