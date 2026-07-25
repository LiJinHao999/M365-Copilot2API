package chathub

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
)

type Tool struct {
	Type     string          `json:"type"`
	Function json.RawMessage `json:"function,omitempty"`
}

// searchIntentRe matches common "please search/look up" cues in Chinese and English.
// Used only to decide whether to attach the BuiltIn BingWebSearch plugin.
var searchIntentRe = regexp.MustCompile(`(?i)(搜索|搜一下|搜下|查一下|查下|查找|查询|检索|联网|上网|实时|今日|今天|最新|新闻|天气|股价|汇率|热搜|热榜|web\s*search|search\s+(for|the|up)|look\s+up|google|bing|what'?s\s+happening|current\s+(price|weather|news)|latest\s+news)`)

func wantsWebSearch(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	// Avoid attaching search for pure tool-result continuation turns.
	if strings.Contains(text, "[tool result") && !searchIntentRe.MatchString(text) {
		return false
	}
	return searchIntentRe.MatchString(text)
}

func clientPlugins(tools []Tool, mcpServerURL string, promptText string) []any {
	plugins := make([]any, 0, len(tools)+2)
	// Keyword-triggered BuiltIn web search, matching the Ciallo / browser-compatible path.
	// Only inject when the user text looks like a search/lookup request so ordinary
	// coding turns are not forced through Bing.
	if wantsWebSearch(promptText) {
		plugins = append(plugins, map[string]any{
			"Id":     "BingWebSearch",
			"Source": "BuiltIn",
		})
	}
	if mcpServerURL != "" {
		plugins = append(plugins, map[string]any{
			"Id":                "mcp-gateway",
			"Source":            "MCPServer",
			"Description":       "MCP Gateway tools",
			"Transport":         "mcp",
			"TransportUrl":      mcpServerURL,
			"TransportProtocol": "https://copilot.microsoft.com/schemas/plugins/local/transport/1.0",
		})
	}
	for _, t := range tools {
		var f struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		}
		if json.Unmarshal(t.Function, &f) != nil || f.Name == "" {
			continue
		}
		plugins = append(plugins, map[string]any{"Id": f.Name, "Source": "Client", "Description": f.Description, "Parameters": f.Parameters})
	}
	return plugins
}

// validCustomToneID allows free-form upstream tones beyond the built-in catalog
// so admins can map newly observed ChatHub tones without a code change.
func ValidCustomToneID(tone string) bool {
	tone = strings.TrimSpace(tone)
	if tone == "" || len(tone) > 128 {
		return false
	}
	for _, r := range tone {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	// Prefer ChatHub-looking identifiers (Magic / Claude_Fable / Gpt_5_5_Chat).
	return true
}
