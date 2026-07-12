import { useCallback, useEffect, useRef } from "react";
import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import {
  registerApprovalActions,
  useApprovalSubmit,
  type ApprovalActions,
  type ApprovalDecision,
  type ApprovalSubmitOptions,
  type RememberScope,
} from "@/plugins/builtin/agent/public/hitl";
import { canSubmitApproval } from "@/plugins/builtin/agent/public/messagePresentation";

export interface ApprovalArgsCommitter {
  commit: () => Record<string, unknown> | undefined | null;
}

export interface ApprovalCardActionState {
  pending: ApprovalDecision | null;
  disabled: boolean;
  approve: () => void;
  decline: () => void;
}

export function approvalSubmitOptions({
  editedArgs,
  rememberScope,
}: {
  editedArgs?: Record<string, unknown>;
  rememberScope?: RememberScope;
}): ApprovalSubmitOptions | undefined {
  if (editedArgs === undefined && rememberScope === undefined) return undefined;
  return {
    ...(editedArgs !== undefined ? { editedArgs } : {}),
    ...(rememberScope !== undefined ? { rememberScope } : {}),
  };
}

export function canRegisterApprovalActions({
  runId,
  itemId,
  status,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
}): boolean {
  return Boolean(runId && itemId && status === "requires-action");
}

export function useApprovalCardActions({
  runId,
  itemId,
  status,
  argsEditor,
  rememberScope,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
  argsEditor?: ApprovalArgsCommitter;
  rememberScope?: RememberScope;
}): ApprovalCardActionState {
  const { submit, pending } = useApprovalSubmit(runId, itemId);

  const approve = useCallback(() => {
    const editedArgs = argsEditor?.commit();
    if (editedArgs === null) return;
    submit("approved", approvalSubmitOptions({ editedArgs, rememberScope }));
  }, [argsEditor, rememberScope, submit]);

  const decline = useCallback(() => {
    submit("declined", approvalSubmitOptions({ rememberScope }));
  }, [rememberScope, submit]);

  const actionsRef = useRef<ApprovalActions>({ approve, decline });
  actionsRef.current = { approve, decline };

  const registerable = canRegisterApprovalActions({ runId, itemId, status });
  useEffect(() => {
    if (!registerable || !itemId) return;
    return registerApprovalActions(itemId, {
      approve: () => actionsRef.current.approve(),
      decline: () => actionsRef.current.decline(),
    });
  }, [registerable, itemId]);

  return {
    pending,
    disabled: !canSubmitApproval({ runId, itemId, pending, status }),
    approve,
    decline,
  };
}
