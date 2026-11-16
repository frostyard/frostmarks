package marks

import (
	"testing"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// TestAuthorizeHooks checks that each hook of an RPC on an id reads the folder
// that the bookmark is stored in: the grant on /work refuses the bookmark of
// /home for a read, a change, a move and a delete, and a move names the folder
// that the caller does not hold.
func TestAuthorizeHooks(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/home")
	secret := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/secret", Folder: "/home"})
	ctx := caller(t, item(readCap, "/work"), item(writeCap, "/work"))

	_, err := app.API().GetBookmark(ctx, &marksv1.GetBookmarkRequest{Id: secret.GetId()})
	wantText(t, err, "permission denied: needs marks:read on folder=/home")

	title := "changed"
	_, err = app.API().UpdateBookmark(ctx, &marksv1.UpdateBookmarkRequest{Id: secret.GetId(), Title: &title})
	wantText(t, err, "permission denied: needs marks:write on folder=/home")

	_, err = app.API().MoveBookmark(ctx, &marksv1.MoveBookmarkRequest{Id: secret.GetId(), Folder: "/work"})
	wantText(t, err, "permission denied: needs marks:write on folder=/home")

	_, err = app.API().DeleteBookmark(ctx, &marksv1.DeleteBookmarkRequest{Id: secret.GetId()})
	wantText(t, err, "permission denied: needs marks:write on folder=/home")

	t.Run("a bookmark that moved out of the grant", func(t *testing.T) {
		mine := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/mine", Folder: "/work"})
		if _, err := app.API().MoveBookmark(local(t), &marksv1.MoveBookmarkRequest{Id: mine.GetId(), Folder: "/home"}); err != nil {
			t.Fatalf("the local user's move: %v", err)
		}
		_, err := app.API().GetBookmark(ctx, &marksv1.GetBookmarkRequest{Id: mine.GetId()})
		wantText(t, err, "permission denied: needs marks:read on folder=/home")
	})

	t.Run("a move names the destination folder", func(t *testing.T) {
		bus := inprocBus(t, app)
		got := subscribe(t, bus, "marks.>")
		drain(t, app, bus)
		collect(t, got)

		mine := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/kept", Folder: "/work"})
		drain(t, app, bus)
		collect(t, got)

		_, err := app.API().MoveBookmark(ctx, &marksv1.MoveBookmarkRequest{Id: mine.GetId(), Folder: "/home"})
		wantText(t, err, "permission denied: needs marks:write on folder=/home")

		if got := count(t, app, "hearth_outbox"); got != 0 {
			t.Errorf("the outbox holds %d rows after the refusal, want none", got)
		}
		kept, err := app.API().GetBookmark(local(t), &marksv1.GetBookmarkRequest{Id: mine.GetId()})
		if err != nil {
			t.Fatalf("GetBookmark: %v", err)
		}
		if got := kept.GetBookmark().GetFolder(); got != "/work" {
			t.Errorf("the bookmark's folder after the refusal = %q, want /work", got)
		}
	})
}

// TestListsFilterByGrant checks that the app's lists and its search keep only the
// rows that the caller's grant covers, and that a list or a search of the root is
// refused.
func TestListsFilterByGrant(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/work/reading")
	mustCreateFolder(t, app, "/home")
	mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://work.example.com/1", Folder: "/work"})
	mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://work.example.com/2", Folder: "/work/reading"})
	mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://home.example.com/1", Folder: "/home"})
	ctx := caller(t, item(readCap, "/work"))

	t.Run("the list of 1 folder and its folders", func(t *testing.T) {
		response, err := app.API().ListBookmarks(ctx, &marksv1.ListBookmarksRequest{Folder: "/work", Recursive: true})
		if err != nil {
			t.Fatalf("ListBookmarks: %v", err)
		}
		if len(response.GetBookmarks()) != 2 {
			t.Fatalf("the list = %d bookmarks, want the 2 of /work and /work/reading", len(response.GetBookmarks()))
		}
		for _, bookmark := range response.GetBookmarks() {
			if bookmark.GetFolder() != "/work" && bookmark.GetFolder() != "/work/reading" {
				t.Errorf("the list holds the bookmark of %s", bookmark.GetFolder())
			}
			if len(bookmark.GetTags()) != 0 {
				t.Errorf("the bookmark %s holds tags, and it has none", bookmark.GetId())
			}
		}
	})

	t.Run("the list of the root is refused", func(t *testing.T) {
		_, err := app.API().ListBookmarks(ctx, &marksv1.ListBookmarksRequest{Folder: "/", Recursive: true})
		wantText(t, err, "permission denied: needs marks:read on folder=/")
		_, err = app.API().SearchBookmarks(ctx, &marksv1.SearchBookmarksRequest{Query: "example", Folder: "/"})
		wantText(t, err, "permission denied: needs marks:read on folder=/")
		_, err = app.API().ListFolders(ctx, &marksv1.ListFoldersRequest{Folder: "/", Recursive: true})
		wantText(t, err, "permission denied: needs marks:read on folder=/")
	})

	t.Run("the list of the folders in 1 folder", func(t *testing.T) {
		response, err := app.API().ListFolders(ctx, &marksv1.ListFoldersRequest{Folder: "/work", Recursive: true})
		if err != nil {
			t.Fatalf("ListFolders: %v", err)
		}
		if len(response.GetFolders()) != 1 || response.GetFolders()[0].GetPath() != "/work/reading" {
			t.Errorf("the folders below /work = %v, want the 1 /work/reading", response.GetFolders())
		}
	})

	// The generated check refuses a list of the root for a caller that holds
	// /work, so the filter of the rows is checked on the app's own call.
	t.Run("the grant filter of the rows", func(t *testing.T) {
		listed, err := app.ListBookmarks(ctx, &marksv1.ListBookmarksRequest{Folder: "/", Recursive: true})
		if err != nil {
			t.Fatalf("ListBookmarks of the root: %v", err)
		}
		if len(listed.GetBookmarks()) != 2 {
			t.Errorf("the list of the root = %d bookmarks, want the 2 of /work", len(listed.GetBookmarks()))
		}
		found, err := app.SearchBookmarks(ctx, &marksv1.SearchBookmarksRequest{Query: "example", Folder: "/"})
		if err != nil {
			t.Fatalf("SearchBookmarks of the root: %v", err)
		}
		if len(found.GetBookmarks()) != 2 {
			t.Errorf("the search of the root = %d bookmarks, want the 2 of /work", len(found.GetBookmarks()))
		}
		folders, err := app.ListFolders(ctx, &marksv1.ListFoldersRequest{Folder: "/", Recursive: true})
		if err != nil {
			t.Fatalf("ListFolders of the root: %v", err)
		}
		if len(folders.GetFolders()) != 2 {
			t.Errorf("the folders of the root = %v, want the 2 of /work", folders.GetFolders())
		}
		for _, folder := range folders.GetFolders() {
			if folder.GetPath() == "/home" {
				t.Error("the folders of the root hold /home, and the grant holds /work")
			}
		}
	})

	// hearth's refusal is no Connect error: internal/rpc maps it to
	// permission_denied, with this text.
	t.Run("a caller without a capability", func(t *testing.T) {
		_, err := app.API().ListBookmarks(readonly(t), &marksv1.ListBookmarksRequest{Folder: "/work"})
		wantText(t, err, "permission denied: needs marks:read on folder=/work")
	})
}
