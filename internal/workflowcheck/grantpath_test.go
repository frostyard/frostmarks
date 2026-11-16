package workflowcheck

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// wildcardPath matches a code span that holds a folder path with a `*`: a
// grant path is a plain folder path, and hearth's authz matches it by equality
// and by the `/` that follows it, so `/reading/**` is a literal segment `**`
// and covers no folder below `/reading`.
var wildcardPath = regexp.MustCompile("`/[^`\\s]*\\*[^`]*`")

// grantPathProblems returns 1 problem for each Markdown file in fsys (under
// docs/ and site/content/, and AGENTS.md and README.md) that writes a folder
// path with a wildcard.
func grantPathProblems(fsys fs.FS) []string {
	var problems []string
	check := func(name string) {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			return
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, span := range wildcardPath.FindAllString(line, -1) {
				problems = append(problems, name+":"+strconv.Itoa(i+1)+": the folder path "+span+
					" holds a wildcard, but a grant path is a plain folder that covers itself and every folder below it")
			}
		}
	}
	for _, name := range []string{"AGENTS.md", "README.md"} {
		if _, err := fs.Stat(fsys, name); err == nil {
			check(name)
		}
	}
	for _, root := range []string{"docs", "site/content"} {
		if _, err := fs.Stat(fsys, root); err != nil {
			continue
		}
		_ = fs.WalkDir(fsys, root, func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				problems = append(problems, name+": "+err.Error())
				return nil
			}
			if !d.IsDir() && path.Ext(name) == ".md" {
				check(name)
			}
			return nil
		})
	}
	slices.Sort(problems)
	return problems
}

// TestNoWildcardGrantPaths guards the docs of the grant: a folder path in the
// docs and the site has no wildcard, because hearth's authz has none.
func TestNoWildcardGrantPaths(t *testing.T) {
	for _, problem := range grantPathProblems(os.DirFS(filepath.Join("..", ".."))) {
		t.Error(problem)
	}
}

// TestGrantPathProblems checks the rule on fixtures that pass and fail.
func TestGrantPathProblems(t *testing.T) {
	for _, tc := range []struct {
		name, file, text string
		want             int
	}{
		{"a plain path", "site/content/guides/a.md", "| path | `/reading` | covers it |\n", 0},
		{"the root", "docs/specs/a.md", "`/` covers every folder\n", 0},
		{"a repository glob", "AGENTS.md", "`gen/**` and `third_party/**` are generated\n", 0},
		{"a double star", "site/content/guides/a.md", "| path | `/reading/**` | covers it |\n", 1},
		{"a single star", "docs/design/a.md", "a grant on `/reading/*`\n", 1},
		{"a star in the README", "README.md", "grant `/a*`\n", 1},
		{"a text file", "docs/a.txt", "`/reading/**`\n", 0},
	} {
		fsys := fstest.MapFS{tc.file: {Data: []byte(tc.text)}}
		if got := grantPathProblems(fsys); len(got) != tc.want {
			t.Errorf("%s: grantPathProblems = %q, want %d problem(s)", tc.name, got, tc.want)
		}
	}
}
