import { useCallback, useState } from "react";
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

  const resume = useCallback(
    (marker: P, response: InterruptResponse["response"], settled: ResolvePatch) => {
      if (!parentRunId || !itemId || pending !== null) return;
      const sessionResume = useAgentStore.getState().sessions[sessionId]?.resume;
      if (!sessionResume) return;
      setPending(marker);
      sessionResume(
        asRunId(parentRunId),
        [{ itemId: asItemId(itemId), response }],
        () => useAgentStore.getState().resolveInterrupt(sessionId, itemId, settled),
        () => setPending(null),
      );
    },
    [parentRunId, itemId, pending, sessionId],
  );

  return { pending, resume };
}
