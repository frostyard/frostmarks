package catalog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
)

// The limits of a tool catalog (lodge's docs/specs/app-contract.md, Apps on
// hearth).
const (
	// maxTools is the most tools a catalog holds.
	maxTools = 128
	// maxNameLen is the longest tool name, in bytes.
	maxNameLen = 47
	// maxDescriptionLen is the longest description, in bytes.
	maxDescriptionLen = 4096
	// maxCatalogLen is the largest catalog, in bytes.
	maxCatalogLen = 1 << 20
)

// namePattern is the pattern of a tool's name.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// manifest is 1 operation of the app's hearth manifest, as far as the catalog
// rules read it.
type manifest struct {
	// Operations are the app's operations.
	Operations []struct {
		// Expose is who reaches the operation.
		Expose []string `json:"expose"`
		// Tool is the operation's tool name, for an agent operation.
		Tool string `json:"tool"`
	} `json:"operations"`
}

// readManifest reads gen/manifest.json.
func readManifest(t *testing.T) manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "gen", "manifest.json"))
	if err != nil {
		t.Fatalf("read the manifest: %v", err)
	}
	var out manifest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("read the manifest: %v", err)
	}
	return out
}

// doc is 1 tool of the written catalog.
type doc struct {
	// Name is the tool's name.
	Name string `json:"name"`
	// Description tells an agent when to call the tool.
	Description string `json:"description"`
	// InputSchema is the schema of the arguments.
	InputSchema map[string]any `json:"inputSchema"`
	// OutputSchema is the schema of the result, or nil.
	OutputSchema map[string]any `json:"outputSchema"`
}

// written is 1 written catalog: its text and its tools.
type written struct {
	// data is the catalog's text.
	data []byte
	// tools are the catalog's tools.
	tools []doc
}

// write returns the catalog of the app's tools, and its tools as JSON.
func write(t *testing.T) written {
	t.Helper()
	data, err := Tools(marksv1.MarksTools(nil))
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	var out struct {
		Tools []doc `json:"tools"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("read the catalog: %v", err)
	}
	return written{data: data, tools: out.Tools}
}

// TestCatalogFollowsLodge checks lodge's rules of a tool catalog against the
// app's own.
func TestCatalogFollowsLodge(t *testing.T) {
	got := write(t)

	if len(got.data) > maxCatalogLen {
		t.Errorf("the catalog is %d bytes, and the limit is %d", len(got.data), maxCatalogLen)
	}
	if !bytes.HasSuffix(got.data, []byte("}\n")) {
		t.Errorf("the catalog does not end with 1 newline: %q", got.data[len(got.data)-5:])
	}
	if len(got.tools) > maxTools {
		t.Errorf("the catalog holds %d tools, and the limit is %d", len(got.tools), maxTools)
	}

	seen := make(map[string]bool, len(got.tools))
	for _, tool := range got.tools {
		if !namePattern.MatchString(tool.Name) {
			t.Errorf("the tool name %q does not match %s", tool.Name, namePattern)
		}
		if len(tool.Name) > maxNameLen {
			t.Errorf("the tool name %q is %d bytes, and the limit is %d", tool.Name, len(tool.Name), maxNameLen)
		}
		if "marks_"+tool.Name == "request_access" {
			t.Errorf("the tool %q is marks_request_access, which lodge refuses", tool.Name)
		}
		if seen[tool.Name] {
			t.Errorf("the tool %q appears twice", tool.Name)
		}
		seen[tool.Name] = true
		if len(tool.Description) > maxDescriptionLen {
			t.Errorf("the description of %q is %d bytes, and the limit is %d", tool.Name, len(tool.Description), maxDescriptionLen)
		}
		checkSchema(t, tool.Name, "inputSchema", tool.InputSchema)
		if tool.OutputSchema != nil {
			checkSchema(t, tool.Name, "outputSchema", tool.OutputSchema)
		}
	}

	// The names are the tool of each operation that an agent reaches, and no
	// other.
	manifest := readManifest(t)
	var want []string
	for _, operation := range manifest.Operations {
		if slices.Contains(operation.Expose, "agent") {
			want = append(want, operation.Tool)
		}
	}
	slices.Sort(want)
	names := make([]string, 0, len(got.tools))
	for _, tool := range got.tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("the catalog's tools = %v, want the agent operations' tools %v", names, want)
	}
}

// checkSchema checks 1 schema of 1 tool: it is a JSON object whose type is
// object.
func checkSchema(t *testing.T, tool, name string, schema map[string]any) {
	t.Helper()
	if schema == nil {
		t.Errorf("the %s of %q is no JSON object", name, tool)
		return
	}
	if got := schema["type"]; got != "object" {
		t.Errorf("the %s of %q has the type %v, want object", name, tool, got)
	}
}

// TestCatalogIsDeterministic checks that 2 writes of the same tools are the same
// bytes, so that a staged file changes only with the tools.
func TestCatalogIsDeterministic(t *testing.T) {
	first := write(t)
	second := write(t)
	if !bytes.Equal(first.data, second.data) {
		t.Error("2 writes of the catalog differ")
	}
	if !strings.HasPrefix(string(first.data), `{"tools":[{"name":"`) {
		t.Errorf("the catalog starts %q, want the tools of a tools/list result", first.data[:24])
	}
}
