package core

// OutputChannel is the action-level "say something to the user" sink.
// Modeled as an interface so tests capture, real apps wire to SSE /
// websocket streams, and library code never assumes more than "write
// a string". Both writes return an error so transport-backed
// implementations can surface I/O failures.
//
// Implementations MUST be safe for concurrent use: a ProcessConcurrent run
// hands the SAME channel instance to every parallel action branch (see the
// runtime's per-branch ProcessContext), so Write / WriteTyped / Close can be
// called from multiple goroutines at once. A transport-backed channel must
// guard its writer (and not Close while a sibling branch may still Write).
type OutputChannel interface {
	Write(msg string) error
	WriteTyped(topic string, payload any) error
	Close() error
}

// DevNullOutputChannel discards everything — the runtime's default
// when no channel is configured.
var DevNullOutputChannel OutputChannel = devNullChannel{}

type devNullChannel struct{}

func (devNullChannel) Write(string) error           { return nil }
func (devNullChannel) WriteTyped(string, any) error { return nil }
func (devNullChannel) Close() error                 { return nil }
