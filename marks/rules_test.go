package marks

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// TestURLRules checks the URL rule: http and https only, a lower-case scheme and
// host, no fragment, the path as it was given, and the length limit.
func TestURLRules(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", maxURLLen)
	tests := []struct {
		// name names the case.
		name string
		// in is the URL of the request.
		in string
		// want is the canonical URL, when the request passes.
		want string
	}{
		{name: "https", in: "https://example.com/a", want: "https://example.com/a"},
		{name: "http", in: "http://example.com/a", want: "http://example.com/a"},
		{name: "upper case scheme and host", in: "HTTPS://Example.COM/a", want: "https://example.com/a"},
		{name: "path case is kept", in: "https://example.com/Read/Me", want: "https://example.com/Read/Me"},
		{name: "spaces are trimmed", in: "  https://example.com/a  ", want: "https://example.com/a"},
		{name: "the fragment is dropped", in: "https://example.com/a#section", want: "https://example.com/a"},
		{name: "the query is kept", in: "https://example.com/a?q=1&r=2", want: "https://example.com/a?q=1&r=2"},
		{name: "the port is kept", in: "https://example.com:8443/a", want: "https://example.com:8443/a"},
		{name: "javascript is refused", in: "javascript:alert(1)"},
		{name: "ftp is refused", in: "ftp://example.com/a"},
		{name: "mailto is refused", in: "mailto:user@example.com"},
		{name: "no host", in: "https:///a"},
		{name: "no scheme", in: "example.com/a"},
		{name: "empty", in: ""},
		{name: "too long", in: long},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizedURL(test.in)
			if test.want == "" {
				connectErr := wantCode(t, err, connect.CodeInvalidArgument)
				if connectErr.Message() == "" {
					t.Error("the refusal has no text")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizedURL(%q): %v", test.in, err)
			}
			if got != test.want {
				t.Errorf("normalizedURL(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

// TestTagRules checks the tag rule: trimmed, lower-cased, sorted, without a
// repeat, 1 to 64 characters, and no white space, comma or control character.
func TestTagRules(t *testing.T) {
	app := app(t)
	mustCreateFolder(t, app, "/work")

	t.Run("a tag is lower-cased", func(t *testing.T) {
		got, err := normalizedTag("  GoLang ")
		if err != nil {
			t.Fatalf("normalizedTag: %v", err)
		}
		if got != "golang" {
			t.Errorf("normalizedTag(%q) = %q, want golang", "  GoLang ", got)
		}
	})

	t.Run("repeats collapse and the tags are sorted", func(t *testing.T) {
		bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
			Url:    "https://example.com/a",
			Folder: "/work",
			Tags:   []string{"Beta", "alpha", "alpha", "BETA"},
		})
		want := []string{"alpha", "beta"}
		if got := bookmark.GetTags(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("the tags = %v, want %v", got, want)
		}
	})

	t.Run("a 64-character tag passes", func(t *testing.T) {
		tag := strings.Repeat("é", maxTagRunes)
		got, err := normalizedTag(tag)
		if err != nil {
			t.Fatalf("normalizedTag of %d runes: %v", maxTagRunes, err)
		}
		if len([]rune(got)) != maxTagRunes {
			t.Errorf("the canonical tag holds %d runes, want %d", len([]rune(got)), maxTagRunes)
		}
	})

	refused := map[string]string{
		"empty":             "",
		"white space only":  "   ",
		"65 characters":     strings.Repeat("a", maxTagRunes+1),
		"space inside":      "two words",
		"comma":             "a,b",
		"control character": "a\x00b",
	}
	for name, tag := range refused {
		t.Run("refused: "+name, func(t *testing.T) {
			_, err := normalizedTag(tag)
			connectErr := wantCode(t, err, connect.CodeInvalidArgument)
			if !strings.HasPrefix(connectErr.Message(), "invalid tag ") {
				t.Errorf("the refusal = %q, want the text invalid tag \"...\": <reason>", connectErr.Message())
			}
		})
	}

	t.Run("add and remove", func(t *testing.T) {
		bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
			Url:    "https://example.com/tags",
			Folder: "/work",
			Tags:   []string{"reading", "go"},
		})
		url, title := bookmark.GetUrl(), "a title"
		response, err := app.API().UpdateBookmark(local(t), &marksv1.UpdateBookmarkRequest{
			Id:         bookmark.GetId(),
			Url:        &url,
			Title:      &title,
			AddTags:    []string{"SQLite"},
			RemoveTags: []string{"go"},
		})
		if err != nil {
			t.Fatalf("UpdateBookmark: %v", err)
		}
		want := []string{"reading", "sqlite"}
		got := response.GetBookmark().GetTags()
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("the tags = %v, want %v", got, want)
		}
	})

	t.Run("a tag in both lists is refused", func(t *testing.T) {
		bookmark := mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
			Url:    "https://example.com/both",
			Folder: "/work",
		})
		before := count(t, app, "bookmark_tags")
		_, err := app.API().UpdateBookmark(local(t), &marksv1.UpdateBookmarkRequest{
			Id:         bookmark.GetId(),
			AddTags:    []string{"go"},
			RemoveTags: []string{"GO"},
		})
		wantCode(t, err, connect.CodeInvalidArgument)
		if got := count(t, app, "bookmark_tags"); got != before {
			t.Errorf("the bookmark_tags table holds %d rows after a refusal, want the %d of before", got, before)
		}
	})

	t.Run("list by tag", func(t *testing.T) {
		mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
			Url:    "https://example.com/one",
			Folder: "/work",
			Tags:   []string{"news"},
		})
		mustCreateBookmark(t, app, &marksv1.CreateBookmarkRequest{
			Url:    "https://example.com/two",
			Folder: "/work",
			Tags:   []string{"news", "reading"},
		})
		response, err := app.API().ListBookmarks(local(t), &marksv1.ListBookmarksRequest{Folder: "/work", Tag: "NEWS"})
		if err != nil {
			t.Fatalf("ListBookmarks: %v", err)
		}
		if len(response.GetBookmarks()) != 2 {
			t.Errorf("the list of the tag news = %d bookmarks, want 2", len(response.GetBookmarks()))
		}
	})
}

// TestFolderPathRules checks the folder rule: the canonical form of a path, the
// root for an absent value, and invalid_argument for a path that is no folder
// path.
func TestFolderPathRules(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// in is the folder value of the request.
		in string
		// want is the canonical path, when the value passes.
		want string
	}{
		{name: "absent is the root", in: "", want: "/"},
		{name: "the root", in: "/", want: "/"},
		{name: "1 segment", in: "/work", want: "/work"},
		{name: "2 segments", in: "/work/2026", want: "/work/2026"},
		{name: "NFC", in: "/cafe\u0301", want: "/café"},
		{name: "no leading slash", in: "work"},
		{name: "trailing slash", in: "/work/"},
		{name: "empty segment", in: "/a//b"},
		{name: "dot segment", in: "/a/./b"},
		{name: "dot dot segment", in: "/a/../b"},
		{name: "too long", in: "/" + strings.Repeat("a", maxFolderLen)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := folderOf(test.in)
			if test.want == "" {
				wantCode(t, err, connect.CodeInvalidArgument)
				return
			}
			if err != nil {
				t.Fatalf("folderOf(%q): %v", test.in, err)
			}
			if got != test.want {
				t.Errorf("folderOf(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}
