// Package conversation owns the exact provider-neutral model context journal.
// It is distinct from transcript Items, which are a user-interface projection.
package conversation

type Record struct {
	SessionID string
	RunID     string
	Ordinal   int
	Body      []byte
}
