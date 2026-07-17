package a2a

import (
	"errors"
	"fmt"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
)

// Protocol and configuration sentinels. Use errors.Is to detect them.
var (
	// ErrNilCard is returned when a server or adapter is built without an AgentCard.
	ErrNilCard = errors.New("a2a: agent card must not be nil")
	// ErrInvalidCard is returned when an AgentCard cannot be represented on the
	// protocol wire.
	ErrInvalidCard = errors.New("a2a: invalid agent card")

	// ErrNilAgent is returned when a server is built without an Agent.
	ErrNilAgent = errors.New("a2a: agent must not be nil")

	// ErrEmptyCardURL is returned when no card URL is supplied.
	ErrEmptyCardURL = errors.New("a2a: card URL must not be empty")

	// ErrInvalidRPCPattern is returned when ServerConfig.RPCPattern is not a
	// valid net/http ServeMux pattern or conflicts with the AgentCard endpoint.
	ErrInvalidRPCPattern = errors.New("a2a: invalid RPC pattern")

	// ErrInvalidResult is returned when a remote agent returns a nil or
	// otherwise unsupported message result.
	ErrInvalidResult = errors.New("a2a: invalid send-message result")
)

var errNilAgentSequence = errors.New("a2a: agent returned a nil output sequence")

// RemoteAgentError reports that a remote A2A task did not complete
// successfully. It lets a caller use [errors.AsType] to distinguish it from
// transport or protocol failures: the remote was reached and answered, but the
// work failed, was canceled or rejected, or requires unsupported continuation.
type RemoteAgentError struct {
	// State is the task state the remote reported.
	State sdka2a.TaskState
	// Detail is any human-readable message the remote attached, or "".
	Detail string
}

func (e *RemoteAgentError) Error() string {
	if e == nil {
		return "a2a: remote agent task did not complete successfully"
	}
	if e.Detail != "" {
		return fmt.Sprintf("a2a: remote agent task did not complete successfully (state %s): %s", e.State, e.Detail)
	}
	return fmt.Sprintf("a2a: remote agent task did not complete successfully (state %s)", e.State)
}
