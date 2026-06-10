import { useCallback, useState } from "react";
import { asItemId, asRunId } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { WIRE_DECISION, type ApprovalDecision } from "./hitlDecision";

export type { ApprovalDecision };

// Submits the user's HITL approval decision (API.md §6, R-model): it answers
// an open interrupt by starting a continuation Run via the owning session's
// `resume` action (bound in useAgentSession). The card shows its settled state
// immediately from local `pending`; the store-level settle (`resolveInterrupt`)
// only commits once the continuation run actually starts, so a rejected resume
// leaves the interrupt intact and retryable. Addressed by `parentRunId` (the
// interrupted Run) + `itemId` (the toolCall awaiting approval). When either is
// absent the card is a decorative preview.

export interface ApprovalSubmit {
  /**
   * Submit the decision. `editedArgs` (approve-with-modified-args, §6.1) is
   * forwarded only when the user tweaked the tool's arguments before
   * approving — omitted otherwise so the runtime executes the original args.
   */
  submit: (decision: ApprovalDecision, editedArgs?: Record<string, unknown>) => void;
  pending: ApprovalDecision | null;
}

export function useApprovalSubmit(parentRunId?: string, itemId?: string): ApprovalSubmit {
  const [pending, setPending] = useState<ApprovalDecision | null>(null);
  // Pin the owning session at mount. The card renders from the active session's
  // slice, so activeSessionId == owner here; pinning it means a fast tab switch
  // between render and click can't redirect the resume/resolve onto the wrong
  // session (reading activeSessionId at click time could).
  const [sessionId] = useState(() => useSessionStore.getState().activeSessionId);

  const submit = useCallback(
    (decision: ApprovalDecision, editedArgs?: Record<string, unknown>) => {
      if (!parentRunId || !itemId || pending !== null) return;
      const resume = useAgentStore.getState().sessions[sessionId]?.resume;
      if (!resume) return;
      // `pending` drives the card's optimistic settled state on its own. The
      // store mutation (resolveInterrupt: stamp block + drop open interrupt) is
      // deferred until the continuation run actually starts, so a channel-a
      // failure leaves the interrupt intact and the card retryable.
      setPending(decision);
      resume(
        asRunId(parentRunId),
        [
          {
            itemId: asItemId(itemId),
            response: {
              type: "approval",
              decision: WIRE_DECISION[decision],
              ...(editedArgs ? { editedArgs } : {}),
            },
          },
        ],
        () => useAgentStore.getState().resolveInterrupt(sessionId, itemId, { decision }),
        () => setPending(null),
      );
    },
    [parentRunId, itemId, pending, sessionId],
  );

  return { submit, pending };
}
