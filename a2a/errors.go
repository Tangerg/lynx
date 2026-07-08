package a2a

import (
	"errors"
	"fmt"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
)

// Input-validation sentinels. Use errors.Is to detect them.
var (
	// ErrNilCard is returned when a server or adapter is built without an AgentCard.
	ErrNilCard = errors.New("a2a: agent card must not be nil")

	// ErrNilAgent is returned when a server is built without an Agent.
	ErrNilAgent = errors.New("a2a: agent must not be nil")

	// ErrEmptyCardURL is returned by Dial when no card URL is supplied.
	ErrEmptyCardURL = errors.New("a2a: card URL must not be empty")
)

// RemoteAgentError reports that a remote A2A agent ended a task in a
// non-successful terminal state (failed / rejected / canceled). It lets a
// caller errors.As it apart from transport or protocol failures: the remote
// was reached and answered, but the work did not succeed.
type RemoteAgentError struct {
	// State is the terminal task state the remote reported.
	State sdka2a.TaskState
	// Detail is any human-readable message the remote attached, or "".
	Detail string
}

func (e *RemoteAgentError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("a2a: remote agent task ended in %s: %s", e.State, e.Detail)
	}
	return fmt.Sprintf("a2a: remote agent task ended in %s", e.State)
}
