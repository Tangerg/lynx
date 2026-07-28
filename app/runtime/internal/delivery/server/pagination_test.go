package server

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// A page request a read refuses is the client's to fix, so it has to reach the
// wire as invalid_params. Falling through to the unrecognized-error default would
// report internal_error and hide the remedy — correct the limit, or start from the
// first page.
func TestWirePageErrorMapsRefusedPageRequests(t *testing.T) {
	for name, err := range map[string]error{
		"cursor": keyset.ErrInvalidCursor,
		"limit":  keyset.ErrInvalidLimit,
	} {
		t.Run(name, func(t *testing.T) {
			if got := wirePageError(err); !errors.Is(got, protocol.ErrInvalidParams) {
				t.Fatalf("wirePageError = %v, want ErrInvalidParams", got)
			}
			if got := wirePageError(err); !errors.Is(got, err) {
				t.Fatalf("wirePageError dropped the cause: %v", got)
			}
		})
	}
}

func TestWirePageErrorLeavesOtherFailuresAlone(t *testing.T) {
	store := errors.New("store unavailable")
	if got := wirePageError(store); !errors.Is(got, store) || errors.Is(got, protocol.ErrInvalidParams) {
		t.Fatalf("wirePageError = %v, want the store failure unchanged", got)
	}
	if wirePageError(nil) != nil {
		t.Fatal("wirePageError(nil) returned an error")
	}
}
