import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";

// The command palette doubles as a session-jump surface (ENTRY_POINTS T3.1a):
// on a non-empty query it offers matching sessions by title alongside commands.
// An empty query stays commands-only — the palette shouldn't dump the whole
// session list before the user types (the "空查询不展开" rule). Results are
// capped so a large history can't flood the list or the DOM.
const DEFAULT_LIMIT = 20;

export function filterSessionsForPalette(
  sessions: AgentSessionSummary[],
  query: string,
  limit = DEFAULT_LIMIT,
): AgentSessionSummary[] {
  const q = query.trim().toLowerCase();
  if (q === "") return [];
  const matches: AgentSessionSummary[] = [];
  for (const session of sessions) {
    if (session.title.toLowerCase().includes(q)) {
      matches.push(session);
      if (matches.length >= limit) break;
    }
  }
  return matches;
}
