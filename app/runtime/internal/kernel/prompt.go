package kernel

import (
	"context"
	"os"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentdoc"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turnctx"
)

// basePrompt is the always-on identity / behavioral preamble. It
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

// SystemPrompt assembles the system prompt for one turn. Global
// context loads first, project context second, so project knowledge
// extends and overrides the global layer:
//
//	<base prompt>
//	<user memory>     (~/.lyra/LYRA.md — global, user-managed)
//	<project memory>  (<cwd>/LYRA.md — per-session project dir)
//	<discovered>      (agentdoc cascade — global AGENTS.md first
//	                   (~/.lyra, ~/.agents), then project root → cwd)
//
// The project side anchors to the TURN's working directory — the
// session cwd seeded on the process blackboard ([turnCwd]), the same
// seam the fs/shell/skill tools follow — so a session opened on
// project A briefs the model about project A regardless of where
// `lyra serve` was started.
//
// knowledge.Service is Lyra's writable surface (`lyra memory edit`);
// agentdoc is the read-only cross-tool AGENTS.md convention. Engines
// built without a memory service simply yield the base prompt +
// discovered files.
func (e *Engine) SystemPrompt(ctx context.Context) string {
	prompt := composePrompt(ctx, e.knowledge, turnctx.TurnCwd(ctx, e.workdir))
	return appendTodos(ctx, prompt, e.todos)
}

// appendTodos appends the turn's session todo list to prompt when a todo
// store is wired and the session has items. Best-effort: a missing session
// id or a store error silently skips — the list is a convenience for the
// model, never a correctness input, so it must never derail prompt assembly.
// Kept off composePrompt so that function stays focused on the knowledge /
// AGENTS.md cascade (and its direct unit tests need no todo stub).
func appendTodos(ctx context.Context, prompt string, todos todo.Store) string {
	if todos == nil {
		return prompt
	}
	sessionID := turnctx.TurnSession(ctx)
	if sessionID == "" {
		return prompt
	}
	items, err := todos.List(ctx, sessionID)
	if err != nil || len(items) == 0 {
		return prompt
	}
	return prompt + "\n\n## Current todo list (you maintain this via todo_write)\n\n" + todo.Render(items)
}

// composePrompt is the pure form behind [Engine.SystemPrompt],
// exposed unexported so the unit tests (which build stub memory
// services without a full Engine) can exercise the cascade
// directly.
func composePrompt(ctx context.Context, mem knowledge.Service, cwd string) string {
	var b strings.Builder
	b.WriteString(basePrompt)

	if mem != nil {
		userMem, _ := mem.Get(ctx, knowledge.ScopeUser, "")
		if s := strings.TrimSpace(userMem); s != "" {
			b.WriteString("\n\n## User preferences (from ~/.lyra/LYRA.md)\n\n")
			b.WriteString(s)
		}

		projectMem, _ := mem.Get(ctx, knowledge.ScopeProject, cwd)
		if s := strings.TrimSpace(projectMem); s != "" {
			b.WriteString("\n\n## Project knowledge (from <cwd>/LYRA.md)\n\n")
			b.WriteString(s)
		}
	}

	// AGENTS.md cascade — best-effort, silent on error so a missing
	// file or unreadable home dir never derails a turn.
	if dir := resolveCwd(cwd); dir != "" {
		home, _ := os.UserHomeDir()
		if files, err := agentdoc.Discover(ctx, dir, home); err == nil {
			if rendered := agentdoc.Render(files, agentdoc.DefaultMaxBytes); rendered != "" {
				b.WriteString("\n\n## Project context (from AGENTS.md cascade)\n\n")
				b.WriteString(rendered)
			}
		}
	}

	return b.String()
}

// resolveCwd falls back to the process cwd when the turn carried no
// working directory. Returns "" only when both sources fail — in
// which case agentdoc discovery silently skips.
func resolveCwd(cwd string) string {
	if cwd != "" {
		return cwd
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
