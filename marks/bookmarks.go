package marks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/frostyard/hearth/authz"
	"github.com/frostyard/hearth/events/outbox"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// maxSearchResults is how many bookmarks 1 search returns.
const maxSearchResults = 200

// The app's statements over a bookmark, its tags and its search row.
const (
	insertBookmarkSQL = `INSERT INTO bookmarks (id, url, title, notes, folder, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, ?)`
	readBookmarkSQL   = `SELECT id, url, title, notes, folder, create_time, update_time FROM bookmarks WHERE id = ?`
	urlOwnerSQL       = `SELECT id FROM bookmarks WHERE folder = ? AND url = ?`
	updateBookmarkSQL = `UPDATE bookmarks SET url = ?, title = ?, notes = ?, update_time = ? WHERE id = ?`
	moveBookmarkSQL   = `UPDATE bookmarks SET folder = ?, update_time = ? WHERE id = ?`
	deleteBookmarkSQL = `DELETE FROM bookmarks WHERE id = ?`
	selectTagsSQL     = `SELECT tag FROM bookmark_tags WHERE bookmark_id = ? ORDER BY tag`
	insertTagSQL      = `INSERT INTO bookmark_tags (bookmark_id, tag) VALUES (?, ?)`
	deleteTagSQL      = `DELETE FROM bookmark_tags WHERE bookmark_id = ? AND tag = ?`
	insertFTSSQL      = `INSERT INTO bookmarks_fts (id, title, url, notes, tags) VALUES (?, ?, ?, ?, ?)`
	deleteFTSSQL      = `DELETE FROM bookmarks_fts WHERE id = ?`
)

// CreateBookmark saves 1 URL in 1 folder, and it publishes the bookmark's
// created event, in 1 transaction.
//
// The folder must exist. The URL is canonicalized (see normalizedURL), and the
// tags are canonicalized and sorted. The same URL in the same folder is
// already_exists, with the id of the bookmark that holds it; the same URL in
// another folder is allowed.
func (s *Service) CreateBookmark(ctx context.Context, req *marksv1.CreateBookmarkRequest) (*marksv1.CreateBookmarkResponse, error) {
	folder, err := folderOf(req.GetFolder())
	if err != nil {
		return nil, err
	}
	url, err := normalizedURL(req.GetUrl())
	if err != nil {
		return nil, err
	}
	if err := checkText("title", req.GetTitle(), maxTitleLen); err != nil {
		return nil, err
	}
	if err := checkText("notes", req.GetNotes(), maxNotesLen); err != nil {
		return nil, err
	}
	tags, err := normalizedTags(req.GetTags())
	if err != nil {
		return nil, err
	}
	var bookmark *marksv1.Bookmark
	err = s.inTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		exists, err := folderExists(ctx, tx, folder)
		if err != nil {
			return err
		}
		if !exists {
			return notFound("no folder %s", folder)
		}
		owner, err := urlOwner(ctx, tx, folder, url)
		if err != nil {
			return err
		}
		if owner != "" {
			return duplicateURL(url, folder, owner)
		}
		id, err := s.newID()
		if err != nil {
			return err
		}
		now := s.now()
		if _, err := tx.ExecContext(ctx, insertBookmarkSQL, id, url, req.GetTitle(), req.GetNotes(), folder, now.UnixNano(), now.UnixNano()); err != nil {
			return fmt.Errorf("write the bookmark: %w", err)
		}
		if err := writeTags(ctx, tx, id, tags); err != nil {
			return err
		}
		if err := writeSearch(ctx, tx, id, req.GetTitle(), url, req.GetNotes(), tags); err != nil {
			return err
		}
		if err := marksv1.EmitBookmarkCreated(ctx, outbox.Tx(tx), id, folder); err != nil {
			return err
		}
		bookmark = &marksv1.Bookmark{
			Id:         id,
			Url:        url,
			Title:      req.GetTitle(),
			Notes:      req.GetNotes(),
			Tags:       tags,
			Folder:     folder,
			CreateTime: timestamppb.New(now),
			UpdateTime: timestamppb.New(now),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &marksv1.CreateBookmarkResponse{Bookmark: bookmark}, nil
}

// GetBookmark reads 1 bookmark, in the transaction that the generated hook of
// the RPC opened.
func (s *Service) GetBookmark(ctx context.Context, req *marksv1.GetBookmarkRequest) (*marksv1.GetBookmarkResponse, error) {
	tx, err := transaction(ctx)
	if err != nil {
		return nil, err
	}
	bookmark, err := readBookmark(ctx, tx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &marksv1.GetBookmarkResponse{Bookmark: bookmark}, nil
}

// UpdateBookmark changes the fields that the request sets, and it keeps the
// others: the URL, the title, the notes, and the tags it adds and removes.
//
// A change sets update_time and publishes the updated event. A call that
// changes nothing publishes no event and keeps update_time.
func (s *Service) UpdateBookmark(ctx context.Context, req *marksv1.UpdateBookmarkRequest) (*marksv1.UpdateBookmarkResponse, error) {
	var newURL, newTitle, newNotes *string
	if req.Url != nil {
		url, err := normalizedURL(req.GetUrl())
		if err != nil {
			return nil, err
		}
		newURL = &url
	}
	if req.Title != nil {
		if err := checkText("title", req.GetTitle(), maxTitleLen); err != nil {
			return nil, err
		}
		title := req.GetTitle()
		newTitle = &title
	}
	if req.Notes != nil {
		if err := checkText("notes", req.GetNotes(), maxNotesLen); err != nil {
			return nil, err
		}
		notes := req.GetNotes()
		newNotes = &notes
	}
	add, err := normalizeEach(req.GetAddTags())
	if err != nil {
		return nil, err
	}
	remove, err := normalizeEach(req.GetRemoveTags())
	if err != nil {
		return nil, err
	}
	for _, tag := range add {
		if slices.Contains(remove, tag) {
			return nil, invalidArgument("the tag %q is in add_tags and remove_tags", tag)
		}
	}

	tx, err := transaction(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := readBookmark(ctx, tx, req.GetId())
	if err != nil {
		return nil, err
	}
	url, title, notes := stored.GetUrl(), stored.GetTitle(), stored.GetNotes()
	changed := false
	if newURL != nil && *newURL != url {
		owner, err := urlOwner(ctx, tx, stored.GetFolder(), *newURL)
		if err != nil {
			return nil, err
		}
		if owner != "" && owner != stored.GetId() {
			return nil, duplicateURL(*newURL, stored.GetFolder(), owner)
		}
		url = *newURL
		changed = true
	}
	if newTitle != nil && *newTitle != title {
		title = *newTitle
		changed = true
	}
	if newNotes != nil && *newNotes != notes {
		notes = *newNotes
		changed = true
	}
	tags := mergedTags(stored.GetTags(), add, remove)
	if !slices.Equal(tags, stored.GetTags()) {
		changed = true
	}
	if len(tags) > maxTags {
		return nil, invalidArgument("the bookmark would hold %d tags, and the limit is %d", len(tags), maxTags)
	}
	if !changed {
		return &marksv1.UpdateBookmarkResponse{Bookmark: stored}, nil
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, updateBookmarkSQL, url, title, notes, now.UnixNano(), stored.GetId()); err != nil {
		return nil, fmt.Errorf("change the bookmark %s: %w", stored.GetId(), err)
	}
	for _, tag := range remove {
		if _, err := tx.ExecContext(ctx, deleteTagSQL, stored.GetId(), tag); err != nil {
			return nil, fmt.Errorf("remove the tag %q of the bookmark %s: %w", tag, stored.GetId(), err)
		}
	}
	for _, tag := range add {
		if slices.Contains(stored.GetTags(), tag) {
			continue
		}
		if _, err := tx.ExecContext(ctx, insertTagSQL, stored.GetId(), tag); err != nil {
			return nil, fmt.Errorf("add the tag %q to the bookmark %s: %w", tag, stored.GetId(), err)
		}
	}
	if err := writeSearch(ctx, tx, stored.GetId(), title, url, notes, tags); err != nil {
		return nil, err
	}
	if err := marksv1.EmitBookmarkUpdated(ctx, outbox.Tx(tx), stored.GetId(), stored.GetFolder()); err != nil {
		return nil, err
	}
	return &marksv1.UpdateBookmarkResponse{Bookmark: &marksv1.Bookmark{
		Id:         stored.GetId(),
		Url:        url,
		Title:      title,
		Notes:      notes,
		Tags:       tags,
		Folder:     stored.GetFolder(),
		CreateTime: stored.GetCreateTime(),
		UpdateTime: timestamppb.New(now),
	}}, nil
}

// MoveBookmark files 1 bookmark in another folder, and it publishes the
// bookmark's moved_out event on the folder it leaves, and then its moved_in
// event on the folder it arrives in.
//
// The destination must exist. A move to the folder the bookmark is in changes
// nothing and publishes no event.
func (s *Service) MoveBookmark(ctx context.Context, req *marksv1.MoveBookmarkRequest) (*marksv1.MoveBookmarkResponse, error) {
	folder, err := folderOf(req.GetFolder())
	if err != nil {
		return nil, err
	}
	tx, err := transaction(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := readBookmark(ctx, tx, req.GetId())
	if err != nil {
		return nil, err
	}
	exists, err := folderExists(ctx, tx, folder)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound("no folder %s", folder)
	}
	if stored.GetFolder() == folder {
		return &marksv1.MoveBookmarkResponse{Bookmark: stored}, nil
	}
	owner, err := urlOwner(ctx, tx, folder, stored.GetUrl())
	if err != nil {
		return nil, err
	}
	if owner != "" {
		return nil, duplicateURL(stored.GetUrl(), folder, owner)
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, moveBookmarkSQL, folder, now.UnixNano(), stored.GetId()); err != nil {
		return nil, fmt.Errorf("move the bookmark %s: %w", stored.GetId(), err)
	}
	if err := marksv1.EmitBookmarkMovedOut(ctx, outbox.Tx(tx), stored.GetId(), stored.GetFolder()); err != nil {
		return nil, err
	}
	if err := marksv1.EmitBookmarkMovedIn(ctx, outbox.Tx(tx), stored.GetId(), folder); err != nil {
		return nil, err
	}
	return &marksv1.MoveBookmarkResponse{Bookmark: &marksv1.Bookmark{
		Id:         stored.GetId(),
		Url:        stored.GetUrl(),
		Title:      stored.GetTitle(),
		Notes:      stored.GetNotes(),
		Tags:       stored.GetTags(),
		Folder:     folder,
		CreateTime: stored.GetCreateTime(),
		UpdateTime: timestamppb.New(now),
	}}, nil
}

// DeleteBookmark deletes 1 bookmark with its tags and its search row, and it
// publishes the bookmark's deleted event on the folder it was filed in.
func (s *Service) DeleteBookmark(ctx context.Context, req *marksv1.DeleteBookmarkRequest) (*marksv1.DeleteBookmarkResponse, error) {
	tx, err := transaction(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := readBookmark(ctx, tx, req.GetId())
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, deleteBookmarkSQL, stored.GetId()); err != nil {
		return nil, fmt.Errorf("delete the bookmark %s: %w", stored.GetId(), err)
	}
	if _, err := tx.ExecContext(ctx, deleteFTSSQL, stored.GetId()); err != nil {
		return nil, fmt.Errorf("delete the search row of the bookmark %s: %w", stored.GetId(), err)
	}
	if err := marksv1.EmitBookmarkDeleted(ctx, outbox.Tx(tx), stored.GetId(), stored.GetFolder()); err != nil {
		return nil, err
	}
	return &marksv1.DeleteBookmarkResponse{}, nil
}

// ListBookmarks lists the bookmarks of 1 folder, and, with recursive, of every
// folder below it, optionally only those with 1 tag. The folder must exist.
//
// The order is the folder, then the creation time, then the id. The list holds
// only the bookmarks whose folder the caller's grant covers.
func (s *Service) ListBookmarks(ctx context.Context, req *marksv1.ListBookmarksRequest) (*marksv1.ListBookmarksResponse, error) {
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
	tag := ""
	if req.GetTag() != "" {
		tag, err = normalizedTag(req.GetTag())
		if err != nil {
			return nil, err
		}
	}
	query, args := listBookmarksQuery(folder, req.GetRecursive(), tag)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read the bookmarks: %w", err)
	}
	var bookmarks []*marksv1.Bookmark
	for rows.Next() {
		bookmark, err := scanBookmark(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		bookmarks = append(bookmarks, bookmark)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("read the bookmarks: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read the bookmarks: %w", err)
	}
	listed, err := s.allowedBookmarks(ctx, grant, bookmarks)
	if err != nil {
		return nil, err
	}
	return &marksv1.ListBookmarksResponse{Bookmarks: listed}, nil
}

// SearchBookmarks finds the bookmarks whose title, URL, notes or tags hold every
// word of the query, in 1 folder and every folder below it, best match first.
//
// The search is SQLite's FTS5, kept in step with every change. The folder must
// exist, and the result holds only the bookmarks whose folder the caller's grant
// covers, at most maxSearchResults of them.
func (s *Service) SearchBookmarks(ctx context.Context, req *marksv1.SearchBookmarksRequest) (*marksv1.SearchBookmarksResponse, error) {
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
	match, err := searchMatch(req.GetQuery())
	if err != nil {
		return nil, err
	}
	query, args := searchQuery(folder)
	args = append([]any{match}, args...)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search the bookmarks: %w", err)
	}
	var bookmarks []*marksv1.Bookmark
	for rows.Next() {
		bookmark, err := scanBookmark(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		bookmarks = append(bookmarks, bookmark)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("search the bookmarks: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("search the bookmarks: %w", err)
	}
	found, err := s.allowedBookmarks(ctx, grant, bookmarks)
	if err != nil {
		return nil, err
	}
	return &marksv1.SearchBookmarksResponse{Bookmarks: found}, nil
}

// allowedBookmarks returns the bookmarks of a list or a search that the
// caller's grant covers, each with its tags.
func (s *Service) allowedBookmarks(ctx context.Context, grant authz.Grant, bookmarks []*marksv1.Bookmark) ([]*marksv1.Bookmark, error) {
	out := make([]*marksv1.Bookmark, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		if !allowRead(grant, bookmark) {
			continue
		}
		tags, err := readTags(ctx, s.DB, bookmark.GetId())
		if err != nil {
			return nil, err
		}
		bookmark.Tags = tags
		out = append(out, bookmark)
	}
	return out, nil
}

// listBookmarksQuery returns the statement and the arguments of the bookmarks
// of 1 folder.
func listBookmarksQuery(folder string, recursive bool, tag string) (string, []any) {
	query := `SELECT b.id, b.url, b.title, b.notes, b.folder, b.create_time, b.update_time FROM bookmarks AS b`
	var args []any
	if tag != "" {
		query += ` JOIN bookmark_tags AS t ON t.bookmark_id = b.id`
	}
	var where []string
	if tag != "" {
		where = append(where, `t.tag = ?`)
		args = append(args, tag)
	}
	switch {
	case !recursive:
		where = append(where, `b.folder = ?`)
		args = append(args, folder)
	case folder != authz.Root:
		where = append(where, `(b.folder = ? OR substr(b.folder, 1, length(?) + 1) = ? || '/')`)
		args = append(args, folder, folder, folder)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY b.folder, b.create_time, b.id`
	return query, args
}

// searchQuery returns the statement and the arguments of the search of 1
// folder and every folder below it. The match expression is the first argument.
func searchQuery(folder string) (string, []any) {
	query := `SELECT b.id, b.url, b.title, b.notes, b.folder, b.create_time, b.update_time
FROM bookmarks_fts
JOIN bookmarks AS b ON b.id = bookmarks_fts.id
WHERE bookmarks_fts MATCH ?`
	var args []any
	if folder != authz.Root {
		query += ` AND (b.folder = ? OR substr(b.folder, 1, length(?) + 1) = ? || '/')`
		args = append(args, folder, folder, folder)
	}
	query += ` ORDER BY rank LIMIT ?`
	args = append(args, maxSearchResults)
	return query, args
}

// searchMatch returns the FTS5 expression of 1 query: each word of the query as
// 1 FTS5 string, joined by spaces, so that every word must match. No word is
// invalid_argument.
func searchMatch(query string) (string, error) {
	words := strings.Fields(query)
	if len(words) == 0 {
		return "", invalidArgument("the query holds no word")
	}
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = `"` + strings.ReplaceAll(word, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " "), nil
}

// duplicateURL returns the error of 1 URL that its folder holds already.
func duplicateURL(url, folder, id string) error {
	return alreadyExists("the URL %s is already bookmarked in %s as %s", url, folder, id)
}

// urlOwner returns the id of the bookmark of 1 folder that holds 1 URL, or ""
// when the folder holds none.
func urlOwner(ctx context.Context, q querier, folder, url string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, urlOwnerSQL, folder, url).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the bookmark of the URL %s in %s: %w", url, folder, err)
	}
	return id, nil
}

// readBookmark reads 1 bookmark and its tags.
func readBookmark(ctx context.Context, q querier, id string) (*marksv1.Bookmark, error) {
	bookmark := &marksv1.Bookmark{}
	var createTime, updateTime int64
	err := q.QueryRowContext(ctx, readBookmarkSQL, id).Scan(
		&bookmark.Id, &bookmark.Url, &bookmark.Title, &bookmark.Notes, &bookmark.Folder, &createTime, &updateTime,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound("no bookmark %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("read the bookmark %s: %w", id, err)
	}
	bookmark.CreateTime = timestamppb.New(time.Unix(0, createTime).UTC())
	bookmark.UpdateTime = timestamppb.New(time.Unix(0, updateTime).UTC())
	tags, err := readTags(ctx, q, id)
	if err != nil {
		return nil, err
	}
	bookmark.Tags = tags
	return bookmark, nil
}

// scanBookmark reads 1 bookmark of a row that selects the bookmark's 7 columns,
// without its tags.
func scanBookmark(rows *sql.Rows) (*marksv1.Bookmark, error) {
	bookmark := &marksv1.Bookmark{}
	var createTime, updateTime int64
	if err := rows.Scan(&bookmark.Id, &bookmark.Url, &bookmark.Title, &bookmark.Notes, &bookmark.Folder, &createTime, &updateTime); err != nil {
		return nil, fmt.Errorf("read a bookmark: %w", err)
	}
	bookmark.CreateTime = timestamppb.New(time.Unix(0, createTime).UTC())
	bookmark.UpdateTime = timestamppb.New(time.Unix(0, updateTime).UTC())
	return bookmark, nil
}

// readTags returns the tags of 1 bookmark, sorted.
func readTags(ctx context.Context, q querier, id string) ([]string, error) {
	rows, err := q.QueryContext(ctx, selectTagsSQL, id)
	if err != nil {
		return nil, fmt.Errorf("read the tags of the bookmark %s: %w", id, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("read a tag of the bookmark %s: %w", id, err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the tags of the bookmark %s: %w", id, err)
	}
	return tags, nil
}

// writeTags writes the tags of 1 new bookmark.
func writeTags(ctx context.Context, tx *sql.Tx, id string, tags []string) error {
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, insertTagSQL, id, tag); err != nil {
			return fmt.Errorf("write the tag %q of the bookmark %s: %w", tag, id, err)
		}
	}
	return nil
}

// writeSearch writes the search row of 1 bookmark, in the change's transaction:
// its title, its URL, its notes, and its tags joined by 1 space.
func writeSearch(ctx context.Context, tx *sql.Tx, id, title, url, notes string, tags []string) error {
	if _, err := tx.ExecContext(ctx, deleteFTSSQL, id); err != nil {
		return fmt.Errorf("clear the search row of the bookmark %s: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, insertFTSSQL, id, title, url, notes, strings.Join(tags, " ")); err != nil {
		return fmt.Errorf("write the search row of the bookmark %s: %w", id, err)
	}
	return nil
}
