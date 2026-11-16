package marks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/frostyard/hearth/authz"
	"github.com/frostyard/hearth/events/outbox"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// The app's statements over a folder.
const (
	folderExistsSQL = `SELECT 1 FROM folders WHERE path = ?`
	insertFolderSQL = `INSERT INTO folders (path) VALUES (?)`
	deleteFolderSQL = `DELETE FROM folders WHERE path = ?`
	listFoldersSQL  = `SELECT path FROM folders ORDER BY path`
	// belowSQL matches the folders below 1 path: the path itself is not 1 of
	// them.
	belowSQL = `substr(path, 1, length(?) + 1) = ? || '/'`
	// moveFoldersSQL rewrites 1 path and every folder below it: the part after
	// the old path stays.
	moveFoldersSQL = `UPDATE folders SET path = ? || substr(path, length(?) + 1) WHERE path = ? OR substr(path, 1, length(?) + 1) = ? || '/'`
	// moveBookmarksSQL rewrites the folder of every bookmark of 1 folder and of
	// every folder below it. A bookmark's update_time does not change.
	moveBookmarksSQL = `UPDATE bookmarks SET folder = ? || substr(folder, length(?) + 1) WHERE folder = ? OR substr(folder, 1, length(?) + 1) = ? || '/'`
	// selectMovedBookmarksSQL reads the bookmarks that a folder's rename moves,
	// in id order. The folder scope is 3 arguments: the path, and the path
	// twice for the prefix.
	selectMovedBookmarksSQL = `SELECT id, folder FROM bookmarks WHERE folder = ? OR substr(folder, 1, length(?) + 1) = ? || '/' ORDER BY id`
	// countBookmarksSQL counts the bookmarks of 1 folder and of every folder
	// below it.
	countBookmarksSQL = `SELECT COUNT(*) FROM bookmarks WHERE folder = ? OR substr(folder, 1, length(?) + 1) = ? || '/'`
	// countFoldersBelowSQL counts the folders below 1 path.
	countFoldersBelowSQL = `SELECT COUNT(*) FROM folders WHERE ` + belowSQL
)

// CreateFolder makes 1 folder. The root path always exists, so it is not
// created, and the parent of the new folder must exist.
func (s *Service) CreateFolder(ctx context.Context, req *marksv1.CreateFolderRequest) (*marksv1.CreateFolderResponse, error) {
	path, err := folderOf(req.GetPath())
	if err != nil {
		return nil, err
	}
	if path == authz.Root {
		return nil, invalidArgument("the root folder %s always exists", authz.Root)
	}
	parent := parentOf(path)
	err = s.inTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		exists, err := folderExists(ctx, tx, parent)
		if err != nil {
			return err
		}
		if !exists {
			return notFound("no folder %s", parent)
		}
		exists, err = folderExists(ctx, tx, path)
		if err != nil {
			return err
		}
		if exists {
			return alreadyExists("the folder %s exists", path)
		}
		if _, err := tx.ExecContext(ctx, insertFolderSQL, path); err != nil {
			return fmt.Errorf("make the folder %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &marksv1.CreateFolderResponse{Folder: &marksv1.Folder{Path: path}}, nil
}

// RenameFolder gives 1 folder a new path, with every folder below it and every
// bookmark in them, in 1 transaction.
//
// Neither path is the root, and the new path is neither the folder's path nor
// below it. The folder must exist, the new path's parent must exist, and the new
// path must not exist. For each moved bookmark, in id order, the call publishes
// the bookmark's moved_out event on the folder it leaves and its moved_in event
// on the folder it arrives in.
func (s *Service) RenameFolder(ctx context.Context, req *marksv1.RenameFolderRequest) (*marksv1.RenameFolderResponse, error) {
	path, err := folderOf(req.GetPath())
	if err != nil {
		return nil, err
	}
	newPath, err := folderOf(req.GetNewPath())
	if err != nil {
		return nil, err
	}
	switch {
	case path == authz.Root:
		return nil, invalidArgument("the root folder %s is not renamed", authz.Root)
	case newPath == authz.Root:
		return nil, invalidArgument("no folder becomes the root folder %s", authz.Root)
	case newPath == path:
		return nil, invalidArgument("the new path %s is the folder's own path", newPath)
	case isWithin(newPath, path):
		return nil, invalidArgument("the new path %s is below the folder %s", newPath, path)
	}
	parent := parentOf(newPath)
	err = s.inTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		exists, err := folderExists(ctx, tx, path)
		if err != nil {
			return err
		}
		if !exists {
			return notFound("no folder %s", path)
		}
		exists, err = folderExists(ctx, tx, parent)
		if err != nil {
			return err
		}
		if !exists {
			return notFound("no folder %s", parent)
		}
		// The new path must hold nothing: the folder, or a folder below it,
		// would break the tree that the update rewrites.
		taken, err := folderInSubtree(ctx, tx, newPath)
		if err != nil {
			return err
		}
		if taken != "" {
			return alreadyExists("the folder %s exists", taken)
		}
		// Every path that the rename makes must obey the same rules as a
		// request path, before either update writes a row: a longer new path
		// would push a folder below it past the length or the topic limit.
		if err := checkRenamedPaths(ctx, tx, path, newPath); err != nil {
			return err
		}
		moved, err := movedBookmarks(ctx, tx, path)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, moveFoldersSQL, newPath, path, path, path, path); err != nil {
			return fmt.Errorf("rename the folders below %s: %w", path, err)
		}
		if _, err := tx.ExecContext(ctx, moveBookmarksSQL, newPath, path, path, path, path); err != nil {
			return fmt.Errorf("move the bookmarks below %s: %w", path, err)
		}
		for _, bookmark := range moved {
			to := newPath + bookmark.folder[len(path):]
			if err := marksv1.EmitBookmarkMovedOut(ctx, outbox.Tx(tx), bookmark.id, bookmark.folder); err != nil {
				return err
			}
			if err := marksv1.EmitBookmarkMovedIn(ctx, outbox.Tx(tx), bookmark.id, to); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &marksv1.RenameFolderResponse{Folder: &marksv1.Folder{Path: newPath}}, nil
}

// DeleteFolder deletes 1 folder that holds no bookmark and no folder.
//
// The root path is never deleted. A folder that holds anything is
// failed_precondition: its contents must move away first.
func (s *Service) DeleteFolder(ctx context.Context, req *marksv1.DeleteFolderRequest) (*marksv1.DeleteFolderResponse, error) {
	path, err := folderOf(req.GetPath())
	if err != nil {
		return nil, err
	}
	if path == authz.Root {
		return nil, invalidArgument("the root folder %s is not deleted", authz.Root)
	}
	err = s.inTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		exists, err := folderExists(ctx, tx, path)
		if err != nil {
			return err
		}
		if !exists {
			return notFound("no folder %s", path)
		}
		var bookmarks int
		if err := tx.QueryRowContext(ctx, countBookmarksSQL, path, path, path).Scan(&bookmarks); err != nil {
			return fmt.Errorf("count the bookmarks of the folder %s: %w", path, err)
		}
		var folders int
		if err := tx.QueryRowContext(ctx, countFoldersBelowSQL, path, path).Scan(&folders); err != nil {
			return fmt.Errorf("count the folders below %s: %w", path, err)
		}
		if bookmarks > 0 || folders > 0 {
			return failedPrecondition("the folder %s is not empty", path)
		}
		if _, err := tx.ExecContext(ctx, deleteFolderSQL, path); err != nil {
			return fmt.Errorf("delete the folder %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &marksv1.DeleteFolderResponse{}, nil
}

// ListFolders lists the folders in 1 folder, or, with recursive, every folder
// below it, sorted by path. The folder must exist, and the list holds only the
// folders that the caller's grant covers.
func (s *Service) ListFolders(ctx context.Context, req *marksv1.ListFoldersRequest) (*marksv1.ListFoldersResponse, error) {
	grant, err := authz.GrantFrom(ctx)
	if err != nil {
		return nil, err
	}
	folder, err := folderOf(req.GetFolder())
	if err != nil {
		return nil, err
	}
	exists, err := folderExists(ctx, s.DB, folder)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound("no folder %s", folder)
	}
	rows, err := s.DB.QueryContext(ctx, listFoldersSQL)
	if err != nil {
		return nil, fmt.Errorf("read the folders: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	response := &marksv1.ListFoldersResponse{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("read a folder: %w", err)
		}
		if req.GetRecursive() {
			if !isBelow(path, folder) {
				continue
			}
		} else if parentOf(path) != folder {
			continue
		}
		if !allowRead(grant, &marksv1.Bookmark{Folder: path}) {
			continue
		}
		response.Folders = append(response.Folders, &marksv1.Folder{Path: path})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the folders: %w", err)
	}
	return response, nil
}

// folderExists reports whether 1 folder is a stored folder. The root path always
// exists, and it is never stored.
func folderExists(ctx context.Context, q querier, path string) (bool, error) {
	if path == authz.Root {
		return true, nil
	}
	var one int
	err := q.QueryRowContext(ctx, folderExistsSQL, path).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read the folder %s: %w", path, err)
	}
	return true, nil
}

// folderInSubtree returns the first stored folder that is 1 path or below it, in
// path order, or "" when the subtree holds none.
func folderInSubtree(ctx context.Context, q querier, path string) (string, error) {
	var found string
	err := q.QueryRowContext(ctx, `SELECT path FROM folders WHERE path = ? OR `+belowSQL+` ORDER BY path LIMIT 1`, path, path, path).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the folders below %s: %w", path, err)
	}
	return found, nil
}

// checkRenamedPaths returns the error of 1 rename whose resulting paths break a
// folder rule: the folder, each folder below it, and the folder of each bookmark
// below it must pass checkFolderPath.
func checkRenamedPaths(ctx context.Context, tx *sql.Tx, path, newPath string) error {
	folders, err := foldersInSubtree(ctx, tx, path)
	if err != nil {
		return err
	}
	moved, err := movedBookmarks(ctx, tx, path)
	if err != nil {
		return err
	}
	for _, from := range folders {
		if err := checkFolderPath(newPath + from[len(path):]); err != nil {
			return invalidArgument("the rename %s to %s: %s", path, newPath, err.Error())
		}
	}
	for _, bookmark := range moved {
		if err := checkFolderPath(newPath + bookmark.folder[len(path):]); err != nil {
			return invalidArgument("the rename %s to %s, the bookmark %s: %s", path, newPath, bookmark.id, err.Error())
		}
	}
	return nil
}

// foldersInSubtree returns the paths of 1 folder and of every folder below it, in
// path order.
func foldersInSubtree(ctx context.Context, q querier, path string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT path FROM folders WHERE path = ? OR `+belowSQL+` ORDER BY path`, path, path, path)
	if err != nil {
		return nil, fmt.Errorf("read the folders below %s: %w", path, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var out []string
	for rows.Next() {
		var folder string
		if err := rows.Scan(&folder); err != nil {
			return nil, fmt.Errorf("read a folder below %s: %w", path, err)
		}
		out = append(out, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the folders below %s: %w", path, err)
	}
	return out, nil
}

// movedBookmark is 1 bookmark that a folder's rename moves: its id and the
// folder it is in before the rename.
type movedBookmark struct {
	// id is the bookmark's id.
	id string
	// folder is the folder the bookmark is in before the rename.
	folder string
}

// movedBookmarks returns the bookmarks of 1 folder and of every folder below it,
// in id order.
func movedBookmarks(ctx context.Context, q querier, path string) ([]movedBookmark, error) {
	rows, err := q.QueryContext(ctx, selectMovedBookmarksSQL, path, path, path)
	if err != nil {
		return nil, fmt.Errorf("read the bookmarks below %s: %w", path, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var out []movedBookmark
	for rows.Next() {
		var bookmark movedBookmark
		if err := rows.Scan(&bookmark.id, &bookmark.folder); err != nil {
			return nil, fmt.Errorf("read a bookmark below %s: %w", path, err)
		}
		out = append(out, bookmark)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the bookmarks below %s: %w", path, err)
	}
	return out, nil
}
