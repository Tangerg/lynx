package a2a

import (
	"errors"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
)

func TestTextOfResultRequiresSuccessfulResult(t *testing.T) {
	detail := sdka2a.NewMessage(sdka2a.MessageRoleAgent, sdka2a.NewTextPart("more input needed"))
	states := []sdka2a.TaskState{
		sdka2a.TaskStateUnspecified,
		sdka2a.TaskStateAuthRequired,
		sdka2a.TaskStateCanceled,
		sdka2a.TaskStateFailed,
		sdka2a.TaskStateInputRequired,
		sdka2a.TaskStateRejected,
		sdka2a.TaskStateSubmitted,
		sdka2a.TaskStateWorking,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			_, err := textOfResult(&sdka2a.Task{Status: sdka2a.TaskStatus{State: state, Message: detail}})
			remote, ok := errors.AsType[*RemoteAgentError](err)
			if !ok || remote.State != state || remote.Detail != "more input needed" {
				t.Fatalf("textOfResult error = %#v, want RemoteAgentError for %q", err, state)
			}
		})
	}

	completed := &sdka2a.Task{
		Status: sdka2a.TaskStatus{State: sdka2a.TaskStateCompleted},
		Artifacts: []*sdka2a.Artifact{{
			Parts: sdka2a.ContentParts{sdka2a.NewTextPart("done")},
		}},
	}
	if text, err := textOfResult(completed); err != nil || text != "done" {
		t.Fatalf("textOfResult(completed) = %q, %v; want done, nil", text, err)
	}
}

func TestTextOfResultRejectsNilProtocolValues(t *testing.T) {
	var message *sdka2a.Message
	var task *sdka2a.Task
	for _, result := range []sdka2a.SendMessageResult{nil, message, task} {
		if _, err := textOfResult(result); !errors.Is(err, ErrInvalidResult) {
			t.Fatalf("textOfResult(%T) error = %v, want ErrInvalidResult", result, err)
		}
	}
}

func TestRemoteAgentErrorNilReceiver(t *testing.T) {
	var remote *RemoteAgentError
	if got := remote.Error(); got != "a2a: remote agent task did not complete successfully" {
		t.Fatalf("nil RemoteAgentError = %q", got)
	}
}
