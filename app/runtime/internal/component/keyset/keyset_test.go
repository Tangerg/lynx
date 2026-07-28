package keyset

import (
	"errors"
	"testing"
)

func TestRoundTripReturnsTheAnchor(t *testing.T) {
	cursor := Encode("items.list", []string{"ses_1"}, []string{"42"})
	key, err := Decode(cursor, "items.list", []string{"ses_1"})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(key) != 1 || key[0] != "42" {
		t.Fatalf("key = %v, want [42]", key)
	}
}

func TestEmptyCursorIsTheFirstPage(t *testing.T) {
	key, err := Decode("", "items.list", []string{"ses_1"})
	if err != nil || key != nil {
		t.Fatalf("decode empty = (%v, %v), want (nil, nil)", key, err)
	}
}

// A cursor names the query it was minted for. Accepting one from a different
// method or filter set would continue a page against rows it never enumerated,
// which skips and repeats silently — the failure a page cursor exists to prevent.
func TestCursorFromAnotherQueryIsRejected(t *testing.T) {
	cursor := Encode("items.list", []string{"ses_1"}, []string{"42"})

	if _, err := Decode(cursor, "runs.list", []string{"ses_1"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-method decode err = %v, want ErrInvalidCursor", err)
	}
	if _, err := Decode(cursor, "items.list", []string{"ses_2"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-filter decode err = %v, want ErrInvalidCursor", err)
	}
	if _, err := Decode(cursor, "items.list", []string{"ses_1", "desc"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("added-filter decode err = %v, want ErrInvalidCursor", err)
	}
}

// Filters are joined before hashing so that a boundary shift between values is a
// different query, not the same one.
func TestFilterBoundariesAreNotInterchangeable(t *testing.T) {
	cursor := Encode("items.list", []string{"a", "bc"}, []string{"1"})
	if _, err := Decode(cursor, "items.list", []string{"ab", "c"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("shifted-boundary decode err = %v, want ErrInvalidCursor", err)
	}
}

func TestDamagedCursorIsRejected(t *testing.T) {
	for name, cursor := range map[string]string{
		"not base64":  "!!!!",
		"not json":    "aGVsbG8",
		"empty key":   Encode("items.list", []string{"ses_1"}, nil),
		"wrong shape": "eyJ2IjoxfQ",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(cursor, "items.list", []string{"ses_1"}); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("decode err = %v, want ErrInvalidCursor", err)
			}
		})
	}
}

func TestLimitClampsToTheReadsCeiling(t *testing.T) {
	for _, test := range []struct{ requested, max, want int }{
		{requested: 0, max: 200, want: 200},
		{requested: 50, max: 200, want: 50},
		{requested: 500, max: 200, want: 200},
		{requested: 200, max: 200, want: 200},
	} {
		got, err := Limit(test.requested, test.max)
		if err != nil || got != test.want {
			t.Fatalf("Limit(%d, %d) = (%d, %v), want %d", test.requested, test.max, got, err, test.want)
		}
	}
	if _, err := Limit(-1, 200); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("Limit(-1) err = %v, want ErrInvalidLimit", err)
	}
}

// A page returns a cursor exactly when the over-fetch proved there is more, so a
// caller can never mistake a capped page for the end of the collection.
func TestPageOfSignalsMoreOnlyWhenTheOverFetchProvesIt(t *testing.T) {
	key := func(row string) []string { return []string{row} }

	full := PageOf([]string{"a", "b", "c"}, 2, "items.list", []string{"ses_1"}, key)
	if len(full.Rows) != 2 || full.NextCursor == "" {
		t.Fatalf("over-fetched page = %+v, want 2 rows and a cursor", full)
	}
	anchor, err := Decode(full.NextCursor, "items.list", []string{"ses_1"})
	if err != nil || len(anchor) != 1 || anchor[0] != "b" {
		t.Fatalf("anchor = (%v, %v), want the last returned row", anchor, err)
	}

	exact := PageOf([]string{"a", "b"}, 2, "items.list", []string{"ses_1"}, key)
	if len(exact.Rows) != 2 || exact.NextCursor != "" {
		t.Fatalf("exact page = %+v, want 2 rows and no cursor", exact)
	}

	empty := PageOf(nil, 2, "items.list", []string{"ses_1"}, key)
	if len(empty.Rows) != 0 || empty.NextCursor != "" {
		t.Fatalf("empty page = %+v, want nothing", empty)
	}
}
