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

export interface ApprovalActions {
  approve: () => void;
  decline: () => void;
}

interface ApprovalActionOwner {
  sessionId: string;
  rootRunId: string;
  itemId: string;
}

interface ApprovalActionEntry {
  owner: ApprovalActionOwner;
  actions: ApprovalActions;
}

class ApprovalActionRegistry {
  readonly #entries = new Map<string, ApprovalActionEntry>();

  register(owner: ApprovalActionOwner, actions: ApprovalActions): () => void {
    const key = this.#key(owner);
    const entry = { owner, actions };
    this.#entries.set(key, entry);
    return () => {
      if (this.#entries.get(key) === entry) this.#entries.delete(key);
    };
  }

  find(owner: ApprovalActionOwner): ApprovalActions | undefined {
    return this.#entries.get(this.#key(owner))?.actions;
  }

  #key(owner: ApprovalActionOwner): string {
    return JSON.stringify([owner.sessionId, owner.rootRunId, owner.itemId]);
  }
}

const approvalActionRegistry = new ApprovalActionRegistry();

/** Internal keyboard bridge registration. Product cards bind through the
 * identity-capturing registrar returned by useApprovalSubmit. */
export function registerApprovalActions(
  sessionId: string,
  rootRunId: string,
  itemId: string,
  actions: ApprovalActions,
): () => void {
  return approvalActionRegistry.register({ sessionId, rootRunId, itemId }, actions);
}

export function getApprovalActions(
  sessionId: string,
  rootRunId: string,
  itemId: string,
): ApprovalActions | undefined {
  return approvalActionRegistry.find({ sessionId, rootRunId, itemId });
}

export interface ApprovalSubmit {
  submit: (decision: ApprovalDecision, opts?: ApprovalSubmitOptions) => void;
  pending: ApprovalDecision | null;
  registerActions: (actions: ApprovalActions) => () => void;
}

export function useApprovalSubmit(rootRunId?: string, itemId?: string): ApprovalSubmit {
  const { pending, resume, sessionId } = useInterruptResume<ApprovalDecision>(rootRunId, itemId);

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

  const registerActions = useCallback(
    (actions: ApprovalActions) => {
      if (!rootRunId || !itemId) return () => undefined;
      return registerApprovalActions(sessionId, rootRunId, itemId, actions);
    },
    [itemId, rootRunId, sessionId],
  );

  return { submit, pending, registerActions };
}
