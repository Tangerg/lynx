import { Icon, PillButton } from "@/components/common";
import { useApprovalSubmit, type ApprovalDecision } from "@/lib/useApprovalSubmit";

type Props = {
  what: string;
  cmd: string;
  reason: string;
  /** Backend-generated id used to POST the decision back. When absent the
   *  card renders as a decorative pre-HITL preview with no buttons. */
  requestId?: string;
  /** Set by the agui-handlers reducer once the backend has confirmed
   *  receipt of the decision via the lyra.approval-result event. */
  decision?: ApprovalDecision;
};

// Approval card — pure presentation. HTTP / submitting state lives in
// useApprovalSubmit; this component only renders three visual states:
//   - settled       → checkpoint row (decision is the authoritative source)
//   - optimistic    → checkpoint row (pending is the user's last click)
//   - pre-decision  → action card with Approve / Decline buttons
//
// HITL flow:
//   1. Backend Approval(...) step → emit lyra.approval { requestId }
//   2. Reducer materialises an approval content block (this card)
//   3. User clicks → useApprovalSubmit POSTs /permission
//   4. Backend resolves the chan, script emits lyra.approval-result
//   5. Reducer stamps `decision` on the block → card swaps state
export function ApprovalCard({ what, cmd, reason, requestId, decision }: Props) {
  const { submit, pending } = useApprovalSubmit(requestId);

  const finalised = decision ?? pending;
  if (finalised === "approved") {
    return (
      <div className="my-2 flex items-center gap-3 font-mono text-[10.5px] font-semibold text-fg-faint
        before:flex-1 before:h-px before:content-[''] before:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]
        after:flex-1  after:h-px  after:content-[''] after:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]">
        <div className="grid h-[18px] w-[18px] place-items-center rounded-full bg-surface-2 text-accent">
          <Icon name="check" size={11} strokeWidth={3} />
        </div>
        <span>已批准 · 正在执行</span>
      </div>
    );
  }
  if (finalised === "declined") {
    return (
      <div className="my-2 flex items-center gap-3 font-mono text-[10.5px] font-semibold text-fg-faint
        before:flex-1 before:h-px before:content-[''] before:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]
        after:flex-1  after:h-px  after:content-[''] after:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]">
        <div className="grid h-[18px] w-[18px] place-items-center rounded-full bg-surface-2 text-fg-faint">
          <Icon name="x" size={11} />
        </div>
        <span>已拒绝</span>
      </div>
    );
  }

  // Pre-decision card. Buttons disabled when no requestId (decorative
  // preview) or while a request is in flight.
  const disabled = !requestId || pending !== null;
  return (
    <div className="my-3 rounded-xl border border-warning/25 bg-[linear-gradient(180deg,rgba(255,164,43,0.06)_0%,var(--color-surface)_60%)] px-4 py-3.5">
      <div className="mb-2 flex items-center gap-2 font-mono text-[10.5px] font-semibold text-warning">
        <Icon name="shield" size={12} />Approval required
      </div>
      <div className="mb-1.5 text-[14px] font-semibold leading-[1.4] text-fg">{what}</div>
      <code className="my-2 block whitespace-pre-wrap break-all rounded-sm bg-warning/14 px-3 py-2 font-mono text-[12.5px] text-fg">
        $ {cmd}
      </code>
      <div className="mb-3 text-[12px] leading-[1.5] text-fg-muted">{reason}</div>
      <div className="flex items-center gap-2">
        <PillButton
          variant="accent"
          size="sm"
          disabled={disabled}
          onClick={() => submit("approved")}
        >
          Approve
        </PillButton>
        <PillButton
          size="sm"
          disabled={disabled}
          onClick={() => submit("declined")}
        >
          Decline
        </PillButton>
      </div>
    </div>
  );
}
