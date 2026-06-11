package codeintel

import (
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/infra/lsp"
)

// TestNewProblems_FiltersBaseline is the deterministic guard against the LSP
// caching / staleness false positives the baseline diff exists to prevent: a
// pre-existing problem (even one whose line shifted, or one the server
// re-reported verbatim from cache) must not be counted as introduced — only a
// genuinely new diagnostic surfaces.
func TestNewProblems_FiltersBaseline(t *testing.T) {
	before := []lsp.Diagnostic{
		{Message: "undefined: foo", Severity: 1, Range: lsp.Range{Start: lsp.Position{Line: 10}}},
	}
	after := []lsp.Diagnostic{
		// same problem, shifted down 5 lines + re-reported → must be filtered
		{Message: "undefined: foo", Severity: 1, Range: lsp.Range{Start: lsp.Position{Line: 15}}},
		// genuinely new problem the edit introduced
		{Message: "undefined: bar", Severity: 1, Range: lsp.Range{Start: lsp.Position{Line: 20}}},
	}
	got := newProblems(before, after)
	if len(got) != 1 || got[0].Message != "undefined: bar" {
		t.Fatalf("newProblems = %v, want only [undefined: bar]", got)
	}

	// A cached, identical result (after == before) yields nothing.
	if n := newProblems(before, before); len(n) != 0 {
		t.Errorf("newProblems(before, before) = %v, want none (stale cache must not false-positive)", n)
	}
}
