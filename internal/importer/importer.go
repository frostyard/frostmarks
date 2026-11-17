// Package importer plans and applies the import of 1 parsed Netscape bookmark
// file, over the app's API: it reads the target's folders and bookmarks with
// ListFolders and ListBookmarks, and, unless it is a dry run, it writes with
// CreateFolder and CreateBookmark.
//
// The contract is the section "The import" of docs/specs/cli.md. The importer
// checks the folder rules of the API before it writes (see checkFolder), so that
// a refused folder is a refusal of the plan, and no folder is made for it.
package importer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"connectrpc.com/connect"

	"github.com/frostyard/hearth/authz"
	"github.com/frostyard/hearth/events"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
	"github.com/frostyard/frostmarks/internal/netscape"
)

// maxFolderLen is the longest folder path, in bytes (docs/specs/api.md).
const maxFolderLen = 1024

// Result is the plan of 1 import, and, without a dry run, what the import did.
// It marshals to the JSON form of docs/specs/cli.md.
type Result struct {
	// DryRun is true for a plan that wrote nothing.
	DryRun bool `json:"dryRun"`
	// Folder is the canonical target folder.
	Folder string `json:"folder"`
	// FoldersMade is the count of Folders.
	FoldersMade int `json:"foldersMade"`
	// BookmarksAdded is the count of Added.
	BookmarksAdded int `json:"bookmarksAdded"`
	// SkippedDuplicates is the count of Skipped.
	SkippedDuplicates int `json:"skippedDuplicates"`
	// Refused is the count of Refusals.
	Refused int `json:"refused"`
	// Folders are the folders that the import makes, parents first.
	Folders []string `json:"folders"`
	// Added are the bookmarks that the import adds, in the order of the file.
	Added []Added `json:"added"`
	// Skipped are the duplicates, in the order of the file.
	Skipped []Skipped `json:"skipped"`
	// Refusals are the refused folders and bookmarks: the folders, then the
	// refusals of the parser, then the bookmarks that the rules or the daemon
	// refuse.
	Refusals []Refusal `json:"refusals"`
}

// Added is 1 bookmark that the import adds.
type Added struct {
	// ID is the bookmark's id; it is empty in a dry run.
	ID string `json:"id,omitempty"`
	// Folder is the canonical folder of the bookmark.
	Folder string `json:"folder"`
	// URL is the normalized URL.
	URL string `json:"url"`
	// Title is the bookmark's title.
	Title string `json:"title"`
}

// Skipped is 1 bookmark that the import skips as a duplicate.
type Skipped struct {
	// Folder is the canonical folder of the bookmark.
	Folder string `json:"folder"`
	// URL is the normalized URL.
	URL string `json:"url"`
	// Title is the bookmark's title.
	Title string `json:"title"`
}

// Refusal is 1 folder or bookmark that the import refuses.
type Refusal struct {
	// Folder is the folder of the bookmark, or the refused folder itself.
	Folder string `json:"folder"`
	// URL is the URL of the bookmark as the file gives it; it is empty for a
	// refused folder.
	URL string `json:"url"`
	// Title is the bookmark's title; it is empty for a refused folder.
	Title string `json:"title"`
	// Reason is the text of the rule that refuses it.
	Reason string `json:"reason"`
}

// folderPlan is the plan of 1 folder of the file.
type folderPlan struct {
	// path is the canonical path, or the path as the names give it for a refused
	// folder.
	path string
	// refusedBy is the path of the refused folder that this folder is, or is
	// below; it is empty for a folder that passes the rules.
	refusedBy string
}

// Run plans the import of 1 document below a target folder, and, without dryRun,
// applies the plan.
//
// An invalid target is an error, and so is any error of a call besides the
// answers that the plan expects: not_found of ListFolders, already_exists of
// CreateFolder, and already_exists and invalid_argument of CreateBookmark. What
// the import wrote before an error stays, and the next import skips it.
func Run(ctx context.Context, client marksv1connect.MarksClient, doc *netscape.Document, target string, dryRun bool) (*Result, error) {
	canonical, err := canonicalTarget(target)
	if err != nil {
		return nil, err
	}
	result := &Result{
		DryRun:   dryRun,
		Folder:   canonical,
		Folders:  []string{},
		Added:    []Added{},
		Skipped:  []Skipped{},
		Refusals: []Refusal{},
	}

	existing, missing, err := readFolders(ctx, client, canonical)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, path := range existing {
		known[path] = true
	}
	// A target that is not there has no bookmark to read.
	urls := map[string]map[string]bool{}
	if len(missing) == 0 {
		if urls, err = readURLs(ctx, client, canonical); err != nil {
			return nil, err
		}
	}

	// The folders to make: the missing chain of the target, then each folder of
	// the file that passes the rules and is not there.
	toMake := missing
	for _, path := range missing {
		known[path] = true
	}
	plans := map[string]folderPlan{"": {path: canonical}}
	for _, names := range doc.Folders {
		plan := planFolder(canonical, names, plans, result)
		plans[strings.Join(names, "/")] = plan
		if plan.refusedBy != "" || known[plan.path] {
			continue
		}
		known[plan.path] = true
		toMake = append(toMake, plan.path)
	}
	if !dryRun {
		if toMake, err = makeFolders(ctx, client, toMake); err != nil {
			return nil, err
		}
	}
	result.Folders = append(result.Folders, toMake...)

	for _, refusal := range doc.Refusals {
		result.Refusals = append(result.Refusals, Refusal{
			Folder: planOf(canonical, refusal.Path, plans).path,
			URL:    refusal.URL,
			Title:  refusal.Title,
			Reason: refusal.Reason,
		})
	}
	for _, bookmark := range doc.Bookmarks {
		plan := planOf(canonical, bookmark.Path, plans)
		switch {
		case plan.refusedBy != "":
			result.Refusals = append(result.Refusals, Refusal{
				Folder: plan.path,
				URL:    bookmark.URL,
				Title:  bookmark.Title,
				Reason: fmt.Sprintf("the folder %s is refused", plan.refusedBy),
			})
		case urls[plan.path][bookmark.URL]:
			result.Skipped = append(result.Skipped, Skipped{Folder: plan.path, URL: bookmark.URL, Title: bookmark.Title})
		default:
			if urls[plan.path] == nil {
				urls[plan.path] = map[string]bool{}
			}
			urls[plan.path][bookmark.URL] = true
			if dryRun {
				result.Added = append(result.Added, Added{Folder: plan.path, URL: bookmark.URL, Title: bookmark.Title})
			} else if err := addBookmark(ctx, client, plan.path, bookmark, result); err != nil {
				return nil, err
			}
		}
	}

	result.FoldersMade = len(result.Folders)
	result.BookmarksAdded = len(result.Added)
	result.SkippedDuplicates = len(result.Skipped)
	result.Refused = len(result.Refusals)
	return result, nil
}

// canonicalTarget returns the canonical form of the target folder: the root for
// an empty target, and an error for a target that breaks a folder rule.
func canonicalTarget(target string) (string, error) {
	if target == "" {
		target = authz.Root
	}
	return checkFolder(target)
}

// planFolder returns the plan of 1 folder of the file. A folder that breaks a
// folder rule adds 1 refusal to the result, with the rule's text as the reason; a
// folder below a refused folder adds none, and its bookmarks get the refusal of
// the folder.
func planFolder(target string, names []string, plans map[string]folderPlan, result *Result) folderPlan {
	if len(names) == 0 {
		return folderPlan{path: target}
	}
	if parent := plans[strings.Join(names[:len(names)-1], "/")]; parent.refusedBy != "" {
		return folderPlan{path: joinPath(target, names), refusedBy: parent.refusedBy}
	}
	raw := joinPath(target, names)
	cleaned, err := checkFolder(raw)
	if err != nil {
		result.Refusals = append(result.Refusals, Refusal{Folder: raw, Reason: err.Error()})
		return folderPlan{path: raw, refusedBy: raw}
	}
	return folderPlan{path: cleaned}
}

// planOf returns the plan of the folder that 1 list of names gives.
func planOf(target string, names []string, plans map[string]folderPlan) folderPlan {
	if plan, ok := plans[strings.Join(names, "/")]; ok {
		return plan
	}
	return folderPlan{path: joinPath(target, names)}
}

// joinPath returns the path of the folder that a list of names gives below a
// canonical folder.
func joinPath(base string, names []string) string {
	if len(names) == 0 {
		return base
	}
	return strings.TrimSuffix(base, "/") + "/" + strings.Join(names, "/")
}

// parentOf returns the parent of 1 canonical folder path.
func parentOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return authz.Root
}

// checkFolder returns the canonical form of 1 folder path, or the error of the
// folder rule that the path breaks (docs/specs/api.md, "A folder"). It mirrors
// the daemon's checks of a request's folder, which are private to its package:
// the length and the control characters of the value, hearth's canonical form,
// and the event topic of the folder within hearth's limit.
func checkFolder(path string) (string, error) {
	if err := checkValue(path); err != nil {
		return "", err
	}
	cleaned, err := authz.CleanPath(path)
	if err != nil {
		return "", err
	}
	if cleaned != path {
		// The canonical form is what a call sends, and what the daemon checks.
		if err := checkValue(cleaned); err != nil {
			return "", err
		}
	}
	topic, _, err := events.TopicOf(marksv1.EventBookmarkMovedOut, cleaned)
	if err != nil {
		return "", fmt.Errorf("the folder %q: %w", cleaned, err)
	}
	if len(topic) > events.MaxTopicLen {
		return "", fmt.Errorf("the folder %q gives the event topic of %d bytes, and the limit is %d", cleaned, len(topic), events.MaxTopicLen)
	}
	return cleaned, nil
}

// checkValue returns the error of 1 folder value that is too long, or that holds
// a control character.
func checkValue(path string) error {
	if len(path) > maxFolderLen {
		return fmt.Errorf("the folder %q is %d bytes, and the limit is %d", path, len(path), maxFolderLen)
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return fmt.Errorf("the folder %q holds the control character %q", path, r)
		}
	}
	return nil
}

// readFolders reads the folders below the target. For a target that is not there,
// it returns the missing folders instead: the target, and each missing parent
// above it, parents first.
func readFolders(ctx context.Context, client marksv1connect.MarksClient, target string) (existing, missing []string, err error) {
	response, err := client.ListFolders(ctx, &marksv1.ListFoldersRequest{Folder: target, Recursive: true})
	if err == nil {
		for _, folder := range response.GetFolders() {
			existing = append(existing, folder.GetPath())
		}
		return existing, nil, nil
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		return nil, nil, err
	}
	missing = []string{target}
	// The root always exists, so the walk stops there.
	for ancestor := parentOf(target); ancestor != authz.Root; ancestor = parentOf(ancestor) {
		if _, err := client.ListFolders(ctx, &marksv1.ListFoldersRequest{Folder: ancestor}); err == nil {
			break
		} else if connect.CodeOf(err) != connect.CodeNotFound {
			return nil, nil, err
		}
		missing = append(missing, ancestor)
	}
	slices.Reverse(missing)
	return nil, missing, nil
}

// readURLs reads the URLs of the target's bookmarks: the set of URLs of each
// folder, by the folder's path.
func readURLs(ctx context.Context, client marksv1connect.MarksClient, target string) (map[string]map[string]bool, error) {
	response, err := client.ListBookmarks(ctx, &marksv1.ListBookmarksRequest{Folder: target, Recursive: true})
	if err != nil {
		return nil, err
	}
	urls := map[string]map[string]bool{}
	for _, bookmark := range response.GetBookmarks() {
		if urls[bookmark.GetFolder()] == nil {
			urls[bookmark.GetFolder()] = map[string]bool{}
		}
		urls[bookmark.GetFolder()][bookmark.GetUrl()] = true
	}
	return urls, nil
}

// makeFolders makes each folder, in order, and returns the folders that it made:
// a folder that is there already (already_exists) is not one of them.
func makeFolders(ctx context.Context, client marksv1connect.MarksClient, paths []string) ([]string, error) {
	var made []string
	for _, path := range paths {
		_, err := client.CreateFolder(ctx, &marksv1.CreateFolderRequest{Path: path})
		if err == nil {
			made = append(made, path)
		} else if connect.CodeOf(err) != connect.CodeAlreadyExists {
			return nil, err
		}
	}
	return made, nil
}

// addBookmark adds 1 bookmark to a folder, and it adds the outcome to the result:
// the bookmark added, a duplicate (already_exists), or a refusal (invalid_argument,
// with the API's text as the reason). Any other error stops the import.
func addBookmark(ctx context.Context, client marksv1connect.MarksClient, folder string, bookmark netscape.Bookmark, result *Result) error {
	response, err := client.CreateBookmark(ctx, &marksv1.CreateBookmarkRequest{
		Url:    bookmark.URL,
		Title:  bookmark.Title,
		Notes:  bookmark.Notes,
		Tags:   bookmark.Tags,
		Folder: folder,
	})
	if err == nil {
		result.Added = append(result.Added, Added{ID: response.GetBookmark().GetId(), Folder: folder, URL: bookmark.URL, Title: bookmark.Title})
		return nil
	}
	switch connect.CodeOf(err) {
	case connect.CodeAlreadyExists:
		result.Skipped = append(result.Skipped, Skipped{Folder: folder, URL: bookmark.URL, Title: bookmark.Title})
	case connect.CodeInvalidArgument:
		reason := err.Error()
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			reason = connectErr.Message()
		}
		result.Refusals = append(result.Refusals, Refusal{Folder: folder, URL: bookmark.URL, Title: bookmark.Title, Reason: reason})
	default:
		return err
	}
	return nil
}
