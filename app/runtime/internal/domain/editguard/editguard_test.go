package editguard

import (
	"strings"
	"testing"
)

// TestTracker drives the pure read-before-edit invariant directly: a path must
// be recorded before it passes Check, a changed fingerprint is stale, a partial
// read fails a full-overwrite check, and Refresh permits consecutive edits.
func TestTracker(t *testing.T) {
	path := "/workspace/foo.go"
	one := FingerprintOf([]byte("one"))
	two := FingerprintOf([]byte("two"))
	tr := NewTracker()
	const sess = "s1"

	// Never read → missing; the message tells the model to read first.
	if got := tr.Check(sess, path, one, false); got != resultMissing {
		t.Fatalf("unread Check = %v, want missing", got)
	}
	if msg := tr.Check(sess, path, one, false).Message("foo.go", "editing"); !strings.Contains(msg, "must read foo.go before editing") {
		t.Fatalf("missing message = %q", msg)
	}

	// Read (full) → passes; a session boundary is respected.
	tr.Record(sess, path, one, false)
	if got := tr.Check(sess, path, one, false); got != resultOK {
		t.Fatalf("read Check = %v, want ok", got)
	}
	if got := tr.Check("other", path, one, false); got != resultMissing {
		t.Fatalf("cross-session Check = %v, want missing (per-session isolation)", got)
	}

	// Changed content → stale.
	if got := tr.Check(sess, path, two, false); got != resultStale {
		t.Fatalf("changed Check = %v, want stale", got)
	}

	// Refresh re-stamps the current content → passes again.
	tr.Refresh(sess, path, two)
	if got := tr.Check(sess, path, two, false); got != resultOK {
		t.Fatalf("post-refresh Check = %v, want ok", got)
	}

	// A partial read fails only the full-overwrite check (requireFull).
	tr.Record(sess, path, two, true)
	if got := tr.Check(sess, path, two, true); got != resultPartial {
		t.Fatalf("partial full-overwrite Check = %v, want partial", got)
	}
	if got := tr.Check(sess, path, two, false); got != resultOK {
		t.Fatalf("partial edit Check = %v, want ok (partial read allows an edit)", got)
	}

	// A passing result renders no message.
	if msg := resultOK.Message("foo.go", "editing"); msg != "" {
		t.Fatalf("ok message = %q, want empty", msg)
	}
}
