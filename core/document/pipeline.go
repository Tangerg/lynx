package document

import "context"

// Reader sources documents from files, databases, APIs, or another origin.
// Concrete readers live outside core.
type Reader interface {
	Read(ctx context.Context) ([]*Document, error)
}

// Writer persists documents — to files, databases, vector stores, or
// any other sink. The error contract is "all-or-nothing on best
// effort" — implementations document their transactionality.
type Writer interface {
	// Write stores docs at the underlying destination. An error from a
	// later document may or may not roll back earlier writes; consult
	// the implementation's docs.
	Write(ctx context.Context, docs []*Document) error
}
