package server

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// editDiffRows computes a call-scoped structured diff (API.md §4.5 DiffRow[])
// between an edit's old_string and new_string — the literal patch THIS edit
// applied, so the client renders exactly what changed (§12.1 C) instead of
// re-querying the whole worktree. Line numbers are relative to the edited
// snippet: the runtime doesn't know the snippet's absolute offset in the file,
// and a "what changed in this edit" card wants the change's own structure, not
// file coordinates. Returns nil for a no-op edit (old == new) — the client
// then renders a plain "modified" row.
func editDiffRows(oldText, newText string) []protocol.DiffRow {
	if oldText == newText {
		return nil
	}
	a := splitDiffLines(oldText)
	b := splitDiffLines(newText)
	matcher := difflib.NewMatcher(a, b)

	var rows []protocol.DiffRow
	left, right := 1, 1
	emitDeletes := func(i1, i2 int) {
		for i := i1; i < i2; i++ {
			rows = append(rows, protocol.DiffRow{Type: protocol.DiffRowDeleted, LeftLine: left, Code: a[i]})
			left++
		}
	}
	emitAdds := func(j1, j2 int) {
		for j := j1; j < j2; j++ {
			rows = append(rows, protocol.DiffRow{Type: protocol.DiffRowAdded, RightLine: right, Code: b[j]})
			right++
		}
	}
	for _, op := range matcher.GetOpCodes() {
		switch op.Tag {
		case 'e': // equal → context
			for i := op.I1; i < op.I2; i++ {
				rows = append(rows, protocol.DiffRow{Type: protocol.DiffRowContext, LeftLine: left, RightLine: right, Code: a[i]})
				left++
				right++
			}
		case 'd': // delete
			emitDeletes(op.I1, op.I2)
		case 'i': // insert
			emitAdds(op.J1, op.J2)
		case 'r': // replace → the removed lines, then the added ones
			emitDeletes(op.I1, op.I2)
			emitAdds(op.J1, op.J2)
		}
	}
	return rows
}

// splitDiffLines splits text into lines without their terminators (the wire
// DiffRow.Code is line content sans newline, API.md §4.5). A trailing newline
// doesn't yield a spurious empty final line ("a\n" → ["a"]).
func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}
