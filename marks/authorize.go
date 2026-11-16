package marks

import (
	"context"
	"database/sql"

	"github.com/frostyard/hearth/authz"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// This file is the app's whole permission code. The generated check of every RPC
// reads the request: the folder the request names, and for a move, both the
// folder it leaves and the folder it moves to. The hooks below read the folder
// that 1 stored bookmark is filed in, inside the transaction of the call. The
// app's lists keep only the rows that the caller's grant covers, with the same
// capabilities.

// AuthorizeGetBookmark checks marks:read on the folder that 1 stored bookmark is
// filed in, before the call reads it.
func (s *Service) AuthorizeGetBookmark(ctx context.Context, tx *sql.Tx, grant authz.Grant, req *marksv1.GetBookmarkRequest) error {
	return checkStoredFolder(ctx, tx, grant, readCap, req.GetId())
}

// AuthorizeUpdateBookmark checks marks:write on the folder that 1 stored bookmark
// is filed in, before the call changes it.
func (s *Service) AuthorizeUpdateBookmark(ctx context.Context, tx *sql.Tx, grant authz.Grant, req *marksv1.UpdateBookmarkRequest) error {
	return checkStoredFolder(ctx, tx, grant, writeCap, req.GetId())
}

// AuthorizeMoveBookmark checks marks:write on the folder that 1 stored bookmark
// is filed in, before the call moves it. The generated check has already checked
// the folder that the request moves the bookmark to.
func (s *Service) AuthorizeMoveBookmark(ctx context.Context, tx *sql.Tx, grant authz.Grant, req *marksv1.MoveBookmarkRequest) error {
	return checkStoredFolder(ctx, tx, grant, writeCap, req.GetId())
}

// AuthorizeDeleteBookmark checks marks:write on the folder that 1 stored bookmark
// is filed in, before the call deletes it.
func (s *Service) AuthorizeDeleteBookmark(ctx context.Context, tx *sql.Tx, grant authz.Grant, req *marksv1.DeleteBookmarkRequest) error {
	return checkStoredFolder(ctx, tx, grant, writeCap, req.GetId())
}

// checkStoredFolder checks 1 capability on the folder that 1 stored bookmark is
// filed in. A bookmark that is not there is not_found.
func checkStoredFolder(ctx context.Context, tx *sql.Tx, grant authz.Grant, cap, id string) error {
	bookmark, err := readBookmark(ctx, tx, id)
	if err != nil {
		return err
	}
	return grant.Check(cap, authz.Sel{Selector: "folder", Path: bookmark.GetFolder()})
}

// allowRead reports whether the caller's grant covers reading the folder of 1
// bookmark. A list or a search keeps only the rows that it allows.
func allowRead(grant authz.Grant, bookmark *marksv1.Bookmark) bool {
	return grant.Allows(readCap, "folder", bookmark.GetFolder())
}
