import type { ApprovalDecision } from "@/domain";
import { useCallback, useState } from "react";
import { getContainer } from "@/main/container";

// Submits the user's approval / decline decision via the permission
// gateway (see `@/domain/gateways/PermissionGateway`). Pure UI state
// (`pending`) lives here; the HTTP plumbing lives in the gateway impl
// so this hook is transport-agnostic and trivially mockable in tests:
//
//   setContainer({ permission: fakeGateway });
//
// We deliberately do NOT clear `pending` on success — the backend's
// `lyra.approval-result` event will stamp the block's `decision` field,
// and the card renders against that. Clearing here would briefly flicker
// the card back to the pre-decision state.

export type { ApprovalDecision };

export interface ApprovalSubmit {
  submit: (decision: ApprovalDecision) => void;
  pending: ApprovalDecision | null;
}

export function useApprovalSubmit(requestId: string | undefined): ApprovalSubmit {
  const [pending, setPending] = useState<ApprovalDecision | null>(null);

  const submit = useCallback(
    (decision: ApprovalDecision) => {
      if (!requestId || pending !== null) return;
      setPending(decision);
      getContainer()
        .permission.submit({ requestId, decision })
        .catch((err) => {
          console.error("[approval] gateway rejected:", err);
          setPending(null);
        });
    },
    [requestId, pending],
  );

  return { submit, pending };
}
