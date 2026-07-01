import { useCallback, useRef, useState } from "react";
import { asItemId, asRunId, type InterruptResponse } from "@/rpc";
import type { ResolvePatch } from "../ports/viewState";
import { agentSessionState } from "../ports/sessionState";
import { agentViewState } from "../ports/viewState";

// Shared HITL resume scaffold (API.md §6, R-model) behind useApprovalSubmit and
// useQuestionAnswer — the parts that must behave identically for every interrupt
// kind:
//  - pin the owning session at mount: the card renders from the active session's
//    slice (so activeSessionId == owner here), and a fast tab switch between
//    render and click must not redirect the resume/resolve onto another session
//    (reading activeSessionId at click time could);
//  - a one-shot `pending` latch the card settles its optimistic state from;
//  - the guard — missing ids or already-pending → no-op; absent ids mean the
//    card is a decorative preview;
//  - the resume call whose store-level settle (resolveInterrupt) is DEFERRED to
//    the run-started callback — see resumeInterrupt below.
// Each caller supplies, per submit, the pending marker (so the card knows which
// action is settling), the wire response payload, and the resolveInterrupt patch.

/**
 * Fire a HITL resume for one open interrupt and DEFER the optimistic settle to
 * the run-started callback: `resolveInterrupt` runs only once `runs.resume` has
 * actually opened the continuation, so a channel-a failure (rejected resume,
 * §8.1) leaves the interrupt intact + the card retryable. Returns false (a
 * no-op) when the session has no resume binding.
 *
 * The single source of this deferred-settle semantic — shared by the per-card
 * `useInterruptResume` hook (optimistic card state) and the keyboard-path
 * `submitPendingApproval` (no card). `hooks.onSettled` runs after the deferred
 * settle; `hooks.onError` on a channel-a failure — each caller uses them to
 * clear its own in-flight latch.
 */
export function resumeInterrupt(
  sessionId: string,
  parentRunId: string,
  itemId: string,
  response: InterruptResponse["response"],
  settled: ResolvePatch,
  hooks?: { onSettled?: () => void; onError?: () => void },
): boolean {
  const sessionResume = agentViewState().getSession(sessionId)?.resume;
  if (!sessionResume) return false;
  sessionResume(
    asRunId(parentRunId),
    [{ itemId: asItemId(itemId), response }],
    () => {
      agentViewState().resolveInterrupt(sessionId, itemId, settled);
      hooks?.onSettled?.();
    },
    () => hooks?.onError?.(),
  );
  return true;
}

export function useInterruptResume<P>(parentRunId?: string, itemId?: string) {
  const [pending, setPending] = useState<P | null>(null);
  const [sessionId] = useState(() => agentSessionState().getActiveSessionId());
  // Synchronous one-shot latch. `pending` state only updates on the next render,
  // so two submits in the same tick (a fast double-click landing before the card
  // disables its buttons) would both pass a `pending`-based guard and fire two
  // runs.resume. The ref closes that window — parity with useAgentSession.send's
  // `starting` latch. Cleared only on channel-a failure (card stays retryable);
  // on success it stays latched, since the interrupt is now resolved.
  const submitted = useRef(false);

  const resume = useCallback(
    (marker: P, response: InterruptResponse["response"], settled: ResolvePatch) => {
      if (!parentRunId || !itemId || submitted.current) return;
      submitted.current = true;
      setPending(marker);
      const rollback = () => {
        submitted.current = false;
        setPending(null);
      };
      // No resume binding (session torn down) ⇒ never latched; roll back so the
      // card stays actionable. On success the latch stays (interrupt resolved).
      if (
        !resumeInterrupt(sessionId, parentRunId, itemId, response, settled, { onError: rollback })
      )
        rollback();
    },
    [parentRunId, itemId, sessionId],
  );

  return { pending, resume };
}
