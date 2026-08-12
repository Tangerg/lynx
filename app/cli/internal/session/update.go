package session

import (
	"context"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type updater interface {
	UpdateSession(context.Context, agent.UpdateSession) (agent.Session, error)
}

// Update executes one optimistic session mutation and verifies that the
// runtime response fulfills the exact command before consumers project it.
func Update(ctx context.Context, writer updater, update agent.UpdateSession) (agent.Session, error) {
	if err := update.Validate(); err != nil {
		return agent.Session{}, err
	}
	updated, err := writer.UpdateSession(ctx, update)
	if err != nil {
		return agent.Session{}, err
	}
	if err := update.ValidateResult(updated); err != nil {
		return agent.Session{}, err
	}
	return updated, nil
}
