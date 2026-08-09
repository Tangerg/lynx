package agentexec

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/planpresentation"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// agentMemoryInjectBudget bounds the always-on curated-memory block whole-inject
// (pinned items first, then recent). The extractor already caps the auto item
// set well under this; the headroom absorbs a few user-pinned items. Retrieval
// (a later batch) surfaces anything beyond the budget on demand.
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

// systemPrompt assembles the system prompt for one turn. Global
// context loads first, project context second, so project knowledge
// extends and overrides the global layer:
//
//	<base prompt>
//	<user knowledge>       (~/.lyra/LYRA.md — global, user-managed)
//	<pinned agent memory>  (SQLite — project-scoped, agent-managed)
//	<project knowledge>    (<cwd>/LYRA.md — per-session project dir)
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
// appendPlan appends the turn's session Plan to the prompt when a Plan reader
// is wired and the session has Steps. Best-effort: a missing session
// id or a store error silently skips — the Plan is context for the
// model, never a correctness input, so it must never derail prompt assembly.
// Kept off composePrompt so that function stays focused on the knowledge /
// AGENTS.md cascade (and its direct unit tests need no plan stub).
func appendPlanForSession(ctx context.Context, prompt string, plan PlanReader, sessionID string) string {
	if plan == nil || sessionID == "" {
		return prompt
	}
	steps, err := plan.List(ctx, sessionID)
	if err != nil || len(steps) == 0 {
		return prompt
	}
	return prompt + "\n\n## Current Plan\n\n" + planpresentation.Render(steps)
}

// composePrompt assembles the immutable instruction layers for a fresh root.
func composePrompt(ctx context.Context, knowledgeReader KnowledgeReader, agentMemory AgentMemoryReader, cwd, userHome string) string {
	var b strings.Builder
	b.WriteString(basePrompt)

	if knowledgeReader != nil {
		userKnowledge, _ := knowledgeReader.Get(ctx, knowledge.ScopeUser, "")
		if s := strings.TrimSpace(userKnowledge); s != "" {
			b.WriteString("\n\n## User preferences (from ~/.lyra/LYRA.md)\n\n")
			b.WriteString(s)
		}
	}

	if agentMemory != nil {
		// The always-on core is the PINNED items (project + user scope). Non-pinned
		// approved memory is surfaced per turn by relevance (the recall block), so a
		// growing corpus never bloats every prompt.
		var pinned []agentmemory.Item
		if project := strings.TrimSpace(cwd); project != "" {
			items, _ := agentMemory.Items(ctx, agentmemory.ScopeProject, filepath.Clean(project))
			pinned = appendPinned(pinned, items)
		}
		userItems, _ := agentMemory.Items(ctx, agentmemory.ScopeUser, "")
		pinned = appendPinned(pinned, userItems)
		if s := renderPinnedMemory(pinned, agentMemoryInjectBudget); s != "" {
			b.WriteString("\n\n## Pinned memory (managed by Lyra)\n\n")
			b.WriteString(s)
		}
	}

	if knowledgeReader != nil {
		projectKnowledge, _ := knowledgeReader.Get(ctx, knowledge.ScopeProject, cwd)
		if s := strings.TrimSpace(projectKnowledge); s != "" {
			b.WriteString("\n\n## Project knowledge (from <cwd>/LYRA.md)\n\n")
			b.WriteString(s)
		}
	}

	// AGENTS.md cascade — best-effort, silent on error so a missing or unreadable
	// instruction file never derails a turn. User Home is injected rather than
	// rediscovered inside every prompt assembly.
	if dir := strings.TrimSpace(cwd); dir != "" {
		if files, err := promptsource.DiscoverAgentDocs(ctx, dir, userHome); err == nil {
			if rendered := renderAgentDocs(files, agentDocPromptMaxBytes); rendered != "" {
				b.WriteString("\n\n## Project context (from AGENTS.md cascade)\n\n")
				b.WriteString(rendered)
			}
		}
	}

	return b.String()
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
