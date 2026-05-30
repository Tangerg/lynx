import type { ApprovalDecision } from "@/lib/agent/useApprovalSubmit";
import type { BlockStatus } from "@/protocol/agui/viewState";
import { useState } from "react";
import { Divider, Icon, PillButton } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useApprovalSubmit } from "@/lib/agent/useApprovalSubmit";
import { cn } from "@/lib/utils";

type Risk = "low" | "medium" | "high";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the action card with the
   *  Approve / Decline buttons; `"complete"` collapses to a settled
   *  checkpoint row driven by `decision`. */
  status: BlockStatus;
  what: string;
  cmd: string;
  reason: string;
  /** Backend-generated id used to POST the decision back. When absent the
   *  card renders as a decorative pre-HITL preview with no buttons. */
  requestId?: string;
  /** Set by the agui-handlers reducer once the backend has confirmed
   *  receipt of the decision via the lyra.approval-result event. */
  decision?: ApprovalDecision;
  /** Tool arguments about to be executed. When present, the card lets the
   *  user edit them before approving (approve-with-modified-args, §4.3). */
  args?: Record<string, unknown>;
  /** Risk level — drives the badge colour + dot. Defaults to "medium"
   *  when omitted (older backends): "we don't know, be cautious". */
  risk?: Risk;
  /** Free-form action categories (read / write / network / shell /
   *  delete / …) — rendered as chips so the user can see at a glance
   *  what kinds of side effects an approval would unlock. */
  scope?: string[];
  /** Path / URL / resource the action targets. Mono-rendered. */
  target?: string;
  /** Whether the action can be undone. Drives a reversible / permanent
   *  hint; undefined = unknown, no hint. */
  reversible?: boolean;
}

const RISK_BADGE_CLASS: Record<Risk, string> = {
  low: "border-fg-faint/30 bg-fg-faint/10 text-fg-muted",
  medium: "border-warning/40 bg-warning/15 text-warning",
  high: "border-negative/50 bg-negative/15 text-negative",
};

const RISK_I18N_KEY: Record<Risk, string> = {
  low: "approval.risk.low",
  medium: "approval.risk.medium",
  high: "approval.risk.high",
};

// Known scopes get a coloured chip so "delete" reads differently from
// "read" at a glance. Unknown scopes fall back to the neutral chip.
const SCOPE_CHIP_CLASS: Record<string, string> = {
  read: "border-line bg-surface-2 text-fg-muted",
  write: "border-warning/30 bg-warning/10 text-warning",
  network: "border-line bg-surface-2 text-fg-muted",
  shell: "border-warning/30 bg-warning/10 text-warning",
  delete: "border-negative/40 bg-negative/12 text-negative",
};
const SCOPE_CHIP_DEFAULT = "border-line bg-surface-2 text-fg-muted";

// Approval card — pure presentation. HTTP / submitting state lives in
// useApprovalSubmit; this component renders against `status`:
//   - "complete"         → settled checkpoint row (decision is authoritative)
//   - "requires-action"  → action card with Approve / Decline buttons,
//                           or optimistic checkpoint while a submit is in
//                           flight (pending mirrors the user's last click)
//
// HITL flow:
//   1. Backend Approval(...) step → emit lyra.approval { requestId }
//   2. Reducer materialises an approval content block with status="requires-action"
//   3. User clicks → useApprovalSubmit POSTs /permission
//   4. Backend resolves the chan, script emits lyra.approval-result
//   5. Reducer stamps `decision` + flips status to "complete" → card swaps
export function ApprovalCard({
  status,
  what,
  cmd,
  reason,
  requestId,
  decision,
  args,
  risk,
  scope,
  target,
  reversible,
}: Props) {
  const t = useT();
  const { submit, pending } = useApprovalSubmit(requestId);

  // Editable-args state. The textarea seeds from the original args; on
  // approve we re-parse it and forward `editedArgs` only when it changed.
  const hasArgs = args !== undefined;
  const originalArgs = hasArgs ? JSON.stringify(args, null, 2) : "";
  const [editing, setEditing] = useState(false);
  const [argsText, setArgsText] = useState(originalArgs);
  const [argsInvalid, setArgsInvalid] = useState(false);

  const onApprove = () => {
    if (!hasArgs) {
      submit("approved");
      return;
    }
    let editedArgs: Record<string, unknown> | undefined;
    try {
      const parsed = JSON.parse(argsText) as Record<string, unknown>;
      // Only forward when the user actually changed something — a no-op
      // edit should execute the original args, not a re-serialised copy.
      if (JSON.stringify(parsed) !== JSON.stringify(args)) editedArgs = parsed;
    } catch {
      setArgsInvalid(true);
      return; // block approve on malformed JSON
    }
    submit("approved", editedArgs);
  };

  const finalised = status === "complete" ? decision : pending;
  if (finalised === "approved") {
    return (
      <Divider icon={<Icon name="check" size={11} strokeWidth={3} />} intent="accent">
        {t("approval.settled.approved")}
      </Divider>
    );
  }
  if (finalised === "declined") {
    return <Divider icon={<Icon name="x" size={11} />}>{t("approval.settled.declined")}</Divider>;
  }

  // Pre-decision card. Buttons disabled when no requestId (decorative
  // preview) or while a request is in flight.
  const disabled = !requestId || pending !== null;
  const effectiveRisk: Risk = risk ?? "medium";
  return (
    <div className="my-3 rounded-xl border border-warning/25 bg-[linear-gradient(180deg,rgba(255,164,43,0.06)_0%,var(--color-surface)_60%)] px-4 py-3.5">
      <div className="mb-2 flex items-center gap-2 font-mono text-[11px] font-semibold text-warning">
        <Icon name="shield" size={12} />
        <span>{t("approval.required")}</span>
        <span className="flex-1" />
        <span
          className={cn(
            "rounded-sm border px-1.5 py-px font-mono text-[10px] font-semibold uppercase tracking-wider",
            RISK_BADGE_CLASS[effectiveRisk],
          )}
        >
          {t(RISK_I18N_KEY[effectiveRisk])}
        </span>
      </div>
      <div className="mb-1.5 text-[15px] font-semibold leading-[1.4] text-fg">{what}</div>
      <code className="my-2 block whitespace-pre-wrap break-all rounded-sm bg-warning/14 px-3 py-2 font-mono text-[13px] text-fg">
        $ {cmd}
      </code>
      {hasArgs && (
        <div className="mb-2">
          <div className="mb-1 flex items-center gap-2">
            <span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-fg-faint">
              {t("approval.args.label")}
            </span>
            {!editing && (
              <button
                type="button"
                onClick={() => setEditing(true)}
                className="font-mono text-[10.5px] font-semibold text-accent hover:underline"
              >
                {t("approval.args.edit")}
              </button>
            )}
          </div>
          {editing ? (
            <>
              <textarea
                value={argsText}
                aria-label={t("approval.args.label")}
                spellCheck={false}
                rows={Math.min(10, argsText.split("\n").length + 1)}
                onChange={(e) => {
                  setArgsText(e.target.value);
                  setArgsInvalid(false);
                }}
                className={cn(
                  "w-full resize-y rounded-sm bg-surface-2 px-3 py-2 font-mono text-[12px] text-fg focus:outline-none",
                  argsInvalid ? "border border-negative/60" : "border border-line",
                )}
              />
              {argsInvalid && (
                <div className="mt-1 font-mono text-[10.5px] text-negative">
                  {t("approval.args.invalid")}
                </div>
              )}
            </>
          ) : (
            <pre className="m-0 max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-sm bg-surface-2 px-3 py-2 font-mono text-[12px] text-fg-muted">
              {argsText}
            </pre>
          )}
        </div>
      )}
      {(scope?.length || target || reversible !== undefined) && (
        <div className="mb-2 flex flex-wrap items-center gap-1.5">
          {scope?.map((s) => (
            <span
              key={s}
              className={cn(
                "inline-flex items-center rounded-xs border px-1.5 py-px font-mono text-[10.5px] font-semibold uppercase tracking-wider",
                SCOPE_CHIP_CLASS[s] ?? SCOPE_CHIP_DEFAULT,
              )}
            >
              {s}
            </span>
          ))}
          {target && (
            <span className="inline-flex items-center gap-1 rounded-xs border border-line bg-surface-2 px-1.5 py-px font-mono text-[11px] text-fg-muted">
              <Icon name="folder" size={10} className="text-fg-faint" />
              {target}
            </span>
          )}
          {reversible !== undefined && (
            <span
              className={cn(
                "inline-flex items-center gap-1 rounded-xs border px-1.5 py-px font-mono text-[10.5px] font-semibold uppercase tracking-wider",
                reversible
                  ? "border-fg-faint/30 bg-fg-faint/10 text-fg-muted"
                  : "border-negative/40 bg-negative/12 text-negative",
              )}
            >
              {t(reversible ? "approval.reversible" : "approval.permanent")}
            </span>
          )}
        </div>
      )}
      <div className="mb-3 text-[13px] leading-[1.55] text-fg-muted">{reason}</div>
      <div className="flex items-center gap-2">
        <PillButton variant="accent" size="sm" disabled={disabled} onClick={onApprove}>
          {t("approval.action.approve")}
        </PillButton>
        <PillButton size="sm" disabled={disabled} onClick={() => submit("declined")}>
          {t("approval.action.decline")}
        </PillButton>
      </div>
    </div>
  );
}
