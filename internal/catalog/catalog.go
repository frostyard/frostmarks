// Package catalog writes frostmarks' tool catalog, tools.json: 1 tool for each
// of the app's agent operations, in the shape of an MCP tools/list result, which
// lodge reads to show what an agent may call (lodge's
// docs/specs/app-contract.md, Apps on hearth; and docs/specs/lodge-app.md).
//
// The tools are the app's own, which hearth's protoc-gen-hearth-mcp generated:
// their names, their descriptions and their schemas come from the proto, so the
// catalog cannot drift from the API.
package catalog

import (
	"encoding/json"
	"fmt"

	"github.com/frostyard/hearth/mcpserver"
)

// tool is 1 tool of the catalog: an MCP tool, without the hints that the hearth
// manifest's operations give.
type tool struct {
	// Name is the tool's name, which the manifest's operation names too.
	Name string `json:"name"`
	// Description tells an agent when to call the tool.
	Description string `json:"description"`
	// InputSchema is the JSON Schema of the call's arguments.
	InputSchema json.RawMessage `json:"inputSchema"`
	// OutputSchema is the JSON Schema of the call's result, when the tool has
	// 1.
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}

// catalog is 1 tools/list result: the app's tools, in the order of the generated
// ones.
type catalog struct {
	// Tools are the app's tools.
	Tools []tool `json:"tools"`
}

// Tools returns the tool catalog of an app's tools: its tools in the order
// given, as 1 JSON object that ends with 1 newline, so that the same code writes
// the same file.
func Tools(tools []mcpserver.Tool) ([]byte, error) {
	out := catalog{Tools: make([]tool, 0, len(tools))}
	for _, t := range tools {
		out.Tools = append(out.Tools, tool{
			Name:         t.Name,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
		})
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("write the tool catalog: %w", err)
	}
	return append(data, '\n'), nil
}
