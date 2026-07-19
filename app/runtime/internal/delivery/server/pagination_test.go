package server

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestPageByCursorContinuesAfterStableAnchor(t *testing.T) {
	values := []string{"a", "b", "c"}
	first, cursor, err := pageByCursor(values, func(value string) string { return value }, "", 2, 10)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 2 || first[0] != "a" || first[1] != "b" || cursor == "" {
		t.Fatalf("first page = %v cursor=%q", first, cursor)
	}
	second, next, err := pageByCursor(values, func(value string) string { return value }, cursor, 2, 10)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 1 || second[0] != "c" || next != "" {
		t.Fatalf("second page = %v cursor=%q", second, next)
	}
}

func TestPageByCursorRejectsInvalidQueries(t *testing.T) {
	values := []string{"a", "b"}
	tests := []struct {
		name   string
		cursor string
		limit  int
	}{
		{name: "negative limit", limit: -1},
		{name: "malformed cursor", cursor: "%%%"},
		{name: "missing anchor", cursor: base64.RawURLEncoding.EncodeToString([]byte("gone"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := pageByCursor(values, func(value string) string { return value }, tt.cursor, tt.limit, 10)
			if !errors.Is(err, protocol.ErrInvalidParams) {
				t.Fatalf("error = %v, want ErrInvalidParams", err)
			}
		})
	}
}

func TestPageOrderedByCursorToleratesDeletedAnchor(t *testing.T) {
	values := []string{"a", "c", "d"}
	cursor := base64.RawURLEncoding.EncodeToString([]byte("b"))
	page, next, err := pageOrderedByCursor(values, func(value string) string { return value }, cursor, 2, 10)
	if err != nil {
		t.Fatalf("page after deleted anchor: %v", err)
	}
	if len(page) != 2 || page[0] != "c" || page[1] != "d" || next != "" {
		t.Fatalf("page = %v cursor=%q", page, next)
	}
}
