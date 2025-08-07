package document

import (
	"context"
)

// IDGenerator defines an interface for generating unique identifiers.
// Implementations can create IDs based on various strategies and input objects.
type IDGenerator interface {
	// Generate creates a unique identifier string based on optional input objects.
	// The implementation determines how the input objects influence the generated ID.
	Generate(ctx context.Context, obj ...any) string
}
