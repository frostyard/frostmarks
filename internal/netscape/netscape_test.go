package netscape

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// summary returns a document as lines that a test compares: 1 line for each
// folder, bookmark and refusal, in that order.
func summary(doc *Document) []string {
	var lines []string
	for _, folder := range doc.Folders {
		lines = append(lines, "folder "+strings.Join(folder, "/"))
	}
	for _, b := range doc.Bookmarks {
		added := "-"
		if !b.Added.IsZero() {
			added = fmt.Sprint(b.Added.Unix())
		}
		lines = append(lines, fmt.Sprintf("bookmark %s|%s|%s|%s|%s|%s",
			strings.Join(b.Path, "/"), b.URL, b.Title, b.Notes, strings.Join(b.Tags, ","), added))
	}
	for _, r := range doc.Refusals {
		lines = append(lines, fmt.Sprintf("refusal %s|%s|%s|%s", strings.Join(r.Path, "/"), r.URL, r.Title, r.Reason))
	}
	return lines
}

// parseString parses 1 text, and it fails the test on an error.
func parseString(t *testing.T, text string) *Document {
	t.Helper()
	doc, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

// TestFixtures checks that each export parses to its folders, bookmarks and
// refusals.
func TestFixtures(t *testing.T) {
	tests := []struct {
		// name names the case, and the fixture file.
		name string
		// want are the lines of the summary.
		want []string
	}{
		{
			name: "firefox",
			want: []string{
				"folder Reading",
				"folder Reading/Later",
				"folder Bookmarks Toolbar",
				"folder Other Bookmarks",
				"bookmark Reading|https://example.com/article|An article|Read this on the train.|go,reading-list|1700000200",
				"bookmark Reading|https://example.org/a?x=1&y=2|A & B||" + "|1700000400",
				"bookmark Reading/Later|https://example.net/Later|Read later||sqlite|1700000800",
				"bookmark Bookmarks Toolbar|https://example.com/|Example|||1700001200",
				"bookmark Bookmarks Toolbar|https://example.com/|Example again|||1700001400",
				"bookmark Bookmarks Toolbar|https://example.org/docs|Docs|The docs of example.org.|docs|1700001800",
				"bookmark Other Bookmarks|https://example.net/other|Other|||1700002200",
				"bookmark |https://example.com/menu|A menu link|||1700002400",
				`refusal Bookmarks Toolbar|place:sort=8&maxResults=10|Most Visited|the URL "place:sort=8&maxResults=10" must start with http:// or https://`,
			},
		},
		{
			name: "chrome",
			want: []string{
				"folder Bookmarks bar",
				"folder Bookmarks bar/Work",
				"folder Bookmarks bar/Work/Notes",
				"folder Other bookmarks",
				"bookmark Bookmarks bar|https://example.com/|Example|||1700000200",
				"bookmark Bookmarks bar/Work|https://example.org/tracker|Tracker|||1700000500",
				"bookmark Bookmarks bar/Work/Notes|https://example.net/notes|Notes|||1700000800",
				"bookmark Other bookmarks|https://example.com/other|Other|||1700001200",
				`refusal Bookmarks bar|javascript:alert(document.title)|A bookmarklet|the URL "javascript:alert(document.title)" must start with http:// or https://`,
			},
		},
		{
			name: "edge",
			want: []string{
				"folder Favorites bar",
				"folder Favorites bar/Tools",
				"folder Other favorites",
				"bookmark Favorites bar|https://example.com/start|Start page|||1700000200",
				"bookmark Favorites bar/Tools|https://example.org/tools|Tools|||1700000600",
				"bookmark Other favorites|https://example.net/other|Other|||1700000900",
				`refusal Favorites bar|file:///home/user/notes.html|A local file|the URL "file:///home/user/notes.html" must start with http:// or https://`,
			},
		},
		{
			name: "safari",
			want: []string{
				"folder Favorites",
				"folder Favorites/Nested",
				"folder Reading List",
				"bookmark Favorites|https://example.com/favorite|A favorite|||-",
				"bookmark Favorites/Nested|https://example.org/nested|A nested page|||-",
				"bookmark Reading List|https://example.net/reading|Reading|A page to read.||-",
				"bookmark |https://example.com/top|A top-level link|||-",
			},
		},
		{
			name: "pinboard",
			want: []string{
				"bookmark |https://example.com/go|Go and SQLite|Notes on a Go program that uses SQLite.|go,sqlite|1700000000",
				"bookmark |https://example.org/read|Something to read||reading|1700000100",
				"bookmark |https://example.com/go|The same page, in capitals|A duplicate that differs by the case of its host.|go|1700000200",
				"bookmark |https://example.net/page|A page|||1700000300",
				"bookmark |https://example.net/page|A page, without its fragment|||1700000400",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := os.Open(filepath.Join("testdata", test.name+".html"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			doc, err := Parse(file)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := summary(doc); !reflect.DeepEqual(got, test.want) {
				t.Errorf("the summary differs\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// TestFixtureHosts checks that every fixture names only the hosts of the
// reserved example domains.
func TestFixtureHosts(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("the fixtures: %v, %v", files, err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		doc := parseString(t, string(data))
		for _, b := range doc.Bookmarks {
			host := strings.TrimPrefix(strings.TrimPrefix(b.URL, "https://"), "http://")
			host, _, _ = strings.Cut(host, "/")
			if !strings.HasPrefix(host, "example.") {
				t.Errorf("%s: the URL %s is not on an example host", file, b.URL)
			}
		}
	}
}

// TestDeclaration checks which inputs are Netscape bookmark files.
func TestDeclaration(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// input is the text.
		input string
		// want is the error, or nil.
		want error
	}{
		{name: "the declaration", input: "<!DOCTYPE NETSCAPE-Bookmark-file-1>\n<DL></DL>", want: nil},
		{name: "lower case", input: "<!doctype netscape-bookmark-file-1>", want: nil},
		{name: "a byte order mark and a comment", input: "\uFEFF\n<!-- a comment -->\n  <!DOCTYPE NETSCAPE-Bookmark-file-1>", want: nil},
		{name: "no declaration", input: "<html><body>a page</body></html>", want: ErrNotNetscape},
		{name: "another declaration", input: "<!DOCTYPE html>", want: ErrNotNetscape},
		{name: "an empty input", input: "", want: ErrNotNetscape},
		{name: "text before the declaration", input: "text <!DOCTYPE NETSCAPE-Bookmark-file-1>", want: ErrNotNetscape},
		{name: "a comment that never ends", input: "<!-- <!DOCTYPE NETSCAPE-Bookmark-file-1>", want: ErrNotNetscape},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.input))
			if !errors.Is(err, test.want) {
				t.Errorf("Parse = %v, want %v", err, test.want)
			}
		})
	}
}

// zeros is a reader of an endless run of 0 bytes.
type zeros struct{}

// Read fills the buffer with 0 bytes.
func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// TestTooLarge checks the size limit: MaxFileSize bytes pass the size rule, and 1
// byte more is ErrTooLarge.
func TestTooLarge(t *testing.T) {
	_, err := Parse(io.LimitReader(zeros{}, MaxFileSize+1))
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("Parse of %d bytes = %v, want ErrTooLarge", MaxFileSize+1, err)
	}
	_, err = Parse(io.LimitReader(zeros{}, MaxFileSize))
	if !errors.Is(err, ErrNotNetscape) {
		t.Errorf("Parse of %d bytes = %v, want ErrNotNetscape (it is not too large)", MaxFileSize, err)
	}
}

// TestTree checks the rules of the tree and of the scanner.
func TestTree(t *testing.T) {
	const head = "<!DOCTYPE NETSCAPE-Bookmark-file-1>\n"
	tests := []struct {
		// name names the case.
		name string
		// body is the text after the declaration.
		body string
		// want are the lines of the summary.
		want []string
	}{
		{
			name: "single-quoted and unquoted attributes",
			body: `<DL><DT><A HREF='https://example.com/a' ADD_DATE=1700000000 TAGS=go,x>One</A><DT><a href=https://example.com/b?q=1>Two</a></DL>`,
			want: []string{
				"bookmark |https://example.com/a|One||go,x|1700000000",
				"bookmark |https://example.com/b?q=1|Two|||-",
			},
		},
		{
			name: "an attribute with no value and a quoted greater-than",
			body: `<DT><H3 FOLDED>Folder</H3><DL><DT><A HREF="https://example.com/?a>b" TITLE="x>y">Link</A></DL>`,
			want: []string{
				"folder Folder",
				"bookmark Folder|https://example.com/?a>b|Link|||-",
			},
		},
		{
			name: "a comment between tags",
			body: `<DL><!-- <A HREF="https://example.com/no">Comment</A> --><DT><A HREF="https://example.com/yes">Yes</A></DL>`,
			want: []string{
				"bookmark |https://example.com/yes|Yes|||-",
			},
		},
		{
			name: "no end tag of the link",
			body: `<DL><DT><A HREF="https://example.com/a">One<DT><A HREF="https://example.com/b">Two</DL>`,
			want: []string{
				"bookmark |https://example.com/a|One|||-",
				"bookmark |https://example.com/b|Two|||-",
			},
		},
		{
			name: "no end tag of the folder name",
			body: `<DT><H3>Folder<DL><DT><A HREF="https://example.com/a">One</A></DL>`,
			want: []string{
				"folder Folder",
				"bookmark Folder|https://example.com/a|One|||-",
			},
		},
		{
			name: "the end of the file closes each level",
			body: `<DL><DT><H3>A</H3><DL><DT><A HREF="https://example.com/a">One</A>`,
			want: []string{
				"folder A",
				"bookmark A|https://example.com/a|One|||-",
			},
		},
		{
			name: "a stray end of a list",
			body: `</DL></DL><DT><A HREF="https://example.com/a">One</A><DL><DT><A HREF="https://example.com/b">Two</A></DL></DL></DL>`,
			want: []string{
				"bookmark |https://example.com/a|One|||-",
				"bookmark |https://example.com/b|Two|||-",
			},
		},
		{
			name: "a list with no folder opens no folder",
			body: `<DL><DL><DT><A HREF="https://example.com/a">One</A></DL></DL>`,
			want: []string{
				"bookmark |https://example.com/a|One|||-",
			},
		},
		{
			name: "folders of 1 name are 1 folder",
			body: `<DL><DT><H3>A</H3><DL><DT><A HREF="https://example.com/1">1</A></DL><DT><H3>A</H3><DL><DT><A HREF="https://example.com/2">2</A></DL></DL>`,
			want: []string{
				"folder A",
				"bookmark A|https://example.com/1|1|||-",
				"bookmark A|https://example.com/2|2|||-",
			},
		},
		{
			name: "a folder with no bookmark",
			body: `<DL><DT><H3>Empty</H3><DL><p></DL></DL>`,
			want: []string{"folder Empty"},
		},
		{
			name: "folder names",
			body: "<DL><DT><H3>  a/b \t c\n</H3><DL></DL><DT><H3> </H3><DL></DL><DT><H3>.</H3><DL></DL><DT><H3>..</H3><DL></DL><DT><H3>A &amp; B</H3><DL></DL></DL>",
			want: []string{
				"folder a-b c",
				"folder Untitled",
				"folder A & B",
			},
		},
		{
			name: "a description after a folder or a list is not a note",
			body: `<DL><DT><A HREF="https://example.com/a">One</A><DT><H3>F</H3><DD>Folder note<DL><DD>List note</DL><DD>After the list</DL>`,
			want: []string{
				"folder F",
				"bookmark |https://example.com/a|One|||-",
			},
		},
		{
			name: "a description of the first link only",
			body: `<DL><DT><A HREF="https://example.com/a">One</A><DD> First &amp; note <DD>Second</DL>`,
			want: []string{
				"bookmark |https://example.com/a|One|First & note||-",
			},
		},
		{
			name: "a description after a refused link",
			body: `<DL><DT><A HREF="https://example.com/a">One</A><DT><A HREF="javascript:x">Two</A><DD>A note</DL>`,
			want: []string{
				"bookmark |https://example.com/a|One|||-",
				`refusal |javascript:x|Two|the URL "javascript:x" must start with http:// or https://`,
			},
		},
		{
			name: "a link outside every list",
			body: `<DT><A HREF="https://example.com/a">One</A>`,
			want: []string{
				"bookmark |https://example.com/a|One|||-",
			},
		},
		{
			name: "a title with entities and white space",
			body: "<A HREF=\"https://example.com/a\">  A\t&lt;b&gt;\n  &#38; &copy;  \u00a0x </A>",
			want: []string{
				"bookmark |https://example.com/a|A <b> & © x|||-",
			},
		},
		{
			name: "a title with a control character",
			body: "<A HREF=\"https://example.com/a\">a\x00\x01b\x7f c</A>",
			want: []string{
				"bookmark |https://example.com/a|a b c|||-",
			},
		},
		{
			name: "a less-than sign in a text",
			body: `<A HREF="https://example.com/a">1 < 2 <b>x</b></A>`,
			want: []string{
				"bookmark |https://example.com/a|1 < 2|||-",
			},
		},
		{
			name: "invalid UTF-8",
			body: "<A HREF=\"https://example.com/a\">a\xffb\xc3</A>",
			want: []string{
				"bookmark |https://example.com/a|a\uFFFDb\uFFFD|||-",
			},
		},
		{
			name: "dates",
			body: `<A HREF="https://example.com/a" ADD_DATE="abc">1</A><A HREF="https://example.com/b" ADD_DATE="-5">2</A><A HREF="https://example.com/c" ADD_DATE=" 1700000000 ">3</A><A HREF="https://example.com/d" ADD_DATE="99999999999999">4</A><A HREF="https://example.com/e" ADD_DATE>5</A>`,
			want: []string{
				"bookmark |https://example.com/a|1|||-",
				"bookmark |https://example.com/b|2|||-",
				"bookmark |https://example.com/c|3|||1700000000",
				"bookmark |https://example.com/d|4|||-",
				"bookmark |https://example.com/e|5|||-",
			},
		},
		{
			name: "an unterminated tag",
			body: `<A HREF="https://example.com/a">One</A><A HREF="https://example.com/b`,
			want: []string{
				"bookmark |https://example.com/a|One|||-",
				"bookmark |https://example.com/b||||-",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summary(parseString(t, head+test.body))
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("the summary differs\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// TestRefusedURLs checks each kind of URL that the parser refuses, with the text
// of the API's error.
func TestRefusedURLs(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", maxURLLen)
	tests := []struct {
		// name names the case.
		name string
		// link is the <A> tag.
		link string
		// url is the refused URL as given.
		url string
		// reason is the refusal's text.
		reason string
	}{
		{name: "no HREF", link: `<A ADD_DATE="1">Title</A>`, url: "", reason: "the link has no HREF"},
		{name: "javascript", link: `<A HREF="javascript:alert(1)">Title</A>`, url: "javascript:alert(1)", reason: `the URL "javascript:alert(1)" must start with http:// or https://`},
		{name: "data", link: `<A HREF="data:text/plain,hi">Title</A>`, url: "data:text/plain,hi", reason: `the URL "data:text/plain,hi" must start with http:// or https://`},
		{name: "file", link: `<A HREF="file:///a/b">Title</A>`, url: "file:///a/b", reason: `the URL "file:///a/b" must start with http:// or https://`},
		{name: "place", link: `<A HREF="place:sort=8">Title</A>`, url: "place:sort=8", reason: `the URL "place:sort=8" must start with http:// or https://`},
		{name: "no scheme", link: `<A HREF="example.com/a">Title</A>`, url: "example.com/a", reason: `the URL "example.com/a" must start with http:// or https://`},
		{name: "an empty HREF", link: `<A HREF="  ">Title</A>`, url: "", reason: `the URL "" must start with http:// or https://`},
		{name: "no host", link: `<A HREF="https:///path">Title</A>`, url: "https:///path", reason: `the URL "https:///path" has no host`},
		{name: "too long", link: `<A HREF="` + long + `">Title</A>`, url: long, reason: fmt.Sprintf("the URL is %d bytes, and the limit is %d", len(long), maxURLLen)},
		{name: "unparsable", link: `<A HREF="https://exa mple.com/">Title</A>`, url: "https://exa mple.com/", reason: `the URL "https://exa mple.com/" is no URL: parse "https://exa mple.com/": invalid character " " in host name`},
		{name: "a bad escape", link: `<A HREF="https://example.com/%zz">Title</A>`, url: "https://example.com/%zz", reason: `the URL "https://example.com/%zz" is no URL: parse "https://example.com/%zz": invalid URL escape "%zz"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := parseString(t, "<!DOCTYPE NETSCAPE-Bookmark-file-1><DL>"+test.link+"</DL>")
			if len(doc.Bookmarks) != 0 || len(doc.Refusals) != 1 {
				t.Fatalf("the document = %d bookmarks, %d refusals; want 0 and 1", len(doc.Bookmarks), len(doc.Refusals))
			}
			got := doc.Refusals[0]
			if got.URL != test.url || got.Reason != test.reason || got.Title != "Title" {
				t.Errorf("the refusal = %+v, want url %q, reason %q", got, test.url, test.reason)
			}
		})
	}
}

// TestURLs checks the normal form of an accepted URL.
func TestURLs(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// href is the attribute's text.
		href string
		// want is the normalized URL.
		want string
	}{
		{name: "the host in capitals", href: "HTTPS://EXAMPLE.com/Path?Q=A", want: "https://example.com/Path?Q=A"},
		{name: "a fragment", href: "https://example.com/a#b", want: "https://example.com/a"},
		{name: "white space", href: "  http://example.org/x  ", want: "http://example.org/x"},
		{name: "a port and a user", href: "http://u@Example.net:8080/", want: "http://u@example.net:8080/"},
		{name: "a character reference", href: "https://example.com/?a=1&amp;b=2", want: "https://example.com/?a=1&b=2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := parseString(t, `<!DOCTYPE NETSCAPE-Bookmark-file-1><A HREF="`+test.href+`">x</A>`)
			if len(doc.Bookmarks) != 1 || doc.Bookmarks[0].URL != test.want {
				t.Errorf("the bookmarks = %+v, want 1 with the URL %q", doc.Bookmarks, test.want)
			}
		})
	}
}

// TestTitleAndNotesLimits checks the cut of a title and of notes at a character
// boundary.
func TestTitleAndNotesLimits(t *testing.T) {
	// "é" is 2 bytes, so 1025 of them are 2050 bytes, and the cut at 2048 bytes is
	// 1024 of them; a 1-byte prefix moves the boundary, so that the cut lands in
	// the middle of a character.
	title := "a" + strings.Repeat("é", 1025)
	notes := "b" + strings.Repeat("é", 32800)
	doc := parseString(t, "<!DOCTYPE NETSCAPE-Bookmark-file-1><DT><A HREF=\"https://example.com/a\">"+title+"</A><DD>"+notes+"\n<DT><A HREF=\"https://example.com/b\">"+strings.Repeat("c", 3000)+"</A>")
	if len(doc.Bookmarks) != 2 {
		t.Fatalf("the document has %d bookmarks, want 2", len(doc.Bookmarks))
	}
	first := doc.Bookmarks[0]
	if want := "a" + strings.Repeat("é", 1023); first.Title != want {
		t.Errorf("the title has %d bytes, want %d (cut at a character boundary below 2048)", len(first.Title), len(want))
	}
	if want := "b" + strings.Repeat("é", 32767); first.Notes != want {
		t.Errorf("the notes have %d bytes, want %d (cut at a character boundary below 65536)", len(first.Notes), len(want))
	}
	if got := len(doc.Bookmarks[1].Title); got != maxTitleLen {
		t.Errorf("the ASCII title has %d bytes, want %d", got, maxTitleLen)
	}
}

// TestTags checks the rules of the TAGS attribute.
func TestTags(t *testing.T) {
	var many []string
	for i := range 70 {
		many = append(many, fmt.Sprintf("t%02d", i))
	}
	long := strings.Repeat("x", 80)
	tests := []struct {
		// name names the case.
		name string
		// value is the attribute.
		value string
		// want are the tags.
		want []string
	}{
		{name: "none", value: "", want: nil},
		{name: "a list", value: "go,sqlite", want: []string{"go", "sqlite"}},
		{name: "spaces", value: " Reading  List ,  to\tread ", want: []string{"reading-list", "to-read"}},
		{name: "upper case", value: "Go,GO,gO", want: []string{"go"}},
		{name: "repeats", value: "a,b,a,c,b", want: []string{"a", "b", "c"}},
		{name: "empty entries", value: ",a,, ,b,", want: []string{"a", "b"}},
		{name: "control characters", value: "a\x01b,\x02,\x03 c", want: []string{"ab", "c"}},
		{name: "too long", value: long + ",ok", want: []string{strings.Repeat("x", 64), "ok"}},
		{name: "characters, not bytes", value: strings.Repeat("é", 70), want: []string{strings.Repeat("é", 64)}},
		{name: "more than 64", value: strings.Join(many, ","), want: many[:64]},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := bookmarkTags(test.value); !reflect.DeepEqual(got, test.want) {
				t.Errorf("bookmarkTags(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

// TestAddDate checks the time of ADD_DATE.
func TestAddDate(t *testing.T) {
	if got, want := addDate("1700000000"), time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC); !got.Equal(want) {
		t.Errorf("addDate = %v, want %v", got, want)
	}
	if got := addDate("0"); !got.IsZero() {
		t.Errorf("addDate(0) = %v, want the zero time", got)
	}
}

// TestPathsDoNotAlias checks that the folders of 2 bookmarks do not share the
// memory of their paths.
func TestPathsDoNotAlias(t *testing.T) {
	doc := parseString(t, `<!DOCTYPE NETSCAPE-Bookmark-file-1><DL><DT><H3>A</H3><DL><DT><A HREF="https://example.com/1">1</A></DL><DT><H3>B</H3><DL><DT><A HREF="https://example.com/2">2</A></DL></DL>`)
	if len(doc.Bookmarks) != 2 {
		t.Fatalf("the document has %d bookmarks, want 2", len(doc.Bookmarks))
	}
	if got := doc.Bookmarks[0].Path; !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("the path of the first bookmark = %q, want [A]", got)
	}
	if got := doc.Bookmarks[1].Path; !reflect.DeepEqual(got, []string{"B"}) {
		t.Errorf("the path of the second bookmark = %q, want [B]", got)
	}
}

// TestTooDeep checks the depth limit: MaxLevels open <DL> levels pass, and 1
// more is ErrTooDeep, so that the parser's memory stays linear in the input.
func TestTooDeep(t *testing.T) {
	deep := func(levels int) string {
		return "<!DOCTYPE NETSCAPE-Bookmark-file-1>" + strings.Repeat("<DT><H3>a</H3><DL><p>", levels)
	}
	if _, err := Parse(strings.NewReader(deep(MaxLevels))); err != nil {
		t.Errorf("Parse of %d levels = %v, want no error", MaxLevels, err)
	}
	if _, err := Parse(strings.NewReader(deep(MaxLevels + 1))); !errors.Is(err, ErrTooDeep) {
		t.Errorf("Parse of %d levels = %v, want ErrTooDeep", MaxLevels+1, err)
	}
}
