// Everything, in every session, that is waiting on a person.
//
// The sidebar already puts a dot on a session that is waiting, which answers
// "is this one blocked" for the session you are looking at. It does not answer
// "what is waiting on me right now" — for that you scan the list hunting dots,
// and an approval card that has scrolled out of its own transcript says nothing
// at all. This is the read model behind the surface that does answer it.
//
// Sourced from `interrupts.list` with NO session filter: the runtime already
// keeps the waiting sets ordered longest-wait-first across the whole install,
// which is the order a queue of things blocking you wants to be read in. Deriving
// the same list from session status would lose both the ordering and what each
// session is actually waiting FOR.

import type { AgentInterrupt, AgentPendingInterruptSet } from "@/plugins/sdk";
import type { PendingInterruptKind } from "@/plugins/sdk/types/agentSessionView";
import { createDataQuery } from "@/plugins/sdk";

export const PENDING_WORK_KEY = "pendingWork";

export interface PendingWorkItem {
  /** Interrupt sets resume as a unit, so the set — not the interrupt — is the
   *  row: one click, one destination, one resume. */
  id: string;
  sessionId: string;
  rootRunId: string;
  kind: PendingInterruptKind;
  /** The tool an approval is for, or the question being asked. Already the
   *  reader's words: nothing here is a catalog key, because the subject is the
   *  agent's own text. */
  subject: string;
  /** How many more asks are in the same set, beyond the one named above. */
  more: number;
  /** ISO-8601, from the wire. Formatted at render, like every other stamp. */
  waitingSince: string;
}

/** The one line of the ask a row shows: the tool an approval is for, or the first
 *  of a question's fields — a Question is a LIST of prompts, and the rest of them
 *  are part of the same ask. */
function subjectOf(interrupt: AgentInterrupt): string {
  return interrupt.type === "question"
    ? (interrupt.payload.question.fields[0]?.prompt ?? "")
    : interrupt.payload.tool.name;
}

/**
 * One row per waiting set.
 *
 * A set can hold several asks — a batch of tool calls approved together. The row
 * names the first and counts the rest rather than splitting the set into rows,
 * because resuming answers the whole set at once: N rows that all disappear on
 * one click would be N lies about how much work is left.
 */
export function pendingWorkItems(sets: readonly AgentPendingInterruptSet[]): PendingWorkItem[] {
  const items: PendingWorkItem[] = [];
  for (const set of sets) {
    const first = set.interrupts[0];
    if (!first) continue;
    items.push({
      id: `${set.sessionId}:${set.rootRunId}`,
      sessionId: set.sessionId,
      rootRunId: set.rootRunId,
      kind: first.type,
      subject: subjectOf(first),
      more: set.interrupts.length - 1,
      waitingSince: set.createdAt,
    });
  }
  return items;
}

export const usePendingWork = createDataQuery<PendingWorkItem[]>(PENDING_WORK_KEY);
