package engine

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// basePrompt is the always-on identity / behavioural preamble. It
// stays small on purpose — anything project-specific lives in
// LYRA.md and gets appended at SystemPrompt time. Anything
// user-specific lives in ~/.lyra/LYRA.md.
const basePrompt = `You are Lyra, a general-purpose AI coding agent.

You can read and modify files, run shell commands, search the
codebase, and (when configured) fetch web content. Use the
available tools to accomplish the user's task; explain your
reasoning briefly when it isn't obvious. Prefer concrete actions
over hypotheticals.

When you change files, show the change. When a tool returns an
error, read the message and adjust — don't blindly retry. If a
task is ambiguous, ask one focused question rather than guess.`

// SystemPrompt assembles the system prompt for one turn. The
// shape is:
//
//	<base prompt>
//	<user memory>     (when ~/.lyra/LYRA.md is non-empty)
//	<project memory>  (when <cwd>/LYRA.md is non-empty)
//
// User scope first because cross-project preferences should be
// readable as the agent's default tendency; project scope last so
// it can override or refine. Each section is wrapped in a clear
// markdown header so the model can tell which knowledge layer it
// came from.
//
// Engines built without a memory service simply yield the base
// prompt — tests and minimal deployments both stay valid.
func (e *Engine) SystemPrompt(ctx context.Context) string {
	return composePrompt(ctx, e.memSvc)
}

// composePrompt is the pure form behind [Engine.SystemPrompt],
// exposed unexported so the unit tests (which build stub memory
// services without a full Engine) can exercise the cascade
// directly.
func composePrompt(ctx context.Context, mem memory.Service) string {
	var b strings.Builder
	b.WriteString(basePrompt)

	if mem == nil {
		return b.String()
	}

	userMem, _ := mem.Get(ctx, memory.ScopeUser)
	if s := strings.TrimSpace(userMem); s != "" {
		b.WriteString("\n\n## User preferences (from ~/.lyra/LYRA.md)\n\n")
		b.WriteString(s)
	}

	projectMem, _ := mem.Get(ctx, memory.ScopeProject)
	if s := strings.TrimSpace(projectMem); s != "" {
		b.WriteString("\n\n## Project knowledge (from <cwd>/LYRA.md)\n\n")
		b.WriteString(s)
	}

	return b.String()
}
