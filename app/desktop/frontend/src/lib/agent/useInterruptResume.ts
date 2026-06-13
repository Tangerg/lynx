import { useCallback, useRef, useState } from "react";
import { asItemId, asRunId, type InterruptResponse } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

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
//    the run-started callback, so a channel-a failure (rejected runs.resume,
//    §8.1) leaves the interrupt intact and the card retryable.
// Each caller supplies, per submit, the pending marker (so the card knows which
// action is settling), the wire response payload, and the resolveInterrupt patch.

type ResolvePatch = { decision?: "approved" | "declined"; answered?: boolean };

export function useInterruptResume<P>(parentRunId?: string, itemId?: string) {
  const [pending, setPending] = useState<P | null>(null);
  const [sessionId] = useState(() => useSessionStore.getState().activeSessionId);
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
      const sessionResume = useAgentStore.getState().sessions[sessionId]?.resume;
      if (!sessionResume) return;
      submitted.current = true;
      setPending(marker);
      sessionResume(
        asRunId(parentRunId),
        [{ itemId: asItemId(itemId), response }],
        () => useAgentStore.getState().resolveInterrupt(sessionId, itemId, settled),
        () => {
          submitted.current = false;
          setPending(null);
        },
      );
    },
    [parentRunId, itemId, sessionId],
  );

  return { pending, resume };
}
