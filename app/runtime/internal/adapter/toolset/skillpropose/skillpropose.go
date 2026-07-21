// Package skillpropose exposes the propose_skill tool: the governed way an agent
// suggests a new reusable skill. A proposal is validated against the SKILL.md
// spec, statically scanned for obviously-dangerous content, staged as a draft,
// and then gated behind a HUMAN approval (the same QuestionInterrupt mechanism
// exit_plan_mode uses). Only on approval is the draft promoted into the active
// skill set; the agent can never self-publish. It is the write counterpart to
// the read-only skill tool.
package skillpropose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/tools"
)

const (
	toolName     = "propose_skill"
	approveLabel = "Approve"
	rejectLabel  = "Reject"
)

// Authoring is the write capability the tool needs: stage a draft, promote an
// approved one, or discard a rejected one. The infra skillauthoring.Store
// implements it.
type Authoring interface {
	Enabled() bool
	SaveDraft(ctx context.Context, draft skills.Draft) (skills.DraftHandle, error)
	Promote(ctx context.Context, handle skills.DraftHandle) error
	DiscardDraft(ctx context.Context, handle skills.DraftHandle) error
}

const description = `Propose a new reusable skill (a SKILL.md the agent can later
load) to be saved to the user's global skill library. Use this when you've worked
out a repeatable procedure worth keeping for future sessions — not for one-off
work.

Propose a class-level, reusable procedure ("how to do X in this kind of project"),
and prefer a general umbrella skill over a narrow one-off. Do NOT propose a skill
for: environment/dependency failures or their workarounds, negative assertions
about tools ("tool X doesn't work"), errors already resolved in this session, or
anything obvious from the project's source or standard docs.

The proposal is NOT applied automatically: it is shown to the user for approval.
Only if they approve is the skill added; otherwise it is discarded. Provide a
lowercase-hyphenated name, a one-line description of what the skill does and when
to use it, and the skill body in markdown.`

type proposeArgs struct {
	Name        string `json:"name" jsonschema:"required" jsonschema_description:"Skill id: lowercase alphanumerics joined by single hyphens (e.g. git-bisect-helper)."`
	Description string `json:"description" jsonschema:"required" jsonschema_description:"One line: what the skill does and when to use it — the text the agent reads to decide relevance."`
	Body        string `json:"body" jsonschema:"required" jsonschema_description:"The skill instructions in markdown (the SKILL.md body)."`
}

func (a proposeArgs) draft() skills.Draft {
	return skills.Draft{Name: a.Name, Description: a.Description, Body: a.Body}
}

type tool struct {
	store     Authoring
	interrupt interrupts.Func
}

// New builds the propose_skill tool. A nil store (or one reporting Enabled()
// false) yields a nil tool so the caller omits the feature; a nil interrupt
// resolves to the unavailable one (the tool then can't gate and reports so).
func New(store Authoring, interrupt interrupts.Func) (tools.Tool, error) {
	if store == nil || !store.Enabled() {
		return nil, nil
	}
	if interrupt == nil {
		interrupt = interrupts.Unavailable
	}
	return tools.New[proposeArgs, string](
		tools.Config{Name: toolName, Description: description},
		(&tool{store: store, interrupt: interrupt}).propose,
	)
}

func (t *tool) propose(ctx context.Context, in proposeArgs) (string, error) {
	draft := in.draft()
	if err := draft.Validate(); err != nil {
		// Recoverable: the agent fixes the proposal and retries rather than aborting.
		return "Rejected — " + err.Error(), nil
	}
	if reason, dangerous := draft.Scan(); dangerous {
		return "Rejected — " + reason + ". Rewrite the skill without it.", nil
	}
	arguments, err := in.arguments()
	if err != nil {
		return "", err
	}
	prompt := proposePrompt(draft, arguments)
	pending := runs.Interrupt{Kind: runs.QuestionInterruptKind, Question: &prompt}
	if err := pending.Validate(); err != nil {
		return "", fmt.Errorf("propose_skill: %w", err)
	}
	handle, err := t.store.SaveDraft(ctx, draft)
	if err != nil {
		return "", err
	}
	res, err := t.interrupt(ctx,
		interrupts.InterruptKey(string(runs.QuestionInterruptKind), toolName, arguments),
		pending,
	)
	if err != nil {
		return "", errors.Join(err, t.store.DiscardDraft(context.WithoutCancel(ctx), handle))
	}

	if selectedChoice(res.Answer) != approveLabel {
		if err := t.store.DiscardDraft(context.WithoutCancel(ctx), handle); err != nil {
			return "", fmt.Errorf("propose_skill: discard rejected draft %q: %w", draft.Name, err)
		}
		return "The user declined the skill proposal; it was not added.", nil
	}
	if err := t.store.Promote(ctx, handle); err != nil {
		return "", err
	}
	return "The user approved the skill '" + draft.Name + "'; it was added to the global skill library and is now loadable via the skill tool.", nil
}

func proposePrompt(draft skills.Draft, arguments string) runs.QuestionPrompt {
	preview := draft.Description + "\n\n" + draft.Body
	return runs.QuestionPrompt{
		ToolName:  toolName,
		Arguments: arguments,
		Questions: []runs.QuestionSpec{{
			Question: "The agent proposes a new skill \"" + draft.Name + "\". Add it to your skills?\n\n" + preview,
			Header:   "New skill",
			Options: []runs.QuestionOptionSpec{
				{Label: approveLabel, Description: "Save this skill to your global library"},
				{Label: rejectLabel, Description: "Discard the proposal"},
			},
		}},
	}
}

func (a proposeArgs) arguments() (string, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("propose_skill: encode arguments: %w", err)
	}
	return string(b), nil
}

func selectedChoice(answer map[string][]string) string {
	if v := answer[interrupts.QuestionFieldName(0)]; len(v) > 0 {
		return v[0]
	}
	return ""
}
