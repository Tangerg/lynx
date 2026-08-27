package runtimeembedded

import "github.com/Tangerg/scope/app/runtime/protocol"

// requireCompletePage protects adapters for list operations whose request has
// no continuation cursor. Accepting NextCursor there would silently truncate
// the CLI projection because the runtime offers no way to fetch the remainder.
func requireCompletePage[T any](operation string, page *protocol.Page[T]) ([]T, error) {
	if page == nil {
		return nil, runtimeContractViolation("%s returned a nil page", operation)
	}
	if page.NextCursor != "" {
		return nil, runtimeContractViolation("%s returned an unusable continuation cursor", operation)
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

func (c *cursorTraversal) Current() string { return c.current }

func (c *cursorTraversal) Advance(next string) (bool, error) {
	if next == "" {
		c.current = ""
		return false, nil
	}
	if _, exists := c.seen[next]; exists {
		return false, runtimeContractViolation("%s returned a cyclic continuation cursor", c.operation)
	}
	c.seen[next] = struct{}{}
	c.current = next
	return true, nil
}
