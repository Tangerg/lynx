package http

import (
	"testing"
	"time"
)

// TestCompareEventID_NumericNotLexical pins the bug the global eventId
// scheme fixes: the old per-run format (evt_<runid>_<seq>) compared
// lexically, so "..._10" sorted before "..._5". The padded global format
// must compare numerically.
func TestCompareEventID_NumericNotLexical(t *testing.T) {
	if compareEventID("evt_00000000005", "evt_00000000010") >= 0 {
		t.Fatal("evt_...005 must sort before evt_...010")
	}
	if compareEventID("evt_00000000010", "evt_00000000005") <= 0 {
		t.Fatal("evt_...010 must sort after evt_...005")
	}
	if compareEventID("evt_00000000007", "evt_00000000007") != 0 {
		t.Fatal("equal ids must compare equal")
	}
}

func TestStreamBuffer_SinceReturnsTailInOrder(t *testing.T) {
	b := &streamBuffer{}
	now := time.Now()
	for _, id := range []string{"evt_00000000001", "evt_00000000002", "evt_00000000003"} {
		b.append(streamRecord{eventID: id, at: now})
	}
	got := b.since("evt_00000000001")
	if len(got) != 2 || got[0].eventID != "evt_00000000002" || got[1].eventID != "evt_00000000003" {
		t.Fatalf("since(001) = %+v, want [002 003]", got)
	}
	if all := b.since(""); len(all) != 3 {
		t.Fatalf(`since("") = %d, want 3 (all)`, len(all))
	}
}
