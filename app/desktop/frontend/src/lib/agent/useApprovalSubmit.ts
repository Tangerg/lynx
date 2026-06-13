import { useCallback } from "react";
import { WIRE_DECISION, type ApprovalDecision } from "./hitlDecision";
import { useInterruptResume } from "./useInterruptResume";

export type { ApprovalDecision };

// Submits the user's HITL approval decision (API.md §6, R-model) over the shared
// useInterruptResume scaffold (which owns session pinning, the pending latch,
// the guard, and the deferred settle). This hook only builds the approval-
// specific wire payload (editedArgs / remember) and decision patch.

export interface ApprovalSubmitOptions {
  /** Forwarded only when the user tweaked the tool's arguments before
   *  approving (approve-with-modified-args, §6.1) — omitted otherwise so the
   *  runtime executes the original args. One-shot: never part of remember. */
  editedArgs?: Record<string, unknown>;
  /** Remember this decision (approve OR deny) for the rest of the session,
   *  keyed by tool name (AUX_API §6) — the runtime stops asking for it. */
  rememberForSession?: boolean;
}

export interface ApprovalSubmit {
  submit: (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => void;
  pending: ApprovalDecision | null;
}

export function useApprovalSubmit(parentRunId?: string, itemId?: string): ApprovalSubmit {
  const { pending, resume } = useInterruptResume<ApprovalDecision>(parentRunId, itemId);

  const submit = useCallback(
    (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => {
      resume(
        decision,
        {
          type: "approval",
          decision: WIRE_DECISION[decision],
          ...(opts?.editedArgs ? { editedArgs: opts.editedArgs } : {}),
          ...(opts?.rememberForSession ? { remember: { scope: "session" as const } } : {}),
        },
        { decision },
      );
    },
    [resume],
  );

  return { submit, pending };
}
