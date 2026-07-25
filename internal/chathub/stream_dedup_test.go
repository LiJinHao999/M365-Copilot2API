package chathub

import (
	"strings"
	"testing"
)

func TestUnseenDeltaDedupesSnapshotsAndCursors(t *testing.T) {
	steps := []struct {
		incoming   string
		wantDelta  string
		wantSkip   bool
		wantStream string
	}{
		{"Hello", "Hello", false, "Hello"},
		{"Hello", "", true, "Hello"},
		{"Hello world", " world", false, "Hello world"},
		{"!", "!", false, "Hello world!"},
		{"Hello world!", "", true, "Hello world!"},
		// Non-prefix full snapshot must not re-append.
		{"Totally different answer", "", true, "Hello world!"},
	}
	streamed := ""
	for i, step := range steps {
		delta, skip := unseenDelta(streamed, step.incoming)
		if delta != step.wantDelta || skip != step.wantSkip {
			t.Fatalf("step %d incoming=%q delta=%q skip=%v want delta=%q skip=%v", i, step.incoming, delta, skip, step.wantDelta, step.wantSkip)
		}
		streamed += delta
		if streamed != step.wantStream {
			t.Fatalf("step %d streamed=%q want %q", i, streamed, step.wantStream)
		}
	}
}

func TestUnseenDeltaHandlesRepeatedFullSnapshots(t *testing.T) {
	frames := []string{"A", "AB", "ABC", "ABC", "D", "ABCDE"}
	streamed := ""
	for _, f := range frames {
		delta, skip := unseenDelta(streamed, f)
		if skip {
			continue
		}
		streamed += delta
	}
	if streamed != "ABCDE" {
		t.Fatalf("got %q", streamed)
	}
}

func TestUnseenDeltaKeepsTailWhenCitationsAppear(t *testing.T) {
	// Simulates M365 inserting private-use cite markers into a later snapshot.
	// Without citation-aware matching, the second frame is treated as non-prefix
	// and the tail ("明天...") is dropped.
	streamed := ""
	d, skip := unseenDelta(streamed, "今天闷热。")
	if skip || d != "今天闷热。" {
		t.Fatalf("first=%q skip=%v", d, skip)
	}
	streamed += d

	// Later full snapshot with cite junk inserted before the new tail.
	incoming := "今天闷热。citeturn1search1明天更热。"
	d, skip = unseenDelta(streamed, incoming)
	if skip {
		t.Fatal("should not skip citation-extended snapshot")
	}
	streamed += d
	cleaned := CleanM365Citations(streamed)
	if cleaned != "今天闷热。明天更热。" {
		t.Fatalf("got %q", cleaned)
	}
}

func TestCleanM365Citations(t *testing.T) {
	in := "气温：29℃ citeturn1search4\n**风力**：东南风 <cite>turn1search11</cite>"
	got := CleanM365Citations(in)
	if !containsAll(got, "气温：29℃", "**风力**：东南风") {
		t.Fatalf("got %q", got)
	}
	if containsCitePUA(got) || indexOf(strings.ToLower(got), "cite") >= 0 {
		t.Fatalf("cite remains: %q", got)
	}
}

func TestFinalTextReconcilePrefersCompleteFinal(t *testing.T) {
	streamed := "今天闷热。更热。" // missing 明天
	final := "今天闷热。明天更热。"
	got := finalTextReconcile(streamed, final)
	if got != final {
		t.Fatalf("got %q", got)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if indexOf(s, p) < 0 {
			return false
		}
	}
	return true
}

func containsCitePUA(s string) bool {
	for _, r := range s {
		if r >= 0xE000 && r <= 0xF8FF {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
