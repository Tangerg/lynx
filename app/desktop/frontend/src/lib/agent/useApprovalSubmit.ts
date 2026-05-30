import { useCallback, useState } from "react";
import { getContainer } from "@/main/container";
import { asApprovalRequestId } from "@/rpc";

// Submits the user's HITL decision via the JSON-RPC `runs.approval.submit`
// method (the cached client from `container.methods()`). Pure UI state
// (`pending`) lives here; the transport is the shared RpcClient, so tests
// mock it the same way every other method does:
//
//   setContainer({ methods: () => fakeMethods });
//
// We deliberately do NOT clear `pending` on success — the backend's
// `lyra.approval-result` event stamps the block's `decision` field and the
// card renders against that. Clearing here would flicker the card back to
// its pre-decision state.

// UI decision vocabulary (past-tense). The protocol wire uses the
// imperative pair "approve" | "deny" (API.md §4.3); we map at the call
// boundary below so the rest of the view layer keeps its own vocabulary.
export type ApprovalDecision = "approved" | "declined";

const WIRE_DECISION = { approved: "approve", declined: "deny" } as const;

export interface ApprovalSubmit {
  /**
   * Submit the decision. `editedArgs` (approve-with-modified-args, §4.3)
   * is forwarded only when the user tweaked the tool's arguments before
   * approving — omitted otherwise so the runtime executes the original args.
   */
  submit: (decision: ApprovalDecision, editedArgs?: Record<string, unknown>) => void;
  pending: ApprovalDecision | null;
}

export function useApprovalSubmit(requestId: string | undefined): ApprovalSubmit {
  const [pending, setPending] = useState<ApprovalDecision | null>(null);

  const submit = useCallback(
    (decision: ApprovalDecision, editedArgs?: Record<string, unknown>) => {
      if (!requestId || pending !== null) return;
      setPending(decision);
      getContainer()
        .methods()
        .runs.approval.submit({
          requestId: asApprovalRequestId(requestId),
          decision: WIRE_DECISION[decision],
          ...(editedArgs ? { editedArgs } : {}),
        })
        .catch((err: unknown) => {
          console.error("[approval] submit rejected:", err);
          setPending(null);
        });
    },
    [requestId, pending],
  );

  return { submit, pending };
}
