package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
	"github.com/frostyard/frostmarks/internal/importer"
	"github.com/frostyard/frostmarks/internal/netscape"
)

// runImport runs the import command: it reads 1 Netscape bookmark file, and it
// plans and applies its import below a target folder (docs/specs/cli.md, "The
// import"). It reads the file before it dials, so that a file that the import
// refuses stops it before a call. It returns the command's exit code.
func runImport(ctx context.Context, dial func() (marksv1connect.MarksClient, error), stdout, stderr io.Writer, path, folder string, dryRun, asJSON bool) int {
	// An error is 1 line, whatever a file name or a path holds.
	fail := func(err error) int {
		fmt.Fprintf(stderr, "marksctl: %s\n", oneLine(err.Error()))
		return exitError
	}
	doc, err := readNetscape(path)
	if err != nil {
		return fail(err)
	}
	client, err := dial()
	if err != nil {
		return fail(err)
	}
	result, err := importer.Run(ctx, client, doc, folder, dryRun)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(result)
	} else {
		err = printImport(stdout, result)
	}
	if err != nil {
		return fail(err)
	}
	return exitOK
}

// readNetscape reads the file at 1 path, and it gives the errors of the file's
// rules the texts of docs/specs/cli.md.
func readNetscape(path string) (*netscape.Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	doc, err := netscape.Parse(file)
	switch {
	case errors.Is(err, netscape.ErrTooLarge):
		return nil, fmt.Errorf("the file %s is larger than %d bytes", path, netscape.MaxFileSize)
	case errors.Is(err, netscape.ErrNotNetscape):
		return nil, fmt.Errorf("the file %s is no Netscape bookmark file", path)
	case errors.Is(err, netscape.ErrTooDeep):
		return nil, fmt.Errorf("the file %s nests more than %d levels", path, netscape.MaxLevels)
	case errors.Is(err, netscape.ErrTooManyFolders):
		return nil, fmt.Errorf("the file %s holds more than %d folders", path, netscape.MaxFolders)
	case err != nil:
		return nil, fmt.Errorf("read the file %s: %w", path, err)
	}
	return doc, nil
}

// printImport writes the text output of 1 import: 1 line for each folder made,
// bookmark added, duplicate and refusal, in that order, then the line of counts,
// which starts with "dry run: " for a dry run.
func printImport(out io.Writer, result *importer.Result) error {
	for _, folder := range result.Folders {
		if _, err := fmt.Fprintf(out, "folder\t%s\n", oneLine(folder)); err != nil {
			return err
		}
	}
	for _, added := range result.Added {
		if _, err := fmt.Fprintf(out, "add\t%s\t%s\t%s\n", oneLine(added.Folder), oneLine(added.URL), oneLine(added.Title)); err != nil {
			return err
		}
	}
	for _, skipped := range result.Skipped {
		if _, err := fmt.Fprintf(out, "skip\t%s\t%s\t%s\n", oneLine(skipped.Folder), oneLine(skipped.URL), oneLine(skipped.Title)); err != nil {
			return err
		}
	}
	for _, refusal := range result.Refusals {
		if _, err := fmt.Fprintf(out, "refuse\t%s\t%s\t%s\n", oneLine(refusal.Folder), oneLine(refusal.URL), oneLine(refusal.Reason)); err != nil {
			return err
		}
	}
	prefix := ""
	if result.DryRun {
		prefix = "dry run: "
	}
	_, err := fmt.Fprintf(out, "%sfolders made: %d, bookmarks added: %d, skipped as duplicates: %d, refused: %d\n",
		prefix, result.FoldersMade, result.BookmarksAdded, result.SkippedDuplicates, result.Refused)
	return err
}
