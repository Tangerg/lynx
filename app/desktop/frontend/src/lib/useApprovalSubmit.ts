import { useCallback, useState } from "react";
import { AGUI_BASE } from "@/lib/http";

// POST the user's approval / decline back to the backend permission
// endpoint. Extracted from ApprovalCard so the card stays pure
// presentation and the HTTP plumbing can be tested / mocked
// independently.
//
// Returns:
//   - `submit(decision)`: fires the request. No-op if the card has no
//     requestId (decorative card) or a submission is already in flight.
//   - `pending`: which decision the user just clicked, while the POST
//     resolves. Drives the optimistic post-decision UI; the authoritative
//     value lands later via the lyra.approval-result reducer.
//
// We deliberately do NOT clear `pending` on success — the backend's
// approval-result event will stamp the block's `decision` field, and
// the card renders against that. Clearing here would briefly flicker
// back to the pre-decision card.

export type ApprovalDecision = "approved" | "declined";

export type ApprovalSubmit = {
  submit: (decision: ApprovalDecision) => void;
  pending: ApprovalDecision | null;
};

export function useApprovalSubmit(requestId: string | undefined): ApprovalSubmit {
  const [pending, setPending] = useState<ApprovalDecision | null>(null);

  const submit = useCallback(
    (decision: ApprovalDecision) => {
      if (!requestId || pending !== null) return;
      setPending(decision);
      fetch(`${AGUI_BASE}/permission`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ requestId, decision }),
      })
        .then(async (r) => {
          if (!r.ok) {
            // eslint-disable-next-line no-console
            console.error(
              "[approval] /permission rejected:",
              r.status,
              await r.text().catch(() => ""),
            );
            setPending(null);
          }
        })
        .catch((err) => {
          // eslint-disable-next-line no-console
          console.error("[approval] network error:", err);
          setPending(null);
        });
    },
    [requestId, pending],
  );

  return { submit, pending };
}
