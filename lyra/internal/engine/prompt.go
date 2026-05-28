package engine

import (
	"context"
	"os"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/agentdoc"
	"github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// basePrompt is the always-on identity / behavioural preamble. It
// stays small on purpose — anything project-specific lives in
// LYRA.md / AGENTS.md and gets appended at SystemPrompt time.
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

// SystemPrompt assembles the system prompt for one turn. The
// shape is:
//
//	<base prompt>
//	<user memory>     (memory.Service.Get(ScopeUser)  — user-managed)
//	<project memory>  (memory.Service.Get(ScopeProject) — user-managed)
//	<discovered>      (agentdoc cascade — AGENTS.md walked from
//	                   cwd up to project root + standard user dirs)
//
// memory.Service is Lyra's writable surface (`lyra memory edit`);
// agentdoc is the read-only cross-tool AGENTS.md convention. Both
// flow in so users get both the curated notes AND the project's
// committed AGENTS.md.
//
// Engines built without a memory service simply yield the base
// prompt + discovered files.
func (e *Engine) SystemPrompt(ctx context.Context) string {
	return composePrompt(ctx, e.memSvc, e.workdir)
}

// composePrompt is the pure form behind [Engine.SystemPrompt],
// exposed unexported so the unit tests (which build stub memory
// services without a full Engine) can exercise the cascade
// directly.
func composePrompt(ctx context.Context, mem memory.Service, workdir string) string {
	var b strings.Builder
	b.WriteString(basePrompt)

	if mem != nil {
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
	}

	// AGENTS.md cascade — best-effort, silent on error so a missing
	// file or unreadable home dir never derails a turn.
	if cwd := resolveCwd(workdir); cwd != "" {
		home, _ := os.UserHomeDir()
		if files, err := agentdoc.Discover(ctx, cwd, home); err == nil {
			if rendered := agentdoc.Render(files, agentdoc.DefaultMaxBytes); rendered != "" {
				b.WriteString("\n\n## Project context (from AGENTS.md cascade)\n\n")
				b.WriteString(rendered)
			}
		}
	}

	return b.String()
}

// resolveCwd picks the engine's configured workdir when set,
// falling back to the process cwd. Returns "" only when both
// sources fail — in which case agentdoc discovery silently skips.
func resolveCwd(workdir string) string {
	if workdir != "" {
		return workdir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
