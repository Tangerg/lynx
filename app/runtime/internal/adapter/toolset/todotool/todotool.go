// Package todotool exposes the model-facing todo_write tool over its narrow
// persistence view. It is named todotool (not todo) to avoid colliding with the
// domain/todo domain package it builds on — the same disambiguation the
// lsptools package uses for the codeintel adapter.
//
// The tool is the LLM-facing presentation of the todo domain: it parses the
// model's full-list replacement, runs the domain's progress-integrity check
// ([todo.Validate]), and persists via the store — keeping the rules in the
// domain and only the wire shape + recovery messaging here (the same split as
// the editguard tool wrappers).
package todotool

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/todopresentation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/tools"
)

const description = `Maintain a structured task list for the current session.

Use this to plan and track multi-step work so progress survives across turns.
Call it whenever the plan changes: when you start a task, finish one, discover
new steps, or re-scope.

Pass the COMPLETE updated list every time — it REPLACES the stored list (it is
not a delta). Rules, enforced by the runtime:
  - Keep exactly ONE task "in_progress" at a time; the rest "pending" or "completed".
  - Use blocked_reason when a pending/in_progress task is stuck on a concrete blocker.
  - Use next_action for the immediate next move on pending/in_progress tasks.
  - Mark a task "completed" ONLY when it is fully done (tests pass, no errors),
    and complete them ONE AT A TIME — do not flip several to completed in one call.
  - Completed tasks must not carry blocked_reason or next_action.
Skip this tool for trivial single-step requests; it is for real multi-step work.`

// writeArgs is the model-facing argument shape; [tools.New] derives the
// JSON schema from it and decodes calls back into it, so the advertised schema
// and parsed value cannot drift. The items mirror [todo.Item] with the
// LLM-facing descriptions kept here (out of the domain type); the handler maps
// them across.
type writeArgs struct {
	Todos []todoItemArg `json:"todos" jsonschema:"required" jsonschema_description:"The complete task list, in order. Replaces the stored list."`
}

type todoItemArg struct {
	Content       string `json:"content" jsonschema:"required" jsonschema_description:"Imperative description of the task (e.g. \"Add the retry guard to fetch()\")."`
	Status        string `json:"status" jsonschema:"required,enum=pending,enum=in_progress,enum=completed" jsonschema_description:"pending = not started; in_progress = actively working (at most one); completed = fully done."`
	BlockedReason string `json:"blocked_reason,omitempty" jsonschema_description:"Why this pending/in_progress task is blocked. Leave empty when it is not blocked."`
	NextAction    string `json:"next_action,omitempty" jsonschema_description:"The immediate next action for this pending/in_progress task. Leave empty for completed tasks."`
}

// items maps the parsed args to the domain type.
func (a writeArgs) items() []todo.Item {
	out := make([]todo.Item, len(a.Todos))
	for i, t := range a.Todos {
		out[i] = todo.Item{
			Content:       t.Content,
			Status:        todo.Status(t.Status),
			BlockedReason: t.BlockedReason,
			NextAction:    t.NextAction,
		}
	}
	return out
}

type tool struct {
	store Store
}

// Store is the todo_write tool's complete persistence need. Session lifecycle
// cleanup is intentionally outside this model-facing adapter.
type Store interface {
	List(ctx context.Context, sessionID string) ([]todo.Item, error)
	Replace(ctx context.Context, sessionID string, items []todo.Item) error
}

// New builds the todo_write tool over store. It returns a nil tool and nil
// error when store is nil so the caller can simply omit the tool — the feature
// is disabled, not a broken tool. The session id is read per-call off the
// turn's blackboard ([turnctx.TurnSession]), so one tool instance serves every
// session.
func New(store Store) (tools.Tool, error) {
	if store == nil {
		return nil, nil
	}
	return tools.New[writeArgs, string](
		tools.Config{Name: "todo_write", Description: description},
		(&tool{store: store}).write,
	)
}

func (t *tool) write(ctx context.Context, a writeArgs) (string, error) {
	sessionID := turnctx.TurnSession(ctx)
	if sessionID == "" {
		return "error: no active session — cannot maintain a todo list", nil
	}
	items := a.items()
	prev, err := t.store.List(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if err := todo.Validate(prev, items); err != nil {
		if errors.Is(err, todo.ErrInvalid) {
			// Recoverable: surface the rule the model broke so it fixes
			// the list and retries, rather than aborting the run.
			return "Rejected — " + err.Error(), nil
		}
		return "", err
	}
	if err := t.store.Replace(ctx, sessionID, items); err != nil {
		return "", err
	}
	if rendered := todopresentation.Render(items); rendered != "" {
		return "Todo list updated:\n" + rendered, nil
	}
	return "Todo list cleared.", nil
}
