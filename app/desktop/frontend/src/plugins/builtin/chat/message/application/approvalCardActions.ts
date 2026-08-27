import { useCallback, useEffect } from "react";
import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import {
  useApprovalSubmit,
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
  approve: (rememberScope?: RememberScope) => void;
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
  runtimeAvailable = true,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
  runtimeAvailable?: boolean;
}): boolean {
  return Boolean(runtimeAvailable && runId && itemId && status === "requires-action");
}

export function useApprovalCardActions({
  runId,
  itemId,
  status,
  argsEditor,
  runtimeAvailable,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
  argsEditor?: ApprovalArgsCommitter;
  runtimeAvailable: boolean;
}): ApprovalCardActionState {
  const { submit, pending, registerActions } = useApprovalSubmit(runId, itemId);

  const approve = useCallback(
    (rememberScope?: RememberScope) => {
      const editedArgs = argsEditor?.commit();
      if (editedArgs === null) return;
      submit("approved", approvalSubmitOptions({ editedArgs, rememberScope }));
    },
    [argsEditor, submit],
  );

  const decline = useCallback(() => {
    submit("declined");
  }, [submit]);

  const registerable = canRegisterApprovalActions({ runId, itemId, status, runtimeAvailable });
  useEffect(() => {
    if (!registerable) return;
    return registerActions({
      approve: () => approve(),
      decline,
    });
  }, [approve, decline, registerable, registerActions]);

  return {
    pending,
    disabled: !runtimeAvailable || !canSubmitApproval({ runId, itemId, pending, status }),
    approve,
    decline,
  };
}
