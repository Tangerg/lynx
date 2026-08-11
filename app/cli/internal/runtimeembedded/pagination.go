package runtimeembedded

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// requireCompletePage protects adapters for list operations whose request has
// no continuation cursor. Accepting NextCursor there would silently truncate
// the CLI projection because the runtime offers no way to fetch the remainder.
func requireCompletePage[T any](operation string, page *protocol.Page[T]) ([]T, error) {
	if page == nil {
		return nil, fmt.Errorf("%s: runtime returned a nil page", operation)
	}
	if page.NextCursor != "" {
		return nil, fmt.Errorf("%s: runtime returned an unusable continuation cursor", operation)
	}
	return page.Data, nil
}

type cursorTraversal struct {
	operation string
	current   string
	seen      map[string]struct{}
}

func newCursorTraversal(operation, initial string) *cursorTraversal {
	return &cursorTraversal{
		operation: operation,
		current:   initial,
		seen:      map[string]struct{}{initial: {}},
	}
}

func (traversal *cursorTraversal) Current() string { return traversal.current }

func (traversal *cursorTraversal) Advance(next string) (bool, error) {
	if next == "" {
		traversal.current = ""
		return false, nil
	}
	if _, exists := traversal.seen[next]; exists {
		return false, fmt.Errorf("%s: runtime returned a cyclic continuation cursor", traversal.operation)
	}
	traversal.seen[next] = struct{}{}
	traversal.current = next
	return true, nil
}
