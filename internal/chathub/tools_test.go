package chathub

import "testing"

func TestWantsWebSearchKeywords(t *testing.T) {
	yes := []string{
		"帮我搜索一下今天北京天气",
		"查一下英伟达股价",
		"最新新闻有哪些",
		"search for the current weather in Tokyo",
		"look up USD to CNY exchange rate",
	}
	no := []string{
		"帮我重构这段 Go 代码",
		"解释一下 map 和 slice 的区别",
		"[tool result id=call_1]\nexit 0\n",
	}
	for _, s := range yes {
		if !wantsWebSearch(s) {
			t.Fatalf("expected search intent: %q", s)
		}
	}
	for _, s := range no {
		if wantsWebSearch(s) {
			t.Fatalf("did not expect search intent: %q", s)
		}
	}
}

func TestClientPluginsInjectsBingOnSearchIntent(t *testing.T) {
	plugins := clientPlugins(nil, "", "帮我搜索今天的新闻")
	if len(plugins) != 1 {
		t.Fatalf("plugins=%#v", plugins)
	}
	p := plugins[0].(map[string]any)
	if p["Id"] != "BingWebSearch" || p["Source"] != "BuiltIn" {
		t.Fatalf("plugin=%#v", p)
	}
	// Non-search prompts should not force Bing.
	if got := clientPlugins(nil, "", "重构这个函数"); len(got) != 0 {
		t.Fatalf("unexpected plugins for coding prompt: %#v", got)
	}
}

func TestValidCustomToneID(t *testing.T) {
	if !ValidCustomToneID("Claude_Opus_Experimental") {
		t.Fatal("custom tone should be allowed")
	}
	if ValidCustomToneID("bad tone!") || ValidCustomToneID("") {
		t.Fatal("invalid tones accepted")
	}
}
