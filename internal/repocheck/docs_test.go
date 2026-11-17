package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var (
	// [text](target) and ![alt](target), with an optional "title".
	mdLink = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	// A scheme (https:, mailto:) marks a link this test does not follow.
	scheme      = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	heading     = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	codeSpan    = regexp.MustCompile("`[^`]*`")
	inlineLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	indexedDocs = []string{"adr", "design", "specs", "plans"}
)

// root is the repository's root, from this package's directory.
var root = filepath.Join("..", "..")

// skipDir names the directories no repository rule looks into.
func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "build", "dist", ".worktrees":
		return true
	}
	return false
}

// syncMarker is the file that frostyard/core's skill sync writes in each skill
// directory it manages (core ADR-0026).
const syncMarker = ".synced-from-core"

// managedByCore reports whether dir is a directory that frostyard/core's skill
// sync owns: it holds the sync marker. Its text is core's, and a change goes to
// core, so no rule of this repository looks into it; core's own gate checks it.
func managedByCore(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, syncMarker))
	return err == nil && info.Mode().IsRegular()
}

// skipTree reports whether a walk skips the directory at path: a directory
// that skipDir names, or 1 that frostyard/core manages.
func skipTree(path, name string) bool {
	return skipDir(name) || managedByCore(path)
}

// markdownFiles lists the repository's markdown files, symbolic links
// (CLAUDE.md and the like point at AGENTS.md) and templates excluded.
func markdownFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipTree(path, d.Name()) || d.Name() == "TEMPLATE" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && strings.HasSuffix(path, ".md") && d.Name() != "TEMPLATE.md" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no markdown files found")
	}
	return out
}

// prose returns a markdown file's lines outside fenced code blocks, with code
// spans blanked, so examples in code are not read as links.
func prose(text string) []string {
	var out []string
	fence := ""
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			out = append(out, "")
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			out = append(out, "")
			continue
		}
		out = append(out, codeSpan.ReplaceAllStringFunc(line, func(s string) string {
			return strings.Repeat(" ", len(s))
		}))
	}
	return out
}

// slug is GitHub's anchor for a heading: its text lowercased, with everything
// but letters, digits, marks, connectors, spaces and hyphens dropped, and
// spaces turned into hyphens.
func slug(text string) string {
	text = inlineLink.ReplaceAllString(text, "$1")
	text = strings.ReplaceAll(text, "`", "")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-', unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.Pc):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// anchors returns the anchors of a markdown file's headings, numbering repeats
// as GitHub does (heading, heading-1, …).
func anchors(text string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	fence := ""
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			continue
		}
		m := heading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		s := slug(m[1])
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

// TestRelativeLinksResolve checks that every relative link in the repository's
// markdown names a file or directory that exists, and that a #fragment names a
// heading of its target (AGENTS.md, Documentation rules). Links to other
// repositories are absolute https: URLs, which this test does not fetch.
func TestRelativeLinksResolve(t *testing.T) {
	cache := map[string]map[string]bool{}
	anchorsOf := func(path string) map[string]bool {
		if a, ok := cache[path]; ok {
			return a
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		a := anchors(string(data))
		cache[path] = a
		return a
	}
	links := 0
	for _, file := range markdownFiles(t) {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range prose(string(data)) {
			for _, m := range mdLink.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if scheme.MatchString(target) {
					continue
				}
				links++
				where := file + ":" + strconv.Itoa(i+1)
				if strings.HasPrefix(target, "/") {
					t.Errorf("%s: %q is absolute; use a relative path", where, target)
					continue
				}
				path, fragment, _ := strings.Cut(target, "#")
				dest := file
				if path != "" {
					dest = filepath.Join(filepath.Dir(file), filepath.FromSlash(path))
					if _, err := os.Stat(dest); err != nil {
						t.Errorf("%s: %q does not exist", where, target)
						continue
					}
				}
				if fragment != "" && strings.HasSuffix(dest, ".md") && !anchorsOf(dest)[fragment] {
					t.Errorf("%s: %q names no heading of %s", where, target, filepath.Base(dest))
				}
			}
		}
	}
	if links == 0 {
		t.Fatal("no relative links found")
	}
}

// TestEveryDocIsIndexed checks that every document in docs/ has a line in
// docs/README.md's index (AGENTS.md, Housekeeping).
func TestEveryDocIsIndexed(t *testing.T) {
	readme := filepath.Join(root, "docs", "README.md")
	data, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	linked := map[string]bool{}
	for _, m := range mdLink.FindAllStringSubmatch(string(data), -1) {
		path, _, _ := strings.Cut(m[1], "#")
		linked[filepath.Clean(filepath.FromSlash(path))] = true
	}
	var docs []string
	top, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	docs = append(docs, top...)
	for _, dir := range indexedDocs {
		paths, err := filepath.Glob(filepath.Join(root, "docs", dir, "*.md"))
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, paths...)
	}
	for _, doc := range docs {
		if doc == readme || filepath.Base(doc) == "TEMPLATE.md" {
			continue
		}
		rel, err := filepath.Rel(filepath.Dir(readme), doc)
		if err != nil {
			t.Fatal(err)
		}
		if !linked[rel] {
			t.Errorf("%s has no line in docs/README.md's index", filepath.ToSlash(rel))
		}
	}
}

// TestSlugMatchesGitHub checks the anchor form on the headings this repository
// writes.
func TestSlugMatchesGitHub(t *testing.T) {
	for text, want := range map[string]string{
		"Phase 2 — The lodge app and the save page": "phase-2--the-lodge-app-and-the-save-page",
		"`marksctl search-bookmarks WORDS`":         "marksctl-search-bookmarks-words",
		"The grant's folder path":                   "the-grants-folder-path",
		"Sign-in and browser headers":               "sign-in-and-browser-headers",
		"[ADR-0004](0004-x.md) and caching":         "adr-0004-and-caching",
		"Tokens, logins and nodes":                  "tokens-logins-and-nodes",
	} {
		if got := slug(text); got != want {
			t.Errorf("slug(%q) = %q, want %q", text, got, want)
		}
	}
}

// TestSkipTree checks which directories the repository's walks skip: the
// named build and tool directories, and a skill directory that frostyard/core's
// sync manages, but no directory of this repository's own.
func TestSkipTree(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	synced := mk("synced")
	if err := os.WriteFile(filepath.Join(synced, syncMarker), []byte("Managed by frostyard/core\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	own := mk("own")
	markerDir := mk("marker-dir")
	mk(filepath.Join("marker-dir", syncMarker))
	linked := mk("linked")
	if err := os.Symlink(filepath.Join(synced, syncMarker), filepath.Join(linked, syncMarker)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		want       bool
	}{
		{"a skill that core manages", synced, true},
		{"a directory of this repository", own, false},
		{"a marker that is a directory", markerDir, false},
		{"a marker that is a symbolic link", linked, false},
		{"a .git directory", mk(".git"), true},
		{"a build directory", mk("build"), true},
	} {
		if got := skipTree(tc.path, filepath.Base(tc.path)); got != tc.want {
			t.Errorf("%s: skipTree(%q) = %v, want %v", tc.name, tc.path, got, tc.want)
		}
	}
}
