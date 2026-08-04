package plan

import (
	"context"
	"errors"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
)

const enterDescription = `Enter read-only Plan mode for the current session.

Use this when the user asks you to investigate, design, or propose a Plan before
making changes. It blocks write, command, and network tools only for this
session and remembers the current permission mode for exit_plan_mode to restore.
It does not create or modify the Plan; use set_plan after entering. Entering
requires no approval because it only reduces permissions.`

type enterArgs struct{}

type enterer struct{ modes enterPolicy }

func newEnter(modes enterPolicy) (toolcontract.Tool, error) {
	if modes == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[enterArgs, string](
		toolcontract.FuncConfig{Name: "enter_plan_mode", Description: enterDescription},
		(&enterer{modes: modes}).enter,
	)
}

func (t *enterer) enter(ctx context.Context, _ enterArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "", errors.New("enter_plan_mode: no active session")
	}
	changed, err := t.modes.EnterPlanMode(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !changed {
		return "The session is already in Plan mode. Continue investigating or update the Plan with set_plan.", nil
	}
	return "Plan mode entered for this session. Write, command, and network tools are now blocked. Investigate with read-only tools and maintain the proposed Plan with set_plan.", nil
}
