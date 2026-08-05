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

/**
 * Where the highlight lands after the list changes under it.
 *
 * Kept pure because this is where a hand-driven list goes wrong: the index is
 * held in state, the list is derived from a query, and typing one more character
 * can leave the index past the end — which renders as no highlight at all and an
 * Enter that opens nothing.
 */
export function clampHighlight(index: number, length: number): number {
  if (length === 0) return 0;
  return Math.min(Math.max(index, 0), length - 1);
}

/** Arrow-key movement, wrapping at both ends: a list this short is faster to
 *  reach the bottom of by pressing Up once. */
export function moveHighlight(index: number, length: number, delta: number): number {
  if (length === 0) return 0;
  return (((index + delta) % length) + length) % length;
}
