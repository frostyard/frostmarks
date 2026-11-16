package workflowcheck

import (
	"encoding/json/v2"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestHooksAreRiskPaths guards .github/risk-paths.json against a new permission
// hook: 1 hook decides who may read or change 1 stored bookmark or folder, so
// the file that declares it must be a risk path at L1 or L2 (frostyard/core
// ADR-0068). marks/authorize.go holds the 4 hooks and the grant filters.
func TestHooksAreRiskPaths(t *testing.T) {
	cfg := readRiskPaths(t)
	files := hookGoFiles(t)
	problems := hookRiskProblems(cfg, files)
	for _, problem := range problems {
		t.Error(problem)
	}
	hooks := 0
	for _, src := range files {
		names, err := authorizeHooks(src)
		if err != nil {
			t.Fatal(err)
		}
		hooks += len(names)
	}
	switch {
	case hooks > 0:
		// Every file that declares a hook is a risk path, which the loop
		// above checks.
	case hookFilePresent(t):
		t.Fatal("marks/authorize.go declares no method whose name starts with Authorize, so this test guards nothing: the 4 hooks check a stored bookmark and a stored folder")
	default:
		// The file lands in the app's own change; this test guards the hook
		// from the first file on.
		t.Log("marks/authorize.go is not in the tree yet, so there is no permission hook to guard")
	}
	t.Logf("%d hand-written Go file(s), %d permission hook(s)", len(files), hooks)
}

// hookFilePresent reports whether the app's file of permission hooks is in the
// tree. When it is not, no permission hook exists to guard yet.
func hookFilePresent(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join("..", "..", "marks", "authorize.go"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("marks/authorize.go: %v", err)
	}
	return err == nil
}

// TestHookRiskProblems checks the hook rule on in-memory files and lists.
func TestHookRiskProblems(t *testing.T) {
	hook := []byte("package marks\n\n// AuthorizeGetBookmark checks 1 bookmark.\nfunc (s *Service) AuthorizeGetBookmark() {}\n")
	plain := []byte("package marks\n\n// GetBookmark reads 1 bookmark.\nfunc (s *Service) GetBookmark() {}\n")
	files := map[string][]byte{
		"marks/authorize.go": hook,
		"marks/service.go":   plain,
	}
	entry := func(glob, level string) riskPath {
		return riskPath{Glob: glob, Level: level, Why: "permission-checks: the permission hooks and the grant filters"}
	}
	for _, tc := range []struct {
		name string
		cfg  riskPathFile
		want int
	}{
		{
			name: "an entry at L1 for the file",
			cfg:  riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: []riskPath{entry("marks/authorize.go", "L1")}},
		},
		{
			name: "an entry at L2 for a directory",
			cfg:  riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: []riskPath{entry("marks/**", "L2")}},
		},
		{
			name: "no entry for the file",
			cfg:  riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: []riskPath{entry("marks/service.go", "L2")}},
			want: 1,
		},
		{
			name: "an entry at L3 only",
			cfg:  riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: []riskPath{entry("marks/**", "L3")}},
			want: 1,
		},
		{
			name: "no entry at all",
			cfg:  riskPathFile{SchemaVersion: 2, DefaultLevel: "L3"},
			want: 1,
		},
	} {
		problems := hookRiskProblems(tc.cfg, files)
		if len(problems) != tc.want {
			t.Errorf("%s: hookRiskProblems = %v, want %d problem(s)", tc.name, problems, tc.want)
			continue
		}
		if tc.want == 1 && !strings.Contains(problems[0], "AuthorizeGetBookmark") {
			t.Errorf("%s: the problem = %q, want it to name the hook", tc.name, problems[0])
		}
	}
}

// hookRiskProblems returns 1 problem for each Go file that declares a method
// whose name starts with "Authorize", and that no risk-path entry at L1 or L2
// matches.
func hookRiskProblems(cfg riskPathFile, files map[string][]byte) []string {
	var problems []string
	for _, path := range slices.Sorted(maps.Keys(files)) {
		names, err := authorizeHooks(files[path])
		if err != nil {
			problems = append(problems, path+": "+err.Error())
			continue
		}
		if len(names) == 0 || coveredAtL1OrL2(cfg, path) {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s declares the permission hook %s, and no entry at L1 or L2 of .github/risk-paths.json matches it: add an entry for %s, or for the directory that holds it",
			path, strings.Join(names, ", "), path))
	}
	return problems
}

// coveredAtL1OrL2 reports whether 1 risk-path entry at L1 or L2 matches path.
func coveredAtL1OrL2(cfg riskPathFile, path string) bool {
	for _, entry := range cfg.Paths {
		if entry.Level != "L1" && entry.Level != "L2" {
			continue
		}
		if matchGlob(entry.Glob, path) {
			return true
		}
	}
	return false
}

// authorizeHooks returns each method name of 1 Go file's source that starts with
// "Authorize": the permission hook of 1 RPC on an object id.
func authorizeHooks(src []byte) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "1.go", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || !strings.HasPrefix(fn.Name.Name, "Authorize") {
			continue
		}
		out = append(out, fn.Name.Name)
	}
	return out, nil
}

// hookGoFiles returns each hand-written Go file of the repository, as its
// source, by its path from the repository root. A generated file (under gen/)
// and a test file are left out.
func hookGoFiles(t *testing.T) map[string][]byte {
	t.Helper()
	root := filepath.Join("..", "..")
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "gen" || skipped(d.Name(), nil) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// readRiskPaths reads .github/risk-paths.json.
func readRiskPaths(t *testing.T) riskPathFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "risk-paths.json"))
	if err != nil {
		t.Fatalf(".github/risk-paths.json: %v", err)
	}
	var cfg riskPathFile
	if err := json.Unmarshal(data, &cfg, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf(".github/risk-paths.json: %v", err)
	}
	return cfg
}
