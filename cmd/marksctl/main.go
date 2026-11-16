// Command marksctl is frostmarks' command-line client: 1 command for each RPC of
// the app's service, and the app's stdio MCP server.
//
// Every command calls the daemon over the app's private socket, so that 1 process
// owns the database and the outbox.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/frostyard/hearth/authz"
	"github.com/frostyard/hearth/mcpserver"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1/marksv1connect"
	"github.com/frostyard/frostmarks/internal/paths"
	"github.com/frostyard/frostmarks/internal/rpc"
)

// version is the build's version, which -ldflags sets.
var version = "dev"

// The CLI's exit codes.
const (
	// exitOK is a call that passed.
	exitOK = 0
	// exitError is a call that failed.
	exitError = 1
	// exitUsage is a usage error: no command, an unknown command, a missing or
	// an extra argument, or a bad flag.
	exitUsage = 2
)

// usageText is the CLI's usage, which every usage error prints.
const usageText = `usage: marksctl [-socket PATH] [-json] COMMAND [ARGS]

The commands are:
  create-bookmark  -url URL [-title T] [-notes N] [-tag T]... [-folder F]
  get-bookmark     ID
  update-bookmark  ID [-url U] [-title T] [-notes N] [-add-tag T]... [-remove-tag T]...
  move-bookmark    ID -folder F
  delete-bookmark  ID
  list-bookmarks   [-folder F] [-recursive] [-tag T]
  search-bookmarks [-folder F] QUERY...
  create-folder    PATH
  rename-folder    PATH NEW_PATH
  delete-folder    PATH
  list-folders     [-folder F] [-recursive]
  mcp              serve the MCP tools on stdin and stdout
  help             print this usage

The socket defaults to $XDG_RUNTIME_DIR/frostmarks.sock. With -json, the response
is printed as proto3 JSON.
`

// main runs the CLI and exits with its code.
func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}

// run runs the CLI and returns its exit code.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	global := flag.NewFlagSet("marksctl", flag.ContinueOnError)
	global.SetOutput(stderr)
	global.Usage = func() {
		fmt.Fprint(stderr, usageText)
	}
	socketFlag := global.String("socket", "", "the private socket of frostmarksd")
	asJSON := global.Bool("json", false, "print the response as proto3 JSON")
	if err := global.Parse(args); err != nil {
		return exitUsage
	}
	if global.NArg() == 0 {
		global.Usage()
		return exitUsage
	}
	command := global.Arg(0)
	rest := global.Args()[1:]
	if command == "help" {
		global.Usage()
		return exitOK
	}

	// The socket is resolved when a command needs it, so that a usage error
	// needs no environment.
	dial := func() (marksv1connect.MarksClient, error) {
		if *socketFlag != "" {
			return rpc.NewClient(*socketFlag), nil
		}
		socket, err := paths.Socket(getenv)
		if err != nil {
			return nil, err
		}
		return rpc.NewClient(socket), nil
	}
	usageError := func(format string, args ...any) int {
		fmt.Fprintf(stderr, format+"\n", args...)
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	// positional holds a command's arguments, which parse reads.
	var positional []string
	// parse reads a command's flags and its arguments, which may come before or
	// after the flags: want is -1 for 1 or more arguments.
	parse := func(flags *flag.FlagSet, want int) bool {
		flags.SetOutput(stderr)
		flags.Usage = global.Usage
		read, err := parseFlags(flags, rest)
		if err != nil {
			return false
		}
		positional = read
		if want < 0 {
			if len(positional) < 1 {
				fmt.Fprintf(stderr, "marksctl %s: want at least 1 argument\n", command)
				flags.Usage()
				return false
			}
			return true
		}
		if len(positional) != want {
			fmt.Fprintf(stderr, "marksctl %s: want %d argument(s), and it has %d\n", command, want, len(positional))
			flags.Usage()
			return false
		}
		return true
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "marksctl: %v\n", err)
		return exitError
	}

	ctx := context.Background()
	switch command {
	case "create-bookmark":
		flags := flag.NewFlagSet("marksctl create-bookmark", flag.ContinueOnError)
		url := flags.String("url", "", "the URL to save (required)")
		title := flags.String("title", "", "the bookmark's title")
		notes := flags.String("notes", "", "the bookmark's notes")
		tags := &stringList{}
		flags.Var(tags, "tag", "a tag; repeatable")
		folder := flags.String("folder", "", "the folder to file the bookmark in (default /)")
		if !parse(flags, 0) {
			return exitUsage
		}
		if *url == "" {
			return usageError("marksctl create-bookmark: -url is required")
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.CreateBookmark(ctx, &marksv1.CreateBookmarkRequest{
			Url: *url, Title: *title, Notes: *notes, Tags: *tags, Folder: *folder,
		})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printBookmarks(out, []*marksv1.Bookmark{response.GetBookmark()})
		})

	case "get-bookmark":
		flags := flag.NewFlagSet("marksctl get-bookmark", flag.ContinueOnError)
		if !parse(flags, 1) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.GetBookmark(ctx, &marksv1.GetBookmarkRequest{Id: positional[0]})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printBookmarks(out, []*marksv1.Bookmark{response.GetBookmark()})
		})

	case "update-bookmark":
		flags := flag.NewFlagSet("marksctl update-bookmark", flag.ContinueOnError)
		var url, title, notes string
		flags.StringVar(&url, "url", "", "the bookmark's new URL")
		flags.StringVar(&title, "title", "", "the bookmark's new title")
		flags.StringVar(&notes, "notes", "", "the bookmark's new notes")
		add := &stringList{}
		remove := &stringList{}
		flags.Var(add, "add-tag", "a tag to add; repeatable")
		flags.Var(remove, "remove-tag", "a tag to remove; repeatable")
		if !parse(flags, 1) {
			return exitUsage
		}
		// A field is set only when its flag is given: a field that the call
		// leaves out keeps its value.
		request := &marksv1.UpdateBookmarkRequest{Id: positional[0], AddTags: *add, RemoveTags: *remove}
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "url":
				request.Url = &url
			case "title":
				request.Title = &title
			case "notes":
				request.Notes = &notes
			}
		})
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.UpdateBookmark(ctx, request)
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printBookmarks(out, []*marksv1.Bookmark{response.GetBookmark()})
		})

	case "move-bookmark":
		flags := flag.NewFlagSet("marksctl move-bookmark", flag.ContinueOnError)
		folder := flags.String("folder", "", "the folder to move the bookmark to (required)")
		if !parse(flags, 1) {
			return exitUsage
		}
		if *folder == "" {
			return usageError("marksctl move-bookmark: -folder is required")
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.MoveBookmark(ctx, &marksv1.MoveBookmarkRequest{Id: positional[0], Folder: *folder})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printBookmarks(out, []*marksv1.Bookmark{response.GetBookmark()})
		})

	case "delete-bookmark":
		flags := flag.NewFlagSet("marksctl delete-bookmark", flag.ContinueOnError)
		if !parse(flags, 1) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.DeleteBookmark(ctx, &marksv1.DeleteBookmarkRequest{Id: positional[0]})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, nil)

	case "list-bookmarks":
		flags := flag.NewFlagSet("marksctl list-bookmarks", flag.ContinueOnError)
		folder := flags.String("folder", "", "the folder to list (default /)")
		recursive := flags.Bool("recursive", false, "list every folder below folder")
		tag := flags.String("tag", "", "keep only the bookmarks with this tag")
		if !parse(flags, 0) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.ListBookmarks(ctx, &marksv1.ListBookmarksRequest{Folder: *folder, Recursive: *recursive, Tag: *tag})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printBookmarks(out, response.GetBookmarks())
		})

	case "search-bookmarks":
		flags := flag.NewFlagSet("marksctl search-bookmarks", flag.ContinueOnError)
		folder := flags.String("folder", "", "the folder to search, and every folder below it (default /)")
		if !parse(flags, -1) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.SearchBookmarks(ctx, &marksv1.SearchBookmarksRequest{
			Query:  strings.Join(positional, " "),
			Folder: *folder,
		})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printBookmarks(out, response.GetBookmarks())
		})

	case "create-folder":
		flags := flag.NewFlagSet("marksctl create-folder", flag.ContinueOnError)
		if !parse(flags, 1) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.CreateFolder(ctx, &marksv1.CreateFolderRequest{Path: positional[0]})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printFolders(out, []*marksv1.Folder{response.GetFolder()})
		})

	case "rename-folder":
		flags := flag.NewFlagSet("marksctl rename-folder", flag.ContinueOnError)
		if !parse(flags, 2) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.RenameFolder(ctx, &marksv1.RenameFolderRequest{Path: positional[0], NewPath: positional[1]})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printFolders(out, []*marksv1.Folder{response.GetFolder()})
		})

	case "delete-folder":
		flags := flag.NewFlagSet("marksctl delete-folder", flag.ContinueOnError)
		if !parse(flags, 1) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.DeleteFolder(ctx, &marksv1.DeleteFolderRequest{Path: positional[0]})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, nil)

	case "list-folders":
		flags := flag.NewFlagSet("marksctl list-folders", flag.ContinueOnError)
		folder := flags.String("folder", "", "the parent folder (default /)")
		recursive := flags.Bool("recursive", false, "list every folder below folder")
		if !parse(flags, 0) {
			return exitUsage
		}
		client, err := dial()
		if err != nil {
			return fail(err)
		}
		response, err := client.ListFolders(ctx, &marksv1.ListFoldersRequest{Folder: *folder, Recursive: *recursive})
		if err != nil {
			return fail(err)
		}
		return printResult(stdout, stderr, *asJSON, response, func(out io.Writer) error {
			return printFolders(out, response.GetFolders())
		})

	case "mcp":
		if len(rest) != 0 {
			return usageError("marksctl mcp: it takes no argument")
		}
		return runMCP(stdin, stdout, stderr, func() (string, error) {
			if *socketFlag != "" {
				return *socketFlag, nil
			}
			return paths.Socket(getenv)
		})

	default:
		return usageError("marksctl: unknown command %q", command)
	}
}

// parseFlags reads the flags and the arguments of 1 command, in the order they
// are given: Go's flag package stops at the first argument, so the flags after it
// are read by the next round.
func parseFlags(flags *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		rest := flags.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// runMCP serves the app's MCP tools on the standard output and the standard
// input, and it calls the daemon over the private socket, as the local user.
func runMCP(stdin io.Reader, stdout, stderr io.Writer, socket func() (string, error)) int {
	path, err := socket()
	if err != nil {
		fmt.Fprintf(stderr, "marksctl: %v\n", err)
		return exitError
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := mcpserver.NewServer(
		&mcp.Implementation{Name: "frostmarks", Version: version},
		authz.LocalCaller(),
		marksv1.MarksTools(rpc.NewClient(path)),
	)
	if err := mcpserver.ServeStdio(ctx, server, readCloser{stdin}, writeCloser{stdout}); err != nil {
		fmt.Fprintf(stderr, "marksctl: %v\n", err)
		return exitError
	}
	return exitOK
}

// printResult writes 1 response: as proto3 JSON with asJSON, and else with the
// text that print gives. print is nil for a response with no text.
func printResult(out, stderr io.Writer, asJSON bool, response proto.Message, print func(io.Writer) error) int {
	var err error
	if asJSON {
		var data []byte
		data, err = protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(response)
		if err == nil {
			_, err = fmt.Fprintf(out, "%s\n", data)
		}
	} else if print != nil {
		err = print(out)
	}
	if err != nil {
		fmt.Fprintf(stderr, "marksctl: %v\n", err)
		return exitError
	}
	return exitOK
}

// printBookmarks writes 1 line for each bookmark: its id, its folder, its URL and
// its title, separated by tabs.
func printBookmarks(out io.Writer, bookmarks []*marksv1.Bookmark) error {
	for _, bookmark := range bookmarks {
		_, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\n",
			bookmark.GetId(), oneLine(bookmark.GetFolder()), oneLine(bookmark.GetUrl()), oneLine(bookmark.GetTitle()),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// printFolders writes 1 line for each folder: its path.
func printFolders(out io.Writer, folders []*marksv1.Folder) error {
	for _, folder := range folders {
		if _, err := fmt.Fprintln(out, folder.GetPath()); err != nil {
			return err
		}
	}
	return nil
}

// oneLine returns 1 value on 1 line: each control character, the tab included,
// is a space, so that 1 bookmark is 1 line of the text output.
func oneLine(value string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, value)
}

// stringList is a flag that collects each of its values, in the order they are
// given.
type stringList []string

// String implements flag.Value.
func (l *stringList) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(*l, ",")
}

// Set implements flag.Value.
func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

// readCloser is 1 reader as the closer that the MCP transport needs.
type readCloser struct {
	// reader is the stream.
	reader io.Reader
}

// Read reads 1 block of the stream.
func (r readCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

// Close closes nothing: the stream belongs to the process.
func (readCloser) Close() error {
	return nil
}

// writeCloser is 1 writer as the closer that the MCP transport needs.
type writeCloser struct {
	// writer is the stream.
	writer io.Writer
}

// Write writes 1 block of the stream.
func (w writeCloser) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

// Close closes nothing: the stream belongs to the process.
func (writeCloser) Close() error {
	return nil
}
