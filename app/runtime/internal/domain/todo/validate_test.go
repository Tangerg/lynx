package todo

import (
	"errors"
	"strings"
	"testing"
)

func items(specs ...string) []Item {
	out := make([]Item, 0, len(specs))
	for _, s := range specs {
		// spec form "content:status"
		c, st, _ := strings.Cut(s, ":")
		out = append(out, Item{Content: c, Status: Status(st)})
	}
	return out
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		prev    []Item
		next    []Item
		wantErr bool
	}{
		{"empty is fine", nil, items(), false},
		{"all pending", nil, items("a:pending", "b:pending"), false},
		{"one in_progress ok", nil, items("a:in_progress", "b:pending"), false},
		{"two in_progress rejected", nil, items("a:in_progress", "b:in_progress"), true},
		{"one newly completed ok", items("a:in_progress", "b:pending"), items("a:completed", "b:in_progress"), false},
		{"two newly completed rejected", items("a:pending", "b:pending"), items("a:completed", "b:completed"), true},
		{"already-completed carried forward ok", items("a:completed", "b:in_progress"), items("a:completed", "b:completed"), false},
		{"unknown status rejected", nil, items("a:done"), true},
		{"clearing a completed list ok", items("a:completed", "b:completed"), items(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.prev, tc.next)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate(%v→%v) = nil, want error", tc.prev, tc.next)
				}
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("error %v is not ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(%v→%v) = %v, want nil", tc.prev, tc.next, err)
			}
		})
	}
}
