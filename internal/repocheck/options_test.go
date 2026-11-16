package repocheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// optionsFile is hearth's options file, by its path from hearth's module root
// and from frostmarks' root.
const optionsFile = "frostyard/hearth/v1/options.proto"

// TestVendoredOptions checks that frostmarks' copy of hearth's options file is
// the file of the hearth version that go.mod requires (frostyard/core
// ADR-0085): the app's Protobuf files import the copy, so a difference means
// the app and its plugins disagree about the options. `go list -m` reads the
// module cache, so this test needs no network once the module is there.
func TestVendoredOptions(t *testing.T) {
	vendored := filepath.Join(root, "third_party", filepath.FromSlash(optionsFile))
	copied, err := os.ReadFile(vendored)
	if err != nil {
		t.Fatalf("%s: %v\nFix: copy hearth's file: go list -m -f '{{.Dir}}' %s, then copy proto/%s into third_party/", vendored, err, hearthModule, optionsFile)
	}

	command := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", hearthModule)
	command.Dir = root
	out, err := command.Output()
	if err != nil {
		t.Fatalf("go list -m %s: %v\nFix: run make verify after `go mod download`, which puts the module in the cache", hearthModule, err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Fatalf("go list -m -f '{{.Dir}}' %s printed no directory\nFix: run `go mod download %s`", hearthModule, hearthModule)
	}

	own, err := os.ReadFile(filepath.Join(dir, "proto", filepath.FromSlash(optionsFile)))
	if err != nil {
		t.Fatalf("%s/proto/%s: %v", dir, optionsFile, err)
	}
	if string(own) != string(copied) {
		t.Errorf("third_party/%s differs from %s/proto/%s: copy the file of the hearth version that go.mod requires, and change nothing else:\n\tcp %q third_party/%s",
			optionsFile, dir, optionsFile, filepath.Join(dir, "proto", filepath.FromSlash(optionsFile)), optionsFile)
	}
	t.Logf("third_party/%s equals the file of %s at %s", optionsFile, hearthModule, dir)
}
