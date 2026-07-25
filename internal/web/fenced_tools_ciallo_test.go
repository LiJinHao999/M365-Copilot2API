package web

import "testing"

func TestFencedToolCallsAcceptsCialloToolCallFence(t *testing.T) {
	tools := []map[string]any{
		{"type": "function", "function": map[string]any{"name": "Read", "parameters": map[string]any{"type": "object"}}},
		{"type": "function", "function": map[string]any{"name": "Write", "parameters": map[string]any{"type": "object"}}},
	}
	text := "I'll read it first.\n```tool_call\n{\"name\":\"Read\",\"arguments\":{\"file_path\":\"/tmp/a.go\"}}\n```\n"
	calls := fencedToolCalls(text, tools, "auto")
	if len(calls) != 1 || calls[0].Name != "Read" {
		t.Fatalf("calls=%#v", calls)
	}
	if string(calls[0].Arguments) == "" || !containsBytes(calls[0].Arguments, []byte("file_path")) {
		t.Fatalf("args=%s", calls[0].Arguments)
	}
}

func containsBytes(b, sub []byte) bool {
	return len(b) >= len(sub) && (string(b) == string(sub) || len(sub) == 0 || (len(b) > 0 && containsString(string(b), string(sub))))
}

func containsString(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
