package fs

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Moving a file with apply_patch. The two headers of a unified diff already name
// two paths, so a move needs no format of its own — it was refused by a
// validation, and what makes it safe is what that validation used to stand in
// for: a destination is never overwritten, the origin is reported, and both
// endpoints are visible to the guards the caller wraps this tool in.

func movePatch(t *testing.T, from, to, body string) string {
	t.Helper()
	return "--- a/" + from + "\n+++ b/" + to + "\n" + body
}

func TestApplyPatch_MoveCarriesContentAndRemovesTheOrigin(t *testing.T) {
	dir := t.TempDir()
	from := writeTemp(t, dir, "old.txt", "alpha\nbeta\n")
	to := filepath.Join(dir, "new.txt")

	out, err := NewLocalExecutor("").ApplyPatch(t.Context(), ApplyPatchRequest{
		Patch: movePatch(t, from, to, "@@ -1,2 +1,2 @@\n alpha\n-beta\n+BETA\n"),
	})
	if err != nil {
		t.Fatalf("ApplyPatch: %v", err)
	}

	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("origin still exists (err = %v)", err)
	}
	landed, err := os.ReadFile(to)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(landed) != "alpha\nBETA\n" {
		t.Errorf("destination = %q, want the patched content", landed)
	}
	if len(out.Files) != 1 || out.Files[0].MovedFrom != from || out.Files[0].Path != to {
		t.Errorf("output = %+v, want one file moved %s → %s", out.Files, from, to)
	}
}

func TestApplyPatch_PureRenameNeedsNoHunk(t *testing.T) {
	// git emits a rename with two headers and nothing to apply. Requiring a hunk
	// would make the one patch that changes no content impossible to express.
	dir := t.TempDir()
	from := writeTemp(t, dir, "old.txt", "unchanged\n")
	to := filepath.Join(dir, "renamed.txt")

	out, err := NewLocalExecutor("").ApplyPatch(t.Context(), ApplyPatchRequest{
		Patch: movePatch(t, from, to, ""),
	})
	if err != nil {
		t.Fatalf("ApplyPatch: %v", err)
	}
	if out.Hunks != 0 {
		t.Errorf("hunks = %d, want 0", out.Hunks)
	}
	landed, err := os.ReadFile(to)
	if err != nil || string(landed) != "unchanged\n" {
		t.Errorf("destination = %q (err %v), want the original content", landed, err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("origin still exists (err = %v)", err)
	}
}

func TestApplyPatch_MoveRefusesToOverwriteItsDestination(t *testing.T) {
	// The one outcome a rename must never produce. Without this the model's
	// mistaken destination silently replaces a file nobody asked about.
	dir := t.TempDir()
	from := writeTemp(t, dir, "old.txt", "moving\n")
	occupied := writeTemp(t, dir, "taken.txt", "do not lose me\n")

	_, err := NewLocalExecutor("").ApplyPatch(t.Context(), ApplyPatchRequest{
		Patch: movePatch(t, from, occupied, ""),
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want it to name the collision", err)
	}
	kept, _ := os.ReadFile(occupied)
	if string(kept) != "do not lose me\n" {
		t.Errorf("destination = %q — the refusal did not protect it", kept)
	}
	if _, err := os.Stat(from); err != nil {
		t.Errorf("origin was removed by a refused patch: %v", err)
	}
}

func TestApplyPatch_MoveReportsBothEndpointsAsMutated(t *testing.T) {
	// MutationPaths is what the caller's guard stack reads to lock, to refuse
	// protected directories, and to require a prior read. A move that reported only
	// its destination would remove a file none of those three ever saw.
	dir := t.TempDir()
	from := filepath.Join(dir, "old.txt")
	to := filepath.Join(dir, "new.txt")

	paths, err := patchPaths(movePatch(t, from, to, ""))
	if err != nil {
		t.Fatalf("patchPaths: %v", err)
	}
	for _, want := range []string{from, to} {
		if !slices.Contains(paths, want) {
			t.Errorf("paths = %v, missing %s", paths, want)
		}
	}
}

func TestApplyPatch_RefusesTwoPatchesTouchingOneEndpoint(t *testing.T) {
	// Editing a file and moving another one onto it are two edits to one path, and
	// the result would depend on which was applied first.
	dir := t.TempDir()
	target := writeTemp(t, dir, "target.txt", "one\n")
	moving := writeTemp(t, dir, "moving.txt", "two\n")

	_, err := NewLocalExecutor("").ApplyPatch(t.Context(), ApplyPatchRequest{
		Patch: "--- a/" + target + "\n+++ b/" + target + "\n@@ -1 +1 @@\n-one\n+ONE\n" +
			movePatch(t, moving, target, ""),
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "duplicate file patch") {
		t.Errorf("error = %v, want it to name the duplicate", err)
	}
}
