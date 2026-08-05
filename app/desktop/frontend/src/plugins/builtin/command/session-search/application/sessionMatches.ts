// Jumping to a session by name.
//
// What is left of ⌘K after the command palette went: the one thing only it did.
// The sidebar lists every session but cannot filter, and it loads all of them, so
// finding one by name means scrolling. This is that, and nothing else — no
// commands (they have buttons and shortcuts, and the shortcuts pane lists them),
// no panels (the dock's own picker groups them by scope and shows what is open).

import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";

/** Enough to fill the panel twice over; past that a person narrows the query
 *  instead of scrolling, and the DOM stays bounded either way. */
const DEFAULT_LIMIT = 20;

function byNewest(a: AgentSessionSummary, b: AgentSessionSummary): number {
  if (a.time === b.time) return 0;
  return a.time < b.time ? 1 : -1;
}

/**
 * The sessions to offer, newest first.
 *
 * An empty query answers with the most recent rather than with nothing — this
 * surface exists to go somewhere, and opening it to a blank list would make the
 * common case (back to what I was just doing) the slowest one. The command
 * palette's version returned nothing on an empty query, which was right there:
 * it had commands to show instead.
 */
export function matchSessions(
  sessions: readonly AgentSessionSummary[],
  query: string,
  limit = DEFAULT_LIMIT,
): AgentSessionSummary[] {
  const needle = query.trim().toLowerCase();
  const matched =
    needle === ""
      ? [...sessions]
      : sessions.filter((session) => session.title.toLowerCase().includes(needle));
  return matched.sort(byNewest).slice(0, limit);
}
