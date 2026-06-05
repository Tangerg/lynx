package event

import (
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// Event is the common interface — every concrete event embeds BaseEvent
// so it satisfies these methods without each type re-implementing them.
type Event interface {
	Timestamp() time.Time
	ProcessID() string
	EventName() string
}

// BaseEvent is the embedded carrier shared across all concrete events.
// It's an opaque value object: built via [NewBaseEvent] and read through
// the [Event] interface methods. The timestamp / process id reach the
// wire via [emit]'s envelope (which reads them through Timestamp() /
// ProcessID()), so the fields carry no JSON tags of their own.
type BaseEvent struct {
	at  time.Time
	pid string
}

func (b BaseEvent) Timestamp() time.Time { return b.at }
func (b BaseEvent) ProcessID() string    { return b.pid }
func (b BaseEvent) EventName() string    { return "base" }

// NewBaseEvent stamps a fresh event with the configured time source.
func NewBaseEvent(processID string) BaseEvent {
	return BaseEvent{at: core.Now(), pid: processID}
}

// envelope is the on-wire JSON shape for every event: a discriminator
// field plus the BaseEvent's timestamp / process id plus an opaque
// payload object. Centralized here so each concrete event's MarshalJSON
// is a one-liner.
type envelope struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	ProcessID string    `json:"process_id"`
	Payload   any       `json:"payload,omitempty"`
}

// emit wraps the supplied payload in an envelope, fills the
// discriminator and base fields from e, and marshals to JSON. It's the
// shared body of every event's MarshalJSON.
func emit(e Event, payload any) ([]byte, error) {
	return json.Marshal(envelope{
		Event:     e.EventName(),
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
