package toolset

import "testing"

// TestTracker drives the pure read-before-mutation invariant directly: a path must
// be recorded before it passes Check, a changed fingerprint is stale, and
// Refresh permits consecutive mutations.
func TestTracker(t *testing.T) {
	path := "/workspace/foo.go"
	one := fingerprintOf([]byte("one"))
	two := fingerprintOf([]byte("two"))
	tr := newReadTracker()
	const sess = "s1"

	// Never read → a structured read-required verdict.
	if got := tr.check(sess, path, one); got != readRequired || got.allowed() {
		t.Fatalf("unread Check = %v, want missing", got)
	}

	// Read (full) → passes; a session boundary is respected.
	tr.record(sess, path, one)
	if got := tr.check(sess, path, one); got != mutationAllowed || !got.allowed() {
		t.Fatalf("read Check = %v, want ok", got)
	}
	if got := tr.check("other", path, one); got != readRequired {
		t.Fatalf("cross-session Check = %v, want missing (per-session isolation)", got)
	}

	// Changed content → stale.
	if got := tr.check(sess, path, two); got != contentChanged {
		t.Fatalf("changed Check = %v, want stale", got)
	}

	// Refresh re-stamps the current content → passes again.
	tr.refresh(sess, path, two)
	if got := tr.check(sess, path, two); got != mutationAllowed {
		t.Fatalf("post-refresh Check = %v, want ok", got)
	}
}
