package chathub

import "testing"

func TestUnseenDeltaDedupesSnapshotsAndCursors(t *testing.T) {
	steps := []struct {
		incoming string
		wantDelta string
		wantSkip bool
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
	// Replays the bug: multiple full snapshots then cursor tokens.
	frames := []string{
		"A",
		"AB",
		"ABC",
		"ABC",
		"D",
		"ABCDE",
	}
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
