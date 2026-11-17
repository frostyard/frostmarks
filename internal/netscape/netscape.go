// Package netscape reads 1 Netscape bookmark file: the HTML export of Firefox,
// Chrome, Edge, Safari and Pinboard.
//
// The parser is pure: it reads a stream and returns the file's folders, its
// bookmarks and its refusals, and it calls no RPC. The contract is the section
// "The import" of docs/specs/cli.md. The scanner is written by hand, because the
// files are not well-formed HTML (no closing tag for <DT>, <P> or <DD>, no outer
// <DL> in Safari) and the repository takes no HTML module.
package netscape

import (
	"errors"
	"io"
	"strings"
	"time"
)

// MaxFileSize is the largest input that Parse reads, in bytes (32 MiB).
const MaxFileSize = 33554432

// MaxNameLen is the longest folder name, in bytes. A longer name is cut at a
// character boundary: no folder path of the API holds a name that long, so the
// import refuses the folder, and the parser's memory stays linear in the size of
// the file.
const MaxNameLen = 1024

// MaxFolders is the most distinct folders that a file holds.
const MaxFolders = 65536

// MaxLevels is the most <DL> levels that are open at 1 time. A folder path holds
// at most 512 folders (1024 bytes, 2 for each folder), so a deeper file holds no
// folder that the import can make, and the limit keeps the parser's memory
// linear in the size of the file.
const MaxLevels = 1024

var (
	// ErrTooLarge is the error of an input larger than MaxFileSize.
	ErrTooLarge = errors.New("the input is larger than 33554432 bytes")
	// ErrNotNetscape is the error of an input that does not start with the
	// declaration of a Netscape bookmark file.
	ErrNotNetscape = errors.New("the input is no Netscape bookmark file")
	// ErrTooDeep is the error of an input that opens more than MaxLevels <DL>
	// levels at 1 time.
	ErrTooDeep = errors.New("the input nests more than 1024 levels")
	// ErrTooManyFolders is the error of an input that holds more than MaxFolders
	// distinct folders.
	ErrTooManyFolders = errors.New("the input holds more than 65536 folders")
)

// Document is the content of 1 Netscape bookmark file.
type Document struct {
	// Folders are the folders of the file, in file order and without a repeat: a
	// parent comes before its children, so that Parent is always lower than the
	// folder's own index. The folders of 1 name below 1 parent are 1 folder.
	Folders []Folder
	// Bookmarks are the accepted links of the file, in file order.
	Bookmarks []Bookmark
	// Refusals are the links that the parser refuses, in file order.
	Refusals []Refusal
}

// Folder is 1 folder of a file: its name, and its place in the tree.
type Folder struct {
	// Name is the folder's name, on 1 line, with no "/", at most MaxNameLen
	// bytes.
	Name string
	// Parent is the index in Document.Folders of the folder that holds this
	// folder, or -1 for a folder directly in the target folder.
	Parent int
}

// Bookmark is 1 accepted link of a file.
type Bookmark struct {
	// Folder is the index in Document.Folders of the folder that holds the link,
	// or -1 for a link in the target folder.
	Folder int
	// URL is the normalized URL: http or https, with a lower-case scheme and
	// host and no fragment.
	URL string
	// Title is the text of the link, on 1 line and at most 2048 bytes.
	Title string
	// Notes is the text of the description that follows the link, at most
	// 65536 bytes; it is empty without a description.
	Notes string
	// Tags are the tags of the link, in the order of the file, without a repeat.
	Tags []string
	// Added is the time of ADD_DATE, or the zero time for an absent or an
	// invalid date.
	Added time.Time
}

// Refusal is 1 link that the parser refuses.
type Refusal struct {
	// Folder is the index in Document.Folders of the folder that holds the link,
	// or -1 for a link in the target folder.
	Folder int
	// URL is the HREF as the file gives it, trimmed; it is empty for a link
	// with no HREF.
	URL string
	// Title is the text of the link.
	Title string
	// Reason is the text of the error of the rule that refuses the link.
	Reason string
}

// Parse reads 1 Netscape bookmark file.
//
// It returns ErrTooLarge for an input of more than MaxFileSize bytes, and
// ErrNotNetscape for an input that does not start with the declaration
// <!DOCTYPE NETSCAPE-Bookmark-file-1>, in any letter case, after an optional
// byte order mark, white space and comments. It returns ErrTooDeep for an input
// that opens more than MaxLevels <DL> levels at 1 time, and ErrTooManyFolders for
// an input with more than MaxFolders distinct folders. Each invalid UTF-8
// sequence of the input becomes U+FFFD.
func Parse(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileSize {
		return nil, ErrTooLarge
	}
	text := strings.ToValidUTF8(string(data), "\uFFFD")
	text = strings.TrimPrefix(text, "\uFEFF")
	rest, ok := afterDoctype(text)
	if !ok {
		return nil, ErrNotNetscape
	}
	p := &parser{src: rest, folderIndex: map[folderKey]int{}}
	if err := p.run(); err != nil {
		return nil, err
	}
	return &p.doc, nil
}

// doctypeName is the name that the declaration of a Netscape bookmark file
// carries, after <!DOCTYPE.
const doctypeName = "NETSCAPE-Bookmark-file-1"

// afterDoctype returns the text after the declaration of a Netscape bookmark
// file, and whether the text starts with it: after white space and comments, in
// any letter case.
func afterDoctype(text string) (string, bool) {
	for {
		text = strings.TrimLeft(text, " \t\r\n\f\v")
		if !strings.HasPrefix(text, "<!--") {
			break
		}
		end := strings.Index(text[len("<!--"):], "-->")
		if end < 0 {
			return "", false
		}
		text = text[len("<!--")+end+len("-->"):]
	}
	const start = "<!DOCTYPE"
	if !hasPrefixFold(text, start) {
		return "", false
	}
	rest := text[len(start):]
	trimmed := strings.TrimLeft(rest, " \t\r\n\f\v")
	// The name follows white space, and only white space and ">" follow the name.
	if len(trimmed) == len(rest) || !hasPrefixFold(trimmed, doctypeName) {
		return "", false
	}
	rest = strings.TrimLeft(trimmed[len(doctypeName):], " \t\r\n\f\v")
	if !strings.HasPrefix(rest, ">") {
		return "", false
	}
	return rest[1:], true
}

// hasPrefixFold reports whether a text starts with a prefix, in any letter case.
func hasPrefixFold(text, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}
