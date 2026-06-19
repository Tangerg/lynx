package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestEditDiffRows(t *testing.T) {
	rows := editDiffRows("line1\nline2\nline3\n", "line1\nCHANGED\nline3\n")
	want := []struct {
		typ  protocol.DiffRowType
		code string
	}{
		{protocol.DiffRowContext, "line1"},
		{protocol.DiffRowDeleted, "line2"},
		{protocol.DiffRowAdded, "CHANGED"},
		{protocol.DiffRowContext, "line3"},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i].Type != w.typ || rows[i].Code != w.code {
			t.Fatalf("row[%d] = {%s %q}, want {%s %q}", i, rows[i].Type, rows[i].Code, w.typ, w.code)
		}
	}

	// A no-op edit yields no diff (the client renders a plain "modified" row).
	if editDiffRows("same", "same") != nil {
		t.Fatal("no-op edit should yield nil rows")
	}

	// A pure insert: context line kept, new line added with its right-side number.
	ins := editDiffRows("a\n", "a\nb\n")
	if len(ins) != 2 || ins[1].Type != protocol.DiffRowAdded || ins[1].Code != "b" || ins[1].RightLine != 2 {
		t.Fatalf("insert diff = %+v", ins)
	}
}
