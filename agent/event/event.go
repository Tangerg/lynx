package event

import (
	"encoding/json"
	"time"
)

// Event is the common interface — every concrete event embeds Header
// so it satisfies these methods without each type re-implementing them.
type Event interface {
	Timestamp() time.Time
	ProcessID() string
	Kind() string
}

// Header is the embedded carrier shared across all concrete events.
// It's an opaque value object: built via [NewHeader] and read through
// the [Event] interface methods. The timestamp / process id reach the
// wire via [emit]'s envelope (which reads them through Timestamp() /
// ProcessID()), so the fields carry no JSON tags of their own.
type Header struct {
	at        time.Time
	processID string
}

func (h Header) Timestamp() time.Time { return h.at }
func (h Header) ProcessID() string    { return h.processID }

// NewHeader stamps a fresh event with the current time.
func NewHeader(processID string) Header {
	return Header{at: time.Now(), processID: processID}
}

// envelope is the on-wire JSON shape for every event: a discriminator
// field plus the Header's timestamp / process id plus an opaque
// payload object. Centralized here so each concrete event's MarshalJSON
// is a one-liner.
type envelope struct {
	Kind      string    `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
	ProcessID string    `json:"process_id"`
	Payload   any       `json:"payload,omitempty"`
}

// emit wraps the supplied payload in an envelope, fills the
// discriminator and header fields from e, and marshals to JSON. It's the
// shared body of every event's MarshalJSON.
func emit(e Event, payload any) ([]byte, error) {
	return json.Marshal(envelope{
		Kind:      e.Kind(),
		Timestamp: e.Timestamp(),
		ProcessID: e.ProcessID(),
		Payload:   payload,
	})
}

// errString collapses an error to its message; nil returns "" so the
// JSON encoder can omitempty-elide it.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
