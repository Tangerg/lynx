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

import type { PendingInterruptKind } from "@/plugins/sdk/types/agentSessionView";
import { createDataQuery } from "@/plugins/sdk";

export const PENDING_WORK_KEY = "pendingWork";

/** What the agent stopped to ask a PERSON for — the interrupt vocabulary minus
 *  `toolResult`, which is the runtime asking the runtime. Derived rather than
 *  respelled: a third copy of the union is a third thing to update when the
 *  protocol grows a kind. */
export type PendingWorkKind = Exclude<PendingInterruptKind, "toolResult">;

export interface PendingWorkItem {
  /** Interrupt sets resume as a unit, so the set — not the interrupt — is the
   *  row: one click, one destination, one resume. */
  id: string;
  sessionId: string;
  rootRunId: string;
  kind: PendingWorkKind;
  /** The tool an approval is for, or the question being asked. Already the
   *  reader's words: nothing here is a catalog key, because the subject is the
   *  agent's own text. */
  subject: string;
  /** How many more asks are in the same set, beyond the one named above. */
  more: number;
  /** ISO-8601, from the wire. Formatted at render, like every other stamp. */
  waitingSince: string;
}

/** The wire shape this reads, in this context's own words — the interrupt union
 *  narrowed to the two kinds a person answers. */
export interface PendingInterruptSetLike {
  sessionId: string;
  rootRunId: string;
  createdAt: string;
  interrupts: readonly {
    type: string;
    payload?: {
      tool?: { name?: string };
      /** A Question is a list of FIELDS, each with its own prompt — there is no
       *  question-level text to show, so the first field's prompt is the row's
       *  subject and the rest are part of the same ask. */
      question?: { fields?: readonly { prompt?: string }[] };
    };
  }[];
}

function answerable(set: PendingInterruptSetLike) {
  return set.interrupts.filter((i) => i.type === "approval" || i.type === "question");
}

/**
 * One row per waiting set.
 *
 * A set can hold several asks — a batch of tool calls approved together. The row
 * names the first and counts the rest rather than splitting the set into rows,
 * because resuming answers the whole set at once: N rows that all disappear on
 * one click would be N lies about how much work is left.
 */
export function pendingWorkItems(sets: readonly PendingInterruptSetLike[]): PendingWorkItem[] {
  const items: PendingWorkItem[] = [];
  for (const set of sets) {
    const asks = answerable(set);
    const first = asks[0];
    if (!first) continue;
    items.push({
      id: `${set.sessionId}:${set.rootRunId}`,
      sessionId: set.sessionId,
      rootRunId: set.rootRunId,
      kind: first.type === "question" ? "question" : "approval",
      subject:
        first.type === "question"
          ? (first.payload?.question?.fields?.[0]?.prompt ?? "")
          : (first.payload?.tool?.name ?? ""),
      more: asks.length - 1,
      waitingSince: set.createdAt,
    });
  }
  return items;
}

export const usePendingWork = createDataQuery<PendingWorkItem[]>(PENDING_WORK_KEY);
