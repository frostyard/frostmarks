package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// version is the version that the tests stage.
const version = "0.1.0"

// The paths of the install tree, under ROOT/usr.
var (
	// binaryPaths are the app's 2 binaries.
	binaryPaths = []string{"bin/frostmarksd", "bin/marksctl"}
	// filePaths are the files that the stage writes with 0644.
	filePaths = []string{
		"share/lodge/apps/marks/app.json",
		"share/lodge/apps/marks/manifest.json",
		"share/lodge/apps/marks/tools.json",
		"share/lodge/apps/marks/web/index.html",
		"share/lodge/apps/marks/web/icon.svg",
		"lib/systemd/system/frostmarks.service",
		"lib/systemd/system/frostmarks.socket",
		"lib/sysusers.d/frostmarks.conf",
	}
	// dirPaths are the directories that the stage makes.
	dirPaths = []string{
		"bin",
		"share",
		"share/lodge",
		"share/lodge/apps",
		"share/lodge/apps/marks",
		"share/lodge/apps/marks/web",
		"lib",
		"lib/systemd",
		"lib/systemd/system",
		"lib/sysusers.d",
	}
)

// moduleRoot returns the repository's root: the directory of its go.mod. The
// stage reads the repository's own files, so the test runs there.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("read the working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// fixtures makes a binary directory and a web directory, and it returns them.
func fixtures(t *testing.T) (binDir, webDir string) {
	t.Helper()
	binDir = filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("make the binary directory: %v", err)
	}
	for _, name := range binaries {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n# "+name+"\n"), 0o755); err != nil {
			t.Fatalf("write the binary %s: %v", name, err)
		}
	}
	webDir = filepath.Join(t.TempDir(), "web")
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatalf("make the web directory: %v", err)
	}
	for name, body := range map[string]string{
		"index.html":     "<!doctype html><title>Bookmarks</title>",
		"icon.svg":       `<svg xmlns="http://www.w3.org/2000/svg"><title>Bookmarks</title></svg>`,
		"assets/app.js":  "export const app = 1;\n",
		"assets/app.css": "body { margin: 0 }\n",
	} {
		path := filepath.Join(webDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("make %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return binDir, webDir
}

// TestStage checks the install tree: every path, every mode, the app's manifest,
// and that nothing is written outside usr.
func TestStage(t *testing.T) {
	t.Chdir(moduleRoot(t))
	binDir, webDir := fixtures(t)
	out := t.TempDir()
	if err := stage(version, binDir, webDir, out); err != nil {
		t.Fatalf("stage: %v", err)
	}
	usr := filepath.Join(out, "usr")

	for _, rel := range binaryPaths {
		info, err := os.Stat(filepath.Join(usr, rel))
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
			t.Errorf("the mode of %s = %#o, want %#o", rel, got, want)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", rel)
		}
	}
	for _, rel := range filePaths {
		info, err := os.Stat(filepath.Join(usr, rel))
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
			t.Errorf("the mode of %s = %#o, want %#o", rel, got, want)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", rel)
		}
	}
	for _, rel := range dirPaths {
		info, err := os.Stat(filepath.Join(usr, rel))
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
			t.Errorf("the mode of %s = %#o, want %#o", rel, got, want)
		}
	}

	// The app's manifest keeps its members, and its version is the staged 1.
	data, err := os.ReadFile(filepath.Join(usr, "share/lodge/apps/marks/app.json"))
	if err != nil {
		t.Fatalf("read app.json: %v", err)
	}
	var app map[string]any
	if err := json.Unmarshal(data, &app); err != nil {
		t.Fatalf("read app.json: %v", err)
	}
	for member, want := range map[string]any{
		"contract": float64(1),
		"id":       "marks",
		"user":     "frostmarks",
		"hearth":   "manifest.json",
		"tools":    "tools.json",
		"version":  version,
	} {
		if got := app[member]; got != want {
			t.Errorf("app.json's %s = %v, want %v", member, got, want)
		}
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("app.json does not end with 1 newline")
	}

	// The tool catalog is the app's own.
	catalogData, err := os.ReadFile(filepath.Join(usr, "share/lodge/apps/marks/tools.json"))
	if err != nil {
		t.Fatalf("read tools.json: %v", err)
	}
	if !strings.HasPrefix(string(catalogData), `{"tools":[`) {
		t.Errorf("tools.json starts %q, want a tools/list result", catalogData[:16])
	}

	// Nothing is outside usr.
	var outside []string
	err = filepath.WalkDir(out, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(out, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == "usr" || strings.HasPrefix(rel, "usr/") {
			return nil
		}
		outside = append(outside, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the tree: %v", err)
	}
	if len(outside) > 0 {
		t.Errorf("the stage wrote outside usr: %v", outside)
	}
}

// TestStageRefusals checks what the stage refuses: no version, a web directory
// without its page or its icon, and a symlink in the web directory.
func TestStageRefusals(t *testing.T) {
	t.Chdir(moduleRoot(t))
	binDir, webDir := fixtures(t)
	for _, test := range []struct {
		// name names the case.
		name string
		// version is the version to stage.
		version string
		// web is the directory of the web bundle.
		web string
	}{
		{name: "no version", version: "", web: webDir},
		{name: "no page", version: version, web: without(t, webDir, webIndex)},
		{name: "no icon", version: version, web: without(t, webDir, webIcon)},
		{name: "a symlink in the bundle", version: version, web: withSymlink(t, webDir)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := stage(test.version, binDir, test.web, t.TempDir()); err == nil {
				t.Fatal("stage passed, and it must refuse")
			}
		})
	}
}

// without returns a copy of the web directory dir, less 1 file.
func without(t *testing.T, dir, name string) string {
	t.Helper()
	out := copyWeb(t, dir)
	if err := os.Remove(filepath.Join(out, name)); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	return out
}

// withSymlink returns a copy of the web directory dir, with 1 link in it.
func withSymlink(t *testing.T, dir string) string {
	t.Helper()
	out := copyWeb(t, dir)
	if err := os.Symlink(filepath.Join(out, webIndex), filepath.Join(out, "link.html")); err != nil {
		t.Fatalf("make the link: %v", err)
	}
	return out
}

// copyWeb copies the files of 1 web directory into a new temporary directory.
func copyWeb(t *testing.T, dir string) string {
	t.Helper()
	out := t.TempDir()
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(out, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy the web directory: %v", err)
	}
	return out
}

// TestBinariesAreTheCommands checks that the 2 names of the tree are the
// repository's 2 commands, so that a rename in cmd/ cannot leave the tree with a
// binary of another name.
func TestBinariesAreTheCommands(t *testing.T) {
	root := moduleRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatalf("read cmd/: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, binaries) {
		t.Errorf("cmd/ holds %v, and the tree installs %v", names, binaries)
	}
}
