package toolset

import "testing"

// TestTracker drives the pure read-before-edit invariant directly: a path must
// be recorded before it passes Check, a changed fingerprint is stale, a partial
// read fails a full-overwrite check, and Refresh permits consecutive edits.
func TestTracker(t *testing.T) {
	path := "/workspace/foo.go"
	one := fingerprintOf([]byte("one"))
	two := fingerprintOf([]byte("two"))
	tr := newReadTracker()
	const sess = "s1"

	// Never read → a structured read-required verdict.
	if got := tr.check(sess, path, one, false); got != readRequired || got.allowed() {
		t.Fatalf("unread Check = %v, want missing", got)
	}

	// Read (full) → passes; a session boundary is respected.
	tr.record(sess, path, one, false)
	if got := tr.check(sess, path, one, false); got != editAllowed || !got.allowed() {
		t.Fatalf("read Check = %v, want ok", got)
	}
	if got := tr.check("other", path, one, false); got != readRequired {
		t.Fatalf("cross-session Check = %v, want missing (per-session isolation)", got)
	}

	// Changed content → stale.
	if got := tr.check(sess, path, two, false); got != contentChanged {
		t.Fatalf("changed Check = %v, want stale", got)
	}

	// Refresh re-stamps the current content → passes again.
	tr.refresh(sess, path, two)
	if got := tr.check(sess, path, two, false); got != editAllowed {
		t.Fatalf("post-refresh Check = %v, want ok", got)
	}

	// A partial read fails only the full-overwrite check (requireFull).
	tr.record(sess, path, two, true)
	if got := tr.check(sess, path, two, true); got != fullReadRequired {
		t.Fatalf("partial full-overwrite Check = %v, want partial", got)
	}
	if got := tr.check(sess, path, two, false); got != editAllowed {
		t.Fatalf("partial edit Check = %v, want ok (partial read allows an edit)", got)
	}
}
