package agentexec

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/planpresentation"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// agentMemoryInjectBudget bounds each prompt-resident Agent Memory block by the
// same conservative token estimate: the always-on pinned core and the
// per-message relevant recall. Items stay whole; lower-priority items beyond a
// block's budget remain available through search_memory.
const agentMemoryInjectBudget = 4096

// basePrompt is the always-on identity / behavioral preamble. It
// stays small on purpose — project-specific context lives in agent memory,
// LYRA.md, or AGENTS.md and gets appended during prompt assembly.
// Anything user-specific lives in ~/.lyra/LYRA.md.
const basePrompt = `You are Lyra, a general-purpose AI coding agent.

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
//	<user knowledge>       (~/.lyra/LYRA.md — global, user-managed)
//	<pinned agent memory>  (durable, project-scoped, agent-managed)
//	<project knowledge>    (<project-root>/LYRA.md, when distinct)
//	<workspace knowledge>  (<cwd>/LYRA.md — per-session workspace root)
//	<discovered>      (agentdoc cascade — global AGENTS.md first
//	                   (~/.lyra, ~/.agents), then project root → cwd)
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
// The optional knowledge, memory, document and Plan sources are best-effort:
// they enrich model context but never become correctness inputs for a run.
func (composer *WorkingContextComposer) composeSystemMessage(
	ctx context.Context,
	sessionID string,
	cwd string,
) (corechat.Message, error) {
	var prompt promptComposition
	prompt.append(
		basePrompt,
		contextSourceBasePrompt.source("builtin:lyra"),
	)

	var knowledgeEntries []knowledge.Entry
	if composer.config.Knowledge != nil {
		knowledgeEntries, _ = composer.config.Knowledge.Entries(ctx, cwd)
		for _, entry := range knowledgeEntries {
			if entry.Scope != knowledge.ScopeHome {
				continue
			}
			if content := strings.TrimSpace(entry.Content); content != "" {
				prompt.append(
					"## User preferences (from ~/.lyra/LYRA.md)\n\n"+content,
					contextSourceUserKnowledge.source(entry.Path),
				)
			}
		}
	}

	if composer.config.AgentMemory != nil {
		// The always-on core is the PINNED items (project + user scope). Non-pinned
		// approved memory is surfaced per turn by relevance (the recall block), so a
		// growing corpus never bloats every prompt.
		var pinned []agentmemory.Item
		if project := strings.TrimSpace(cwd); project != "" {
			items, _ := composer.config.AgentMemory.Items(ctx, agentmemory.ScopeProject, filepath.Clean(project))
			pinned = appendPinned(pinned, items)
		}
		userItems, _ := composer.config.AgentMemory.Items(ctx, agentmemory.ScopeUser, "")
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
				"## Project knowledge (from <project-root>/LYRA.md)\n\n"+content,
				contextSourceProjectKnowledge.source(entry.Path),
			)
		case knowledge.ScopeCWD:
			prompt.append(
				"## Workspace knowledge (from <cwd>/LYRA.md)\n\n"+content,
				contextSourceProjectKnowledge.source(entry.Path),
			)
		}
	}

	// AGENTS.md cascade — best-effort, silent on error so a missing or unreadable
	// instruction file never derails a turn. User Home is injected rather than
	// rediscovered inside every prompt assembly.
	if dir := strings.TrimSpace(cwd); dir != "" {
		if files, err := promptsource.DiscoverAgentDocs(ctx, dir, composer.config.UserHome); err == nil {
			newAgentDocumentsPrompt(files, agentDocPromptMaxBytes).appendTo(&prompt)
		}
	}

	composer.appendSessionPlan(ctx, &prompt, sessionID)
	return prompt.systemMessage()
}

// appendSessionPlan appends the turn's Plan when the configured reader has
// steps for the Session. Plan context is informative and remains best-effort.
func (composer *WorkingContextComposer) appendSessionPlan(
	ctx context.Context,
	prompt *promptComposition,
	sessionID string,
) {
	if composer.config.Plan == nil || sessionID == "" {
		return
	}
	steps, err := composer.config.Plan.List(ctx, sessionID)
	if err != nil || len(steps) == 0 {
		return
	}
	prompt.append(
		"## Current Plan\n\n"+planpresentation.Render(steps),
		contextSourceSessionPlan.source(sessionID),
	)
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
