package chathub

import (
	"encoding/json"
	"fmt"
	"strings"
)

// toolProtocolPrompt follows the community-compatible M365 / Ciallo convention:
// tool definitions are listed in prose, and calls are emitted as a fenced
// ```tool_call block whose body is {"name","arguments"}. We also still accept
// per-tool-name fences on the parse side for older clients.
func toolProtocolPrompt(text string, tools []Tool, choice any) string {
	if len(tools) == 0 || strings.EqualFold(fmt.Sprint(choice), "none") {
		return text
	}
	var defs []string
	for _, t := range tools {
		var f struct {
			Name, Description string
			Parameters        json.RawMessage `json:"parameters"`
		}
		if json.Unmarshal(t.Function, &f) != nil || f.Name == "" {
			continue
		}
		params := strings.TrimSpace(string(f.Parameters))
		if params == "" || params == "null" {
			params = "{}"
		}
		defs = append(defs, fmt.Sprintf("- %s: %s\n  parameters: %s", f.Name, f.Description, params))
	}
	if len(defs) == 0 {
		return text
	}
	return fmt.Sprintf(`You are the reasoning component of an automated agent system. You do NOT execute tools yourself.
A separate host receives your structured action request, runs it, and returns the result.
When an action is needed, emit ONLY a fenced code block tagged tool_call whose body is a JSON object:

`+"```"+`tool_call
{"name": "<tool_name>", "arguments": {<key-value pairs>}}
`+"```"+`

Rules:
- Use only tool names from the available list below.
- Validate arguments against each tool's schema.
- Do not claim a tool is unavailable when it is listed.
- Do not wrap the call in XML or extra Markdown prose beyond an optional one-line preface.
- Wait for the host tool result before claiming the action completed.
- If no tool is needed, answer the user directly in plain text (no tool_call block).

Available tools:
%s

User request:
%s`, strings.Join(defs, "\n"), text)
}
