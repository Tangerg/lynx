package agentexec

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/planpresentation"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/scope/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/scope/app/runtime/internal/domain/knowledge"
	plandomain "github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	corechat "github.com/Tangerg/scope/core/chat"
)

// agentMemoryInjectBudget bounds each prompt-resident Agent Memory block by the
// same conservative token estimate: the always-on pinned core and the
// per-message relevant recall. Items stay whole; lower-priority items beyond a
// block's budget remain available through search_memory.
const agentMemoryInjectBudget = 4096

// basePrompt is the always-on identity / behavioral preamble. It
// stays small on purpose — project-specific context lives in agent memory,
// SCOPEAPP.md, or AGENTS.md and gets appended during prompt assembly.
// Anything user-specific lives in ~/.scopeapp/SCOPEAPP.md.
const basePrompt = `You are ScopeApp, a general-purpose AI coding agent.

You can read and modify files, run shell commands, search the
codebase, and (when configured) fetch web content. Use the
available tools to accomplish the user's task; explain your
reasoning briefly when it isn't obvious. Prefer concrete actions
over hypotheticals.

When you change files, show the change. When a tool returns an
error, read the message and adjust — don't blindly retry. If a
task is ambiguous, ask one focused question rather than guess.`

// composeSystemMessage assembles the system prompt for one turn. Global
// context loads from broadest to narrowest so more local knowledge extends and
// overrides the broader layer:
//
//	<base prompt>
//	<user knowledge>       (~/.scopeapp/SCOPEAPP.md — global, user-managed)
//	<pinned agent memory>  (durable, project-scoped, agent-managed)
//	<project knowledge>    (<project-root>/SCOPEAPP.md, when distinct)
//	<workspace knowledge>  (<cwd>/SCOPEAPP.md — per-session workspace root)
//	<discovered>      (agentdoc cascade — global AGENTS.md first
//	                   (~/.scopeapp, ~/.agents), then project root → cwd)
//
// The project side anchors to the TURN's working directory — the
// session cwd carried in application context ([executionctx.CWD]), the same seam
// the fs/shell/skill tools follow — so a session opened on
// project A briefs the model about project A regardless of where the runtime
// server process was started.
//
// KnowledgeReader is the prompt's human-authored knowledge surface; agentdoc is the
// read-only cross-tool AGENTS.md convention.
// Engines built without knowledge or agent memory simply yield the base prompt +
// discovered files.
// A configured Knowledge reader supplies one complete cascade, so a read error
// fails prompt construction instead of silently deleting user instructions.
// Memory remains best-effort enrichment. Current Goal and Plan are authoritative
// Session state: configured read failures fail construction instead of silently
// retaining or deleting an old snapshot. An existing Agent document is likewise
// authoritative authored input: invalid or individually unprojectable material
// fails construction instead of silently deleting instructions.
func (w *WorkingContextComposer) composeSystemMessage(
	ctx context.Context,
	cwd string,
) (corechat.Message, error) {
	var prompt promptComposition
	prompt.append(
		basePrompt,
		contextSourceBasePrompt.source("builtin:scopeapp"),
	)

	var knowledgeEntries []knowledge.Entry
	if w.config.Knowledge != nil {
		var err error
		knowledgeEntries, err = w.config.Knowledge.Entries(ctx, cwd)
		if err != nil {
			return corechat.Message{}, fmt.Errorf("agentexec: load knowledge cascade: %w", err)
		}
		for _, entry := range knowledgeEntries {
			if entry.Scope != knowledge.ScopeHome {
				continue
			}
			if content := strings.TrimSpace(entry.Content); content != "" {
				prompt.append(
					"## User preferences (from ~/.scopeapp/SCOPEAPP.md)\n\n"+content,
					contextSourceUserKnowledge.source(entry.Path),
				)
			}
		}
	}

	if w.config.AgentMemory != nil {
		// The always-on core is the PINNED items (project + user scope). Non-pinned
		// approved memory is surfaced per turn by relevance (the recall block), so a
		// growing corpus never bloats every prompt.
		var pinned []agentmemory.Item
		if project := strings.TrimSpace(cwd); project != "" {
			items, _ := w.config.AgentMemory.Items(ctx, agentmemory.ScopeProject, filepath.Clean(project))
			pinned = appendPinned(pinned, items)
		}
		userItems, _ := w.config.AgentMemory.Items(ctx, agentmemory.ScopeUser, "")
		pinned = appendPinned(pinned, userItems)
		newPinnedMemoryPrompt(pinned, agentMemoryInjectBudget).appendTo(&prompt)
	}

	for _, entry := range knowledgeEntries {
		content := strings.TrimSpace(entry.Content)
		if content == "" {
			continue
		}
		switch entry.Scope {
		case knowledge.ScopeProjectRoot:
			prompt.append(
				"## Project knowledge (from <project-root>/SCOPEAPP.md)\n\n"+content,
				contextSourceProjectKnowledge.source(entry.Path),
			)
		case knowledge.ScopeCWD:
			prompt.append(
				"## Workspace knowledge (from <cwd>/SCOPEAPP.md)\n\n"+content,
				contextSourceProjectKnowledge.source(entry.Path),
			)
		}
	}

	// AGENTS.md cascade. Missing/empty sources contribute nothing; an existing
	// invalid source remains observable so a turn cannot run under silently
	// incomplete instructions. User Home is injected rather than rediscovered
	// inside every prompt assembly.
	if dir := strings.TrimSpace(cwd); dir != "" {
		files, err := promptsource.DiscoverAgentDocs(ctx, dir, w.config.UserHome)
		if err != nil {
			return corechat.Message{}, fmt.Errorf("agentexec: load agent documents: %w", err)
		}
		documents, err := newAgentDocumentsPrompt(files, agentDocPromptMaxBytes)
		if err != nil {
			return corechat.Message{}, fmt.Errorf("agentexec: render agent documents: %w", err)
		}
		documents.appendTo(&prompt)
	}

	return prompt.systemMessage()
}

// CurrentSessionState returns the complete model-facing snapshot of durable
// Session state that can change during one Interaction. Goal precedes Plan in a
// canonical order; each is an isolated message so the reducer can replace the
// old snapshot without changing frozen deployment instructions.
func (w *WorkingContextComposer) CurrentSessionState(
	ctx context.Context,
	sessionID string,
) ([]corechat.Message, error) {
	if w == nil {
		return nil, errors.New("agentexec: working-context composer is nil")
	}
	messages := make([]corechat.Message, 0, 2)
	if w.config.Goal != nil && sessionID != "" {
		current, found, err := w.config.Goal.Current(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("agentexec: load current Session Goal: %w", err)
		}
		if found {
			if err := current.ValidateSnapshot(); err != nil {
				return nil, fmt.Errorf("agentexec: current Session Goal is invalid: %w", err)
			}
			if current.SessionID != sessionID {
				return nil, errors.New("agentexec: current Session Goal belongs to another Session")
			}
			var prompt promptComposition
			prompt.append(
				"## Current Autonomous Goal\n\nObjective: "+strconv.Quote(current.Objective)+
					"\nStatus: "+string(current.Status)+
					"\n\nThis snapshot is current for this model call. Use get_goal for full budget, usage, and reason details.",
				contextSourceSessionGoal.source(sessionID),
			)
			message, err := prompt.systemMessage()
			if err != nil {
				return nil, err
			}
			messages = append(messages, message)
		}
	}
	currentPlan, found, err := w.currentSessionPlan(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if found {
		messages = append(messages, currentPlan)
	}
	return messages, nil
}

func (w *WorkingContextComposer) currentSessionPlan(
	ctx context.Context,
	sessionID string,
) (corechat.Message, bool, error) {
	if w.config.Plan == nil || sessionID == "" {
		return corechat.Message{}, false, nil
	}
	steps, listErr := w.config.Plan.List(ctx, sessionID)
	if listErr != nil {
		return corechat.Message{}, false, fmt.Errorf("agentexec: load current Session Plan: %w", listErr)
	}
	if validationErr := plandomain.ValidateSteps(steps); validationErr != nil {
		return corechat.Message{}, false, fmt.Errorf("agentexec: current Session Plan is invalid: %w", validationErr)
	}
	if len(steps) == 0 {
		return corechat.Message{}, false, nil
	}
	var prompt promptComposition
	prompt.append(
		"## Current Plan\n\n"+planpresentation.Render(steps),
		contextSourceSessionPlan.source(sessionID),
	)
	message, err := prompt.systemMessage()
	if err != nil {
		return corechat.Message{}, false, err
	}
	return message, true, nil
}

// appendPinned appends the pinned items of src to dst.
func appendPinned(dst, src []agentmemory.Item) []agentmemory.Item {
	for _, item := range src {
		if item.Pinned {
			dst = append(dst, item)
		}
	}
	return dst
}
