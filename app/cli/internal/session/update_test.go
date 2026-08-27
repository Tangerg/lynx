package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/workspace"
)

type updateWriterStub struct {
	calls  int
	result agent.Session
}

func (u *updateWriterStub) UpdateSession(context.Context, agent.UpdateSession) (agent.Session, error) {
	u.calls++
	return u.result, nil
}

func TestUpdateValidatesTheCommandBeforeMutationAndTheResultAfterward(t *testing.T) {
	writer := &updateWriterStub{}
	if _, err := Update(t.Context(), writer, agent.UpdateSession{SessionID: "ses_1"}); err == nil {
		t.Fatal("Update accepted an empty mutation")
	}
	if writer.calls != 0 {
		t.Fatalf("invalid command reached the runtime %d time(s)", writer.calls)
	}

	title := "Renamed"
	writer.result = agent.Session{
		ID: "ses_wrong", Title: title, Status: agent.SessionIdle, Revision: 2,
		Workspace: workspace.Workspace{Path: "/workspace", ProjectRoot: "/workspace", Availability: workspace.Available},
	}
	_, err := Update(t.Context(), writer, agent.UpdateSession{
		SessionID: "ses_1", Title: &title, ExpectedRevision: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "runtime returned session") {
		t.Fatalf("Update result error = %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("valid command reached the runtime %d time(s), want 1", writer.calls)
	}
}
