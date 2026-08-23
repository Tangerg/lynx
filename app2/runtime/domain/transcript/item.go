// Package transcript owns durable user-visible facts independently of wire
// presentation. Rich item payloads remain opaque to the store.
package transcript

import "time"

type Record struct {
	ID, SessionID, RunID string
	Ordinal              int
	Body                 []byte
	CreatedAt            time.Time
}
