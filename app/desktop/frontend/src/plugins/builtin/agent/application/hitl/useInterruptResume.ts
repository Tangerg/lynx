import { useCallback, useRef, useState } from "react";
import type { InterruptResumePayload, ResolvePatch } from "../ports/sessionView";
import { agentSessionState } from "../ports/sessionState";
import { stageInterruptResponse } from "./interruptResponseCoordinator";

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
//  - staging the card response into its root Run's atomic interrupt set; the
//    store-level settles are DEFERRED until the complete set opens a
//    continuation — see resumeInterrupt below.
// Each caller supplies, per submit, the pending marker (so the card knows which
// action is settling), the wire response payload, and the resolveInterrupt patch.

/**
 * Stage one HITL answer and DEFER every optimistic settle until the owning
 * root's complete interrupt set has opened one continuation. A channel-a
 * failure (rejected resume, §8.1) leaves the whole set intact and retryable.
 * Returns false when this card is no longer part of an answerable open set or
 * the session has no resume binding.
 *
 * The single source of this deferred-settle semantic — shared by the per-card
 * `useInterruptResume` hook (optimistic card state) and the keyboard-path
 * `submitPendingApproval` (no card). `hooks.onSettled` runs after the deferred
 * settle; `hooks.onError` on a channel-a failure — each caller uses them to
 * clear its own in-flight latch.
 */
export function resumeInterrupt(
  sessionId: string,
  runId: string,
  itemId: string,
  response: InterruptResumePayload,
  settled: ResolvePatch,
  hooks?: { onSettled?: () => void; onError?: () => void },
): boolean {
  return stageInterruptResponse(sessionId, runId, itemId, response, settled, hooks);
}

export function useInterruptResume<P>(runId?: string, itemId?: string) {
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
    (marker: P, response: InterruptResumePayload, settled: ResolvePatch) => {
      if (!runId || !itemId || submitted.current) return;
      submitted.current = true;
      setPending(marker);
      const rollback = () => {
        submitted.current = false;
        setPending(null);
      };
      // No resume binding (session torn down) ⇒ never latched; roll back so the
      // card stays actionable. On success the latch stays (interrupt resolved).
      if (!resumeInterrupt(sessionId, runId, itemId, response, settled, { onError: rollback }))
        rollback();
    },
    [runId, itemId, sessionId],
  );

  return { pending, resume, sessionId };
}
