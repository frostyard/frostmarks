package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// ownModule is frostmarks' own module path. Any package under it is frostmarks'
// own code.
const ownModule = "github.com/frostyard/frostmarks"

// hearthModule is the 1 other frostyard module that frostmarks builds on: the
// toolkit of frostyard/core ADR-0085.
const hearthModule = "github.com/frostyard/hearth"

// refusedPrefixes are the import prefixes that frostmarks must not use: the
// owner's other repositories and Pocket (frostyard/core ADR-0085: an app on
// hearth works with no lodge, and frostmarks imports no lodge, frostyard app or
// pi-pocket code). Any other module under github.com/frostyard/ is refused as
// well; hearth is the only exception.
var refusedPrefixes = []string{
	"github.com/bketelsen/",
	"github.com/earendil-works/",
}

// refused reports whether a module path or an import path belongs to a
// repository that frostmarks must not use.
func refused(path string) bool {
	if path == ownModule || strings.HasPrefix(path, ownModule+"/") {
		return false
	}
	if path == hearthModule || strings.HasPrefix(path, hearthModule+"/") {
		return false
	}
	if strings.HasPrefix(path, "github.com/frostyard/") {
		return true
	}
	for _, prefix := range refusedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// goModProblems returns each module that 1 go.mod's text requires and
// frostmarks must not use.
func goModProblems(text string) []string {
	var (
		problems  []string
		inRequire bool
	)
	for line := range strings.SplitSeq(text, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case line == "require (":
			inRequire = true
			continue
		case inRequire && line == ")":
			inRequire = false
			continue
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) >= 2 && fields[0] == "require":
			problems = refusedIn(problems, fields[1], "go.mod")
		case inRequire && len(fields) >= 1:
			problems = refusedIn(problems, fields[0], "go.mod")
		}
	}
	return problems
}

// goSumProblems returns each module that 1 go.sum's text pins and frostmarks
// must not use.
func goSumProblems(text string) []string {
	var problems []string
	for line := range strings.SplitSeq(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			problems = refusedIn(problems, fields[0], "go.sum")
		}
	}
	return problems
}

// refusedIn adds 1 problem when path is refused.
func refusedIn(problems []string, path, file string) []string {
	if refused(path) {
		return append(problems, file+": "+path+" is not frostmarks' code, and frostmarks imports no lodge, frostyard app or Pocket module")
	}
	return problems
}

// importSpec finds 1 quoted import path of a Go import statement.
var importSpec = regexp.MustCompile(`"([^"]+)"`)

// goImportProblems returns each import of 1 Go file that frostmarks must not
// use.
func goImportProblems(src string) []string {
	var (
		problems []string
		inBlock  bool
	)
	for line := range strings.SplitSeq(src, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case inBlock && strings.HasPrefix(line, ")"):
			inBlock = false
			continue
		case strings.HasPrefix(line, "import ("):
			inBlock = true
			continue
		case strings.HasPrefix(line, "import "):
			for _, m := range importSpec.FindAllStringSubmatch(line, -1) {
				problems = refusedIn(problems, m[1], "the import")
			}
			continue
		}
		if !inBlock {
			continue
		}
		for _, m := range importSpec.FindAllStringSubmatch(line, -1) {
			problems = refusedIn(problems, m[1], "the import")
		}
	}
	return problems
}

// protoImport finds 1 import statement of a .proto file.
var protoImport = regexp.MustCompile(`^\s*import\s+(?:public\s+|weak\s+)?"([^"]+)"`)

// protoOptions is the import path of hearth's options file, the 1 file of
// another repository that the app's own Protobuf files may import. It is the
// copy under third_party/, which a test of this package keeps equal to hearth's
// file.
const protoOptions = "frostyard/hearth/v1/options.proto"

// protoImportProblems returns each import of 1 .proto file that is neither a
// google/protobuf file, hearth's options file, nor 1 of the repository's own
// Protobuf files.
func protoImportProblems(own map[string]bool, src string) []string {
	var problems []string
	for line := range strings.SplitSeq(src, "\n") {
		m := protoImport.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		path := m[1]
		if strings.HasPrefix(path, "google/protobuf/") || path == protoOptions || own[path] {
			continue
		}
		problems = append(problems, "the proto imports "+path+", which is neither google/protobuf, hearth's options file, nor 1 of the repository's own files")
	}
	return problems
}

// protoOwnPaths returns the import paths of the repository's own Protobuf
// files: each file's path from the repository root, and its path from the
// module root that holds it (a "proto" directory).
func protoOwnPaths(files []string) map[string]bool {
	out := make(map[string]bool, 2*len(files))
	for _, file := range files {
		out[file] = true
		for _, root := range []string{"proto/", "/proto/"} {
			if i := strings.LastIndex(file, root); i >= 0 {
				out[file[i+len(root):]] = true
			}
		}
	}
	return out
}

// repoFilesOf returns every file of the repository whose name matches keep.
func repoFilesOf(t *testing.T, keep func(name string) bool) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !keep(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// TestImportBoundary checks that frostmarks imports no lodge, frostyard app or
// Pocket code, in Go or in Protobuf (frostyard/core ADR-0085).
func TestImportBoundary(t *testing.T) {
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var problems []string
		if name == "go.mod" {
			problems = goModProblems(string(data))
		} else {
			problems = goSumProblems(string(data))
		}
		for _, problem := range problems {
			t.Errorf("%s: %s", name, problem)
		}
	}

	goFiles := 0
	for _, file := range repoFilesOf(t, func(name string) bool { return strings.HasSuffix(name, ".go") }) {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		goFiles++
		for _, problem := range goImportProblems(string(data)) {
			t.Errorf("%s: %s", file, problem)
		}
	}
	if goFiles == 0 {
		t.Fatal("no Go file checked")
	}

	var protos []string
	for _, file := range repoFilesOf(t, func(name string) bool { return strings.HasSuffix(name, ".proto") }) {
		// Only the app's own module is held to the rule: third_party/ is
		// hearth's file, and it has hearth's own import path.
		if strings.HasPrefix(file, "proto/") {
			protos = append(protos, file)
		}
	}
	if len(protos) == 0 {
		t.Fatal("no .proto file under proto/ checked")
	}
	own := protoOwnPaths(protos)
	for _, file := range protos {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range protoImportProblems(own, string(data)) {
			t.Errorf("%s: %s", file, problem)
		}
	}
	t.Logf("%d Go file(s), %d .proto file(s), %d own import path(s)", goFiles, len(protos), len(own))
}

// TestImportRules checks the rules of the import boundary on in-memory files.
func TestImportRules(t *testing.T) {
	for path, want := range map[string]bool{
		"github.com/frostyard/frostmarks":              false,
		"github.com/frostyard/frostmarks/marks":        false,
		"github.com/frostyard/hearth":                  false,
		"github.com/frostyard/hearth/authz":            false,
		"github.com/frostyard/hearth/mcpserver":        false,
		"github.com/frostyard/lodge/kit":               true,
		"github.com/frostyard/lodge":                   true,
		"github.com/frostyard/frostnotes/app":          true,
		"github.com/frostyard/porter":                  true,
		"github.com/bketelsen/clix":                    true,
		"github.com/earendil-works/pocket":             true,
		"github.com/santhosh-tekuri/jsonschema/v6":     false,
		"google.golang.org/protobuf/compiler/protogen": false,
		"github.com/frostyard/hearthx":                 true, // another frostyard module
		"github.com/frostyard/frostmarksx":             true, // another frostyard module
		"example.com/marks":                            false,
		"github.com/frostyard/":                        true,
	} {
		if got := refused(path); got != want {
			t.Errorf("refused(%q) = %v, want %v", path, got, want)
		}
	}

	for _, tc := range []struct {
		name     string
		goMod    string
		problems int
	}{
		{"frostmarks and hearth", "module github.com/frostyard/frostmarks\n\ngo 1.27.4\n\nrequire (\n\tgithub.com/frostyard/hearth v0.1.0\n\tgoogle.golang.org/protobuf v1.36.12\n)\n", 0},
		{"a refused module", "module github.com/frostyard/frostmarks\n\nrequire github.com/frostyard/lodge/kit v0.3.0\n", 1},
		{"a refused block module", "module github.com/frostyard/frostmarks\n\nrequire (\n\tgithub.com/bketelsen/clix v0.1.0\n\tgithub.com/frostyard/lodge/kit v0.3.0\n)\n", 2},
	} {
		if got := len(goModProblems(tc.goMod)); got != tc.problems {
			t.Errorf("%s: goModProblems found %d problem(s), want %d", tc.name, got, tc.problems)
		}
	}

	if got := len(goSumProblems("github.com/frostyard/lodge/kit v0.3.0 h1:abc=\n")); got != 1 {
		t.Errorf("goSumProblems found %d problem(s), want 1", got)
	}

	for _, tc := range []struct {
		name     string
		src      string
		problems int
	}{
		{"a plain import", "package a\n\nimport \"github.com/frostyard/hearth/authz\"\n", 0},
		{"an own import", "package a\n\nimport \"github.com/frostyard/frostmarks/marks\"\n", 0},
		{"a refused import", "package a\n\nimport \"github.com/frostyard/lodge/kit\"\n", 1},
		{"a refused import in a block", "package a\n\nimport (\n\t\"fmt\"\n\n\t\"github.com/earendil-works/pocket\"\n)\n", 1},
		{"a string that is no import", "package a\n\nconst url = \"github.com/frostyard/lodge\"\n", 0},
	} {
		if got := len(goImportProblems(tc.src)); got != tc.problems {
			t.Errorf("%s: goImportProblems found %d problem(s), want %d", tc.name, got, tc.problems)
		}
	}

	own := protoOwnPaths([]string{"proto/frostyard/frostmarks/v1/marks.proto"})
	for _, tc := range []struct {
		name     string
		src      string
		problems int
	}{
		{"hearth's options", "import \"frostyard/hearth/v1/options.proto\";\n", 0},
		{"a standard file", "import \"google/protobuf/timestamp.proto\";\n", 0},
		{"a public import of the options", "import public \"frostyard/hearth/v1/options.proto\";\n", 0},
		{"an own file", "import \"frostyard/frostmarks/v1/marks.proto\";\n", 0},
		{"the path from the repository root", "import \"proto/frostyard/frostmarks/v1/marks.proto\";\n", 0},
		{"a file of another repository", "import \"lodge/kit/v1/app.proto\";\n", 1},
		{"a hearth file that is no option", "import \"frostyard/hearth/v1/other.proto\";\n", 1},
	} {
		if got := len(protoImportProblems(own, tc.src)); got != tc.problems {
			t.Errorf("%s: protoImportProblems found %d problem(s), want %d", tc.name, got, tc.problems)
		}
	}
}
