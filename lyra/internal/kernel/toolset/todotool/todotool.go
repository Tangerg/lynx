// Package todotool exposes the model-facing todo_write tool over a
// [todo.Service]. It is named todotool (not todo) to avoid colliding with the
// service/todo domain package it builds on — the same disambiguation the
// lsptools package uses against service/codeintel + infra/lsp.
//
// The tool is the LLM-facing presentation of the todo domain: it parses the
// model's full-list replacement, runs the domain's progress-integrity check
// ([todo.Validate]), and persists via the service — keeping the rules in the
// domain and only the wire shape + recovery messaging here (the same split as
// the editguard tool wrappers).
package todotool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/domain/todo"
	"github.com/Tangerg/lynx/lyra/internal/kernel/toolset/turnctx"
)

const description = `Maintain a structured task list for the current session.

Use this to plan and track multi-step work so progress survives across turns.
Call it whenever the plan changes: when you start a task, finish one, discover
new steps, or re-scope.

Pass the COMPLETE updated list every time — it REPLACES the stored list (it is
not a delta). Rules, enforced by the runtime:
  - Keep exactly ONE task "in_progress" at a time; the rest "pending" or "completed".
  - Mark a task "completed" ONLY when it is fully done (tests pass, no errors),
    and complete them ONE AT A TIME — do not flip several to completed in one call.
Skip this tool for trivial single-step requests; it is for real multi-step work.`

const inputSchema = `{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "description": "The complete task list, in order. Replaces the stored list.",
      "items": {
        "type": "object",
        "properties": {
          "content": {
            "type": "string",
            "description": "Imperative description of the task (e.g. \"Add the retry guard to fetch()\")."
          },
          "status": {
            "type": "string",
            "enum": ["pending", "in_progress", "completed"],
            "description": "pending = not started; in_progress = actively working (at most one); completed = fully done."
          }
        },
        "required": ["content", "status"]
      }
    }
  },
  "required": ["todos"]
}`

type writeArgs struct {
	Todos []todo.Item `json:"todos"`
}

// New builds the todo_write tool over svc. It returns nil when svc is nil so
// the caller can simply omit the tool — the feature is disabled, not a broken
// tool. The session id is read per-call off the turn's blackboard
// ([turnctx.TurnSession]), so one tool instance serves every session.
func New(svc todo.Service) chat.Tool {
	if svc == nil {
		return nil
	}
	t, _ := chat.NewTool(
		chat.ToolDefinition{Name: "todo_write", Description: description, InputSchema: inputSchema},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var a writeArgs
			if err := json.Unmarshal([]byte(arguments), &a); err != nil {
				return fmt.Sprintf("error: invalid arguments: %s", err), nil
			}
			sessionID := turnctx.TurnSession(ctx)
			if sessionID == "" {
				return "error: no active session — cannot maintain a todo list", nil
			}
			prev, err := svc.List(ctx, sessionID)
			if err != nil {
				return "", err
			}
			if err := todo.Validate(prev, a.Todos); err != nil {
				if errors.Is(err, todo.ErrInvalid) {
					// Recoverable: surface the rule the model broke so it fixes
					// the list and retries, rather than aborting the run.
					return "Rejected — " + err.Error(), nil
				}
				return "", err
			}
			if err := svc.Replace(ctx, sessionID, a.Todos); err != nil {
				return "", err
			}
			if rendered := todo.Render(a.Todos); rendered != "" {
				return "Todo list updated:\n" + rendered, nil
			}
			return "Todo list cleared.", nil
		},
	)
	return t
}
