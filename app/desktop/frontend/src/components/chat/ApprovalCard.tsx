import { useState } from "react";
import { Icon, PillButton } from "@/components/common";
import { AGUI_BASE } from "@/lib/http";

type Props = {
  what: string;
  cmd: string;
  reason: string;
  /** Backend-generated id used to POST the decision back. When absent the
   *  card renders as a decorative pre-HITL preview with no buttons. */
  requestId?: string;
  /** Set by the agui-handlers reducer once the backend has confirmed
   *  receipt of the decision via the lyra.approval-result event. The
   *  card swaps to its post-decision look. */
  decision?: "approved" | "declined";
};

// Approval card — drives the human-in-the-loop gate for tool calls.
//
// Flow:
//   1. Backend script hits an Approval(...) step → blocks
//   2. Backend emits lyra.approval CUSTOM event with a requestId
//   3. Reducer materialises an approval content block
//   4. THIS card renders Approve / Decline buttons
//   5. User clicks → POST /permission { requestId, decision }
//   6. Backend resolves the chan + emits lyra.approval-result
//   7. Reducer stamps `decision` on the block → card switches state
//
// `decision` is the source of truth post-click — when it's set, the
// card shows the committed view and ignores its own local "submitting"
// flag. That way switching sessions and coming back keeps the card
// in the same state the backend confirmed.
export function ApprovalCard({ what, cmd, reason, requestId, decision }: Props) {
  const [submitting, setSubmitting] = useState<null | "approved" | "declined">(null);

  const send = async (next: "approved" | "declined") => {
    if (!requestId || submitting || decision) return;
    setSubmitting(next);
    try {
      const r = await fetch(`${AGUI_BASE}/permission`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ requestId, decision: next }),
      });
      if (!r.ok) {
        // eslint-disable-next-line no-console
        console.error("[approval] /permission rejected:", r.status, await r.text().catch(() => ""));
        setSubmitting(null);
      }
      // On success we leave `submitting` set — the backend's
      // lyra.approval-result event flips `decision` on the block,
      // which is what we render against next.
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[approval] network error:", err);
      setSubmitting(null);
    }
  };

  // Post-decision states — show a compact confirmation row instead of
  // the full card. Matches the existing checkpoint style.
  const finalised = decision ?? submitting;
  if (finalised === "approved") {
    return (
      <div className="checkpoint">
        <div className="ico"><Icon name="check" size={11} strokeWidth={3} /></div>
        <span>已批准 · 正在执行</span>
      </div>
    );
  }
  if (finalised === "declined") {
    return (
      <div className="checkpoint">
        <div className="ico" style={{ color: "var(--color-text-faint)" }}>
          <Icon name="x" size={11} />
        </div>
        <span style={{ color: "var(--color-text-faint)" }}>已拒绝</span>
      </div>
    );
  }

  // Pre-decision card. The action buttons are disabled when there's no
  // requestId (decorative preview) or while a request is in flight.
  const disabled = !requestId || submitting !== null;
  return (
    <div className="approval-card">
      <div className="head">
        <Icon name="shield" size={12} />Approval required
      </div>
      <div className="what">{what}</div>
      <code className="cmd">$ {cmd}</code>
      <div className="reason">{reason}</div>
      <div className="actions">
        <PillButton
          variant="accent"
          style={{ height: 30, fontSize: 11 }}
          disabled={disabled}
          onClick={() => send("approved")}
        >
          Approve
        </PillButton>
        <PillButton
          style={{ height: 30, fontSize: 11 }}
          disabled={disabled}
          onClick={() => send("declined")}
        >
          Decline
        </PillButton>
      </div>
    </div>
  );
}
