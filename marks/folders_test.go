package marks

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/frostyard/hearth/authz"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// TestCreateFolderRules checks that 1 folder needs its parent, that it is not
// made twice, and that the root is never made.
func TestCreateFolderRules(t *testing.T) {
	app := app(t)

	_, err := app.API().CreateFolder(local(t), &marksv1.CreateFolderRequest{Path: "/"})
	wantCode(t, err, connect.CodeInvalidArgument)

	_, err = app.API().CreateFolder(local(t), &marksv1.CreateFolderRequest{Path: "/missing/child"})
	connectErr := wantCode(t, err, connect.CodeNotFound)
	if connectErr.Message() != "no folder /missing" {
		t.Errorf("the refusal = %q, want \"no folder /missing\"", connectErr.Message())
	}

	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/work/reading")

	_, err = app.API().CreateFolder(local(t), &marksv1.CreateFolderRequest{Path: "/work"})
	connectErr = wantCode(t, err, connect.CodeAlreadyExists)
	if connectErr.Message() != "the folder /work exists" {
		t.Errorf("the refusal = %q, want \"the folder /work exists\"", connectErr.Message())
	}

	// A path that is no path is refused by the generated check of the RPC, with
	// hearth's *authz.PathError; internal/rpc maps it to invalid_argument.
	_, err = app.API().CreateFolder(local(t), &marksv1.CreateFolderRequest{Path: "work"})
	var pathErr *authz.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("the error of the path work = %v (%T), want *authz.PathError", err, err)
	}

	if got := count(t, app, "folders"); got != 2 {
		t.Errorf("the folders table holds %d rows, want the 2 of /work and /work/reading", got)
	}
}

// TestRenameFolderRules checks the rules of 1 folder's rename: neither path is
// the root, the new path is not the folder's path and not below it, the folder
// exists, the new path's parent exists, and nothing is at the new path.
func TestRenameFolderRules(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/work/reading")
	mustCreateFolder(t, app, "/home")
	mustCreateFolder(t, app, "/archive")
	mustCreateFolder(t, app, "/archive/old")

	rename := func(t *testing.T, path, newPath string) error {
		t.Helper()
		_, err := app.API().RenameFolder(local(t), &marksv1.RenameFolderRequest{Path: path, NewPath: newPath})
		return err
	}

	tests := []struct {
		// name names the case.
		name string
		// path is the folder to rename.
		path string
		// newPath is its new path.
		newPath string
		// code is the error's code.
		code connect.Code
		// text is the error's text.
		text string
	}{
		{name: "the root is not renamed", path: "/", newPath: "/root", code: connect.CodeInvalidArgument},
		{name: "no folder becomes the root", path: "/work", newPath: "/", code: connect.CodeInvalidArgument},
		{name: "the same path", path: "/work", newPath: "/work", code: connect.CodeInvalidArgument},
		{name: "below the folder", path: "/work", newPath: "/work/deeper", code: connect.CodeInvalidArgument},
		{name: "a folder that is not there", path: "/missing", newPath: "/elsewhere", code: connect.CodeNotFound, text: "no folder /missing"},
		{name: "a parent that is not there", path: "/work", newPath: "/nowhere/work", code: connect.CodeNotFound, text: "no folder /nowhere"},
		{name: "a folder is there", path: "/work", newPath: "/home", code: connect.CodeAlreadyExists, text: "the folder /home exists"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := rename(t, test.path, test.newPath)
			connectErr := wantCode(t, err, test.code)
			if test.text != "" && connectErr.Message() != test.text {
				t.Errorf("the refusal = %q, want %q", connectErr.Message(), test.text)
			}
		})
	}

	t.Run("a rename moves the folder, its folders and its bookmarks", func(t *testing.T) {
		bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/a", Folder: "/work/reading"})
		response, err := app.API().RenameFolder(local(t), &marksv1.RenameFolderRequest{Path: "/work", NewPath: "/home/work"})
		if err != nil {
			t.Fatalf("RenameFolder: %v", err)
		}
		if got := response.GetFolder().GetPath(); got != "/home/work" {
			t.Errorf("the renamed folder = %q, want /home/work", got)
		}
		listed, err := app.API().ListFolders(local(t), &marksv1.ListFoldersRequest{Folder: "/home/work", Recursive: true})
		if err != nil {
			t.Fatalf("ListFolders: %v", err)
		}
		if len(listed.GetFolders()) != 1 || listed.GetFolders()[0].GetPath() != "/home/work/reading" {
			t.Errorf("the folders below /home/work = %v, want the 1 /home/work/reading", listed.GetFolders())
		}
		moved, err := app.API().GetBookmark(local(t), &marksv1.GetBookmarkRequest{Id: bookmark.GetId()})
		if err != nil {
			t.Fatalf("GetBookmark(%s): %v", bookmark.GetId(), err)
		}
		if got := moved.GetBookmark().GetFolder(); got != "/home/work/reading" {
			t.Errorf("the bookmark is in %q, want /home/work/reading", got)
		}
		if got, want := moved.GetBookmark().GetUpdateTime().AsTime(), bookmark.GetUpdateTime().AsTime(); !got.Equal(want) {
			t.Errorf("the bookmark's update time after the rename = %v, want the %v of before", got, want)
		}
	})
}

// TestDeleteFolderNotEmpty checks that 1 folder that holds a bookmark or a folder
// is not deleted, that it is deleted once it is empty, and that the root is never
// deleted.
func TestDeleteFolderNotEmpty(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")
	mustCreateFolder(t, app, "/home")
	mustCreateFolder(t, app, "/home/inner")
	bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{Url: "https://example.com/a", Folder: "/work"})

	remove := func(t *testing.T, path string) error {
		t.Helper()
		_, err := app.API().DeleteFolder(local(t), &marksv1.DeleteFolderRequest{Path: path})
		return err
	}

	_, err := app.API().DeleteFolder(local(t), &marksv1.DeleteFolderRequest{Path: "/"})
	wantCode(t, err, connect.CodeInvalidArgument)

	connectErr := wantCode(t, remove(t, "/work"), connect.CodeFailedPrecondition)
	if connectErr.Message() != "the folder /work is not empty" {
		t.Errorf("the refusal = %q, want \"the folder /work is not empty\"", connectErr.Message())
	}
	connectErr = wantCode(t, remove(t, "/home"), connect.CodeFailedPrecondition)
	if connectErr.Message() != "the folder /home is not empty" {
		t.Errorf("the refusal of a folder with a sub-folder = %q, want \"the folder /home is not empty\"", connectErr.Message())
	}

	_, err = app.API().DeleteFolder(local(t), &marksv1.DeleteFolderRequest{Path: "/missing"})
	wantCode(t, err, connect.CodeNotFound)

	if err := remove(t, "/home/inner"); err != nil {
		t.Fatalf("the delete of the empty sub-folder: %v", err)
	}
	if _, err := app.API().MoveBookmark(local(t), &marksv1.MoveBookmarkRequest{Id: bookmark.GetId(), Folder: "/"}); err != nil {
		t.Fatalf("MoveBookmark: %v", err)
	}
	if err := remove(t, "/work"); err != nil {
		t.Fatalf("the delete of /work after the move: %v", err)
	}
	if got := count(t, app, "folders"); got != 1 {
		t.Errorf("the folders table holds %d rows, want the 1 of /home", got)
	}
}
