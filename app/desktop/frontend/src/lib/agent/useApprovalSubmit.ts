import { useCallback, useState } from "react";
import { asItemId, asRunId } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { WIRE_DECISION, type ApprovalDecision } from "./hitlDecision";

export type { ApprovalDecision };

// Submits the user's HITL approval decision (API.md §6, R-model): it
// answers an open interrupt by starting a continuation Run via the active
// session's `resume` action (bound in useAgentSession), and optimistically
// settles the card via `resolveInterrupt`. The interrupt is addressed by
// `parentRunId` (the interrupted Run) + `itemId` (the toolCall awaiting
// approval). When either is absent the card is a decorative preview.

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

  const submit = useCallback(
    (decision: ApprovalDecision, editedArgs?: Record<string, unknown>) => {
      if (!parentRunId || !itemId || pending !== null) return;
      setPending(decision);
      const sid = useSessionStore.getState().activeSessionId;
      useAgentStore.getState().resolveInterrupt(sid, itemId, { decision });
      const resume = useAgentStore.getState().sessions[sid]?.resume;
      resume?.(asRunId(parentRunId), [
        {
          itemId: asItemId(itemId),
          response: {
            type: "approval",
            decision: WIRE_DECISION[decision],
            ...(editedArgs ? { editedArgs } : {}),
          },
        },
      ]);
    },
    [parentRunId, itemId, pending],
  );

  return { submit, pending };
}
