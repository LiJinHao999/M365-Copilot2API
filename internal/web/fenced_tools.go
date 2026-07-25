package web

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	fencedToolCall      = regexp.MustCompile("(?s)```([A-Za-z0-9_-]+)\\s*\\n(.*?)\\n```")
	genericToolCallFence = regexp.MustCompile("(?is)```tool_call\\s*\\n(.*?)```")
)

// fencedToolCalls recovers tool invocations from model text.
// Compatibility order (Ciallo-aligned first, then legacy forms):
//  1. ```tool_call\n{"name","arguments"}\n```
//  2. ```<tool_name>\n{args}\n```
//  3. bash/shell fences auto-mapped to a bash tool
//  4. single-line {"command":...} JSON for shell clients
func fencedToolCalls(text string, tools []map[string]any, choice any) []detectedToolCall {
	allowed := allowedToolNames(tools)
	var out []detectedToolCall

	// 1) Ciallo-style unified tool_call fence.
	for _, m := range genericToolCallFence.FindAllStringSubmatch(text, -1) {
		body := strings.TrimSpace(m[1])
		var obj map[string]any
		if json.Unmarshal([]byte(body), &obj) != nil {
			continue
		}
		name := firstString(obj, "name", "tool", "tool_name", "function")
		if name == "" {
			continue
		}
		if !allowed[name] || !toolChoiceAllows(choice, name) {
			continue
		}
		args := obj["arguments"]
		if args == nil {
			args = obj["args"]
		}
		if args == nil {
			// Treat remaining fields as arguments.
			tmp := map[string]any{}
			for k, v := range obj {
				if k == "name" || k == "tool" || k == "tool_name" || k == "function" {
					continue
				}
				tmp[k] = v
			}
			args = tmp
		}
		b, _ := json.Marshal(args)
		out = append(out, detectedToolCall{ID: callID(name, string(b), len(out)), Type: toolType(name, tools), Name: name, Arguments: b})
	}
	if len(out) > 0 {
		return out
	}

	// 2/3) Per-tool-name fences + bash auto conversion.
	for _, m := range fencedToolCall.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if strings.EqualFold(name, "tool_call") {
			continue // already handled
		}
		args := strings.TrimSpace(m[2])
		var v any
		_ = json.Unmarshal([]byte(args), &v)
		// Auto-convert bash/shell code blocks to tool calls
		if !allowed[name] && (name == "bash" || name == "sh" || name == "shell" || name == "powershell" || name == "cmd") {
			if m, ok := v.(map[string]any); ok {
				if cmd, hasCmd := m["command"]; hasCmd && cmd != "" {
					cmdBytes, _ := json.Marshal(map[string]any{"command": cmd, "timeout": m["timeout"], "workdir": m["workdir"]})
					out = append(out, detectedToolCall{ID: callID("bash", string(cmdBytes), len(out)), Type: "function", Name: "bash", Arguments: cmdBytes})
					continue
				}
			}
			if v == nil {
				cmdBytes, _ := json.Marshal(map[string]any{"command": args})
				out = append(out, detectedToolCall{ID: callID("bash", string(cmdBytes), len(out)), Type: "function", Name: "bash", Arguments: cmdBytes})
				continue
			}
			continue
		}
		if !allowed[name] || !toolChoiceAllows(choice, name) {
			continue
		}
		if v == nil {
			continue
		}
		// If body is {"name","arguments"} even under a named fence, prefer that shape.
		if mv, ok := v.(map[string]any); ok {
			if n := firstString(mv, "name", "tool", "tool_name"); n != "" && allowed[n] {
				argsVal := mv["arguments"]
				if argsVal == nil {
					argsVal = mv["args"]
				}
				if argsVal != nil {
					b, _ := json.Marshal(argsVal)
					out = append(out, detectedToolCall{ID: callID(n, string(b), len(out)), Type: toolType(n, tools), Name: n, Arguments: b})
					continue
				}
			}
		}
		b, _ := json.Marshal(v)
		out = append(out, detectedToolCall{ID: callID(name, string(b), len(out)), Type: toolType(name, tools), Name: name, Arguments: b})
	}

	// 4) Plain JSON objects with a "command" field (not in fenced blocks)
	if len(out) == 0 {
		for i := 0; i < len(text); i++ {
			if text[i] != '{' {
				continue
			}
			end := strings.Index(text[i:], "\n")
			if end < 0 {
				end = len(text) - i
			}
			line := text[i : i+end]
			braceEnd := strings.LastIndex(line, "}")
			if braceEnd < 0 {
				continue
			}
			if !strings.Contains(line[:braceEnd+1], `"command"`) {
				continue
			}
			var obj map[string]any
			if json.Unmarshal([]byte(line[:braceEnd+1]), &obj) != nil {
				continue
			}
			if cmd, hasCmd := obj["command"]; hasCmd && cmd != "" {
				cmdBytes, _ := json.Marshal(map[string]any{"command": cmd, "timeout": obj["timeout"], "workdir": obj["workdir"]})
				out = append(out, detectedToolCall{ID: callID("bash", string(cmdBytes), len(out)), Type: "function", Name: "bash", Arguments: cmdBytes})
				break
			}
		}
	}
	return out
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
