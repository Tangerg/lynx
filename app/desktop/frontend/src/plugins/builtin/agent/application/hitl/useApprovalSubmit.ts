import { useCallback } from "react";
import type { ApprovalDecision, RememberScope } from "../../domain/hitl";
import { WIRE_DECISION } from "./wireDecision";
import { useInterruptResume } from "./useInterruptResume";

export type { RememberScope } from "../../domain/hitl";

// Submits the user's HITL approval decision (API.md §6, R-model) over the shared
// useInterruptResume scaffold (which owns session pinning, the pending latch,
// the guard, and the deferred settle). This hook only builds the approval-
// specific wire payload (editedArgs / remember) and decision patch.

export interface ApprovalSubmitOptions {
  /** Forwarded only when the user tweaked the tool's arguments before
   *  approving (approve-with-modified-args, §6.1) — omitted otherwise so the
   *  runtime executes the original args. One-shot: never part of remember. */
  editedArgs?: Record<string, unknown>;
  /** Persist this decision (approve OR deny) as a rule at the given scope
   *  (AUX_API §6) — the runtime stops asking for matching calls. Omitted = this
   *  once only. */
  rememberScope?: RememberScope;
}

export interface ApprovalSubmit {
  submit: (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => void;
  pending: ApprovalDecision | null;
}

export function useApprovalSubmit(runId?: string, itemId?: string): ApprovalSubmit {
  const { pending, resume } = useInterruptResume<ApprovalDecision>(runId, itemId);

  const submit = useCallback(
    (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => {
      resume(
        decision,
        {
          type: "approval",
          decision: WIRE_DECISION[decision],
          ...(opts?.editedArgs ? { editedArgs: opts.editedArgs } : {}),
          ...(opts?.rememberScope ? { remember: { scope: opts.rememberScope } } : {}),
        },
        { decision },
      );
    },
    [resume],
  );

  return { submit, pending };
}
