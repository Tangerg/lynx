package editguard

import "testing"

// TestTracker drives the pure read-before-edit invariant directly: a path must
// be recorded before it passes Check, a changed fingerprint is stale, a partial
// read fails a full-overwrite check, and Refresh permits consecutive edits.
func TestTracker(t *testing.T) {
	path := "/workspace/foo.go"
	one := FingerprintOf([]byte("one"))
	two := FingerprintOf([]byte("two"))
	tr := NewTracker()
	const sess = "s1"

	// Never read → a structured read-required verdict.
	if got := tr.Check(sess, path, one, false); got != ResultReadRequired || got.Allowed() {
		t.Fatalf("unread Check = %v, want missing", got)
	}

	// Read (full) → passes; a session boundary is respected.
	tr.Record(sess, path, one, false)
	if got := tr.Check(sess, path, one, false); got != ResultAllowed || !got.Allowed() {
		t.Fatalf("read Check = %v, want ok", got)
	}
	if got := tr.Check("other", path, one, false); got != ResultReadRequired {
		t.Fatalf("cross-session Check = %v, want missing (per-session isolation)", got)
	}

	// Changed content → stale.
	if got := tr.Check(sess, path, two, false); got != ResultChanged {
		t.Fatalf("changed Check = %v, want stale", got)
	}

	// Refresh re-stamps the current content → passes again.
	tr.Refresh(sess, path, two)
	if got := tr.Check(sess, path, two, false); got != ResultAllowed {
		t.Fatalf("post-refresh Check = %v, want ok", got)
	}

	// A partial read fails only the full-overwrite check (requireFull).
	tr.Record(sess, path, two, true)
	if got := tr.Check(sess, path, two, true); got != ResultFullReadRequired {
		t.Fatalf("partial full-overwrite Check = %v, want partial", got)
	}
	if got := tr.Check(sess, path, two, false); got != ResultAllowed {
		t.Fatalf("partial edit Check = %v, want ok (partial read allows an edit)", got)
	}

}
