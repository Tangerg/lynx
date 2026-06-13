package editguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTracker drives the read-before-edit invariant directly (no tools): a file
// must be recorded as read before it passes Check, a change on disk makes it
// stale, a partial read fails a full-overwrite check, and Refresh re-stamps so
// consecutive edits pass.
func TestTracker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := NewTracker()
	const sess = "s1"

	// Never read → missing; the message tells the model to read first.
	if got := tr.Check(sess, path, false); got != resultMissing {
		t.Fatalf("unread Check = %v, want missing", got)
	}
	if msg := tr.Check(sess, path, false).Message("foo.go", "editing"); !strings.Contains(msg, "must read foo.go before editing") {
		t.Fatalf("missing message = %q", msg)
	}

	// Read (full) → passes; a session boundary is respected.
	tr.Record(sess, path, false)
	if got := tr.Check(sess, path, false); got != resultOK {
		t.Fatalf("read Check = %v, want ok", got)
	}
	if got := tr.Check("other", path, false); got != resultMissing {
		t.Fatalf("cross-session Check = %v, want missing (per-session isolation)", got)
	}

	// Changed on disk → stale.
	if err := os.WriteFile(path, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tr.Check(sess, path, false); got != resultStale {
		t.Fatalf("changed Check = %v, want stale", got)
	}

	// Refresh re-stamps the current content → passes again.
	tr.Refresh(sess, path)
	if got := tr.Check(sess, path, false); got != resultOK {
		t.Fatalf("post-refresh Check = %v, want ok", got)
	}

	// A partial read fails only the full-overwrite check (requireFull).
	tr.Record(sess, path, true)
	if got := tr.Check(sess, path, true); got != resultPartial {
		t.Fatalf("partial full-overwrite Check = %v, want partial", got)
	}
	if got := tr.Check(sess, path, false); got != resultOK {
		t.Fatalf("partial edit Check = %v, want ok (partial read allows an edit)", got)
	}

	// A passing result renders no message.
	if msg := resultOK.Message("foo.go", "editing"); msg != "" {
		t.Fatalf("ok message = %q, want empty", msg)
	}
}
