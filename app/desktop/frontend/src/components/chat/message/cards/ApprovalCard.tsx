import type { ApprovalDecision, RememberScope } from "@/lib/agent/useApprovalSubmit";
import type { BlockStatus } from "@/protocol/run/viewState";
import type { ApprovalActions } from "@/lib/agent/approvalActions";
import { useEffect, useRef, useState } from "react";
import { Checkbox, Divider, Icon, Segmented } from "@/components/common";
import { HitlCardShell, HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import { registerApprovalActions } from "@/lib/agent/approvalActions";
import { dangerHints } from "@/lib/agent/dangerPatterns";
import { useApprovalSubmit } from "@/lib/agent/useApprovalSubmit";
import { cn } from "@/lib/utils";
import { ApprovalArgsEditor, useApprovalArgsEditor } from "./ApprovalArgsEditor";

type Risk = "low" | "medium" | "high";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the action card with the
   *  Approve / Decline buttons; `"complete"` collapses to a settled
   *  checkpoint row driven by `decision`. */
  status: BlockStatus;
  what: string;
  cmd: string;
  reason: string;
  /** The interrupted Run + the toolCall Item awaiting approval — the HITL
   *  resume target (API.md §6). When either is absent the card is a
   *  decorative pre-HITL preview with no buttons. */
  parentRunId?: string;
  itemId?: string;
  /** Set once the decision is submitted (optimistic) / the run resolves. */
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
  high: "border-negative/40 bg-negative/15 text-negative",
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
// HITL flow (R-model, API.md §6):
//   1. Run ends with outcome.type="interrupt" carrying an approval Interrupt
//   2. Reducer materialises an approval block (status="requires-action")
//      bound to { parentRunId, itemId }
//   3. User clicks → useApprovalSubmit starts a continuation Run via
//      runs.resume + optimistically settles the card (resolveInterrupt)
export function ApprovalCard({
  status,
  what,
  cmd,
  reason,
  parentRunId,
  itemId,
  decision,
  args,
  risk,
  scope,
  target,
  reversible,
}: Props) {
  const t = useT();
  const { submit, pending } = useApprovalSubmit(parentRunId, itemId);

  // Editable-args state — delegated to a dedicated hook (SRP: ApprovalCard
  // renders, useApprovalArgsEditor owns the editing lifecycle + validation).
  const hasArgs = args !== undefined;
  const originalArgs = hasArgs ? JSON.stringify(args, null, 2) : "";
  const argsEditor = useApprovalArgsEditor({ originalArgs });

  // "Don't ask again" + the scope the rule is remembered at (session by default;
  // project keys it to this cwd, global everywhere). Scope is ignored unless
  // remember is on.
  const [remember, setRemember] = useState(false);
  const [rememberScope, setRememberScope] = useState<RememberScope>("session");
  const submitScope = remember ? rememberScope : undefined;

  const onApprove = () => {
    let editedArgs: Record<string, unknown> | undefined;
    if (hasArgs) {
      const result = argsEditor.commit();
      if (result === null) return; // malformed JSON — block approve
      editedArgs = result; // undefined = unchanged, object = edited
    }
    submit("approved", { editedArgs, rememberScope: submitScope });
  };
  const onDecline = () => submit("declined", { rememberScope: submitScope });

  // Bridge the ⌘↩ / ⇧⌘⌫ keyboard path (submitPendingApproval) to THIS card's
  // submit — so the shortcut applies the edited args + remember exactly like the
  // buttons. Register a stable thunk that reads the latest handlers via a ref;
  // only while the card is actionable. (The ref keeps the registration stable
  // across re-renders while still calling the current closure.)
  const actionsRef = useRef<ApprovalActions>({ approve: onApprove, decline: onDecline });
  actionsRef.current = { approve: onApprove, decline: onDecline };
  const resumable = Boolean(parentRunId && itemId && status === "requires-action");
  useEffect(() => {
    if (!resumable || !itemId) return;
    return registerApprovalActions(itemId, {
      approve: () => actionsRef.current.approve(),
      decline: () => actionsRef.current.decline(),
    });
  }, [resumable, itemId]);

  const finalised = status === "complete" ? decision : pending;
  if (finalised === "approved") {
    return <HitlSettledRow label={t("approval.settled.approved")} />;
  }
  if (finalised === "declined") {
    return <Divider icon={<Icon name="x" size={11} />}>{t("approval.settled.declined")}</Divider>;
  }

  // Pre-decision card. Buttons disabled when not resumable (decorative preview),
  // while a request is in flight, OR once the interrupt is no longer open:
  // settleOpenInterrupts downgrades an unacted interrupt to `incomplete` on
  // run-end precisely so its buttons can't resume a dead run.
  const disabled = !parentRunId || !itemId || pending !== null || status !== "requires-action";
  const effectiveRisk: Risk = risk ?? "medium";
  // Client-side destructive-command heuristic (§T2.5) — flags rm -rf / sudo /
  // curl|sh / dd / mkfs / chmod 777 / fork bomb / force-push regardless of the
  // backend's risk field, so a dangerous command always carries a visible "are
  // you sure?" cue.
  const dangers = cmd.trim() ? dangerHints(cmd) : [];
  return (
    <HitlCardShell
      data-slot="approval-card"
      variant="warning"
      icon="shield"
      iconClassName="text-warning"
      label={t("approval.required")}
      trailing={
        <span
          className={cn(
            "rounded-sm border px-1.5 py-px text-[10px] font-medium",
            RISK_BADGE_CLASS[effectiveRisk],
          )}
        >
          {t(RISK_I18N_KEY[effectiveRisk])}
        </span>
      }
    >
      <div className="mb-1.5 text-[16px] font-semibold leading-[1.4] text-fg">{what}</div>
      {/* Shell-prompt command line — only for command-style approvals. Other
          tools have no `cmd` (their payload is just args), so skip the box
          instead of rendering a lonely "$". */}
      {cmd.trim() && (
        <code className="my-1.5 block whitespace-pre-wrap break-all rounded-sm bg-warning/10 px-2.5 py-1.5 font-mono text-[13px] text-fg">
          $ {cmd}
        </code>
      )}
      {dangers.length > 0 && (
        <div className="my-1.5 flex items-start gap-2 rounded-sm border border-negative/50 bg-negative/12 px-2.5 py-1.5 text-[12px] leading-[1.5] text-negative">
          <Icon name="alert" size={13} className="mt-px shrink-0" />
          <span>
            <span className="font-semibold">{t("approval.danger")}</span> {dangers.join(" · ")}
          </span>
        </div>
      )}
      {hasArgs && (
        <ApprovalArgsEditor
          editing={argsEditor.editing}
          argsText={argsEditor.argsText}
          invalid={argsEditor.invalid}
          onEditToggle={argsEditor.setEditing}
          onTextChange={(text) => {
            argsEditor.setArgsText(text);
          }}
        />
      )}
      {(scope?.length || target || reversible !== undefined) && (
        <div className="mb-2 flex flex-wrap items-center gap-1.5">
          {scope?.map((s) => (
            <span
              key={s}
              className={cn(
                "inline-flex items-center rounded-xs border px-1.5 py-px font-mono text-[10.5px] font-semibold",
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
                "inline-flex items-center gap-1 rounded-xs border px-1.5 py-px font-mono text-[10.5px] font-semibold",
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
      <div className="mb-2 text-[13px] leading-[1.55] text-fg-muted">{reason}</div>
      <div className="flex items-center gap-2">
        <button
          type="button"
          data-slot="approval-approve"
          disabled={disabled}
          onClick={onApprove}
          className="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-fg px-3 py-1.5 text-[13px] font-medium text-on-fg transition-opacity duration-150 ease-out hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {t("approval.action.approve")}
          {!disabled && <kbd className="ml-1.5 font-mono text-[10px] opacity-60">⌘↵</kbd>}
        </button>
        <button
          type="button"
          data-slot="approval-decline"
          disabled={disabled}
          onClick={onDecline}
          className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-line bg-transparent px-3 py-1.5 text-[13px] font-medium text-fg transition-colors duration-150 ease-out hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {t("approval.action.decline")}
          {!disabled && <kbd className="ml-1.5 font-mono text-[10px] opacity-60">⇧⌘⌫</kbd>}
        </button>
        <label className="ml-auto flex cursor-pointer items-center gap-1.5 text-[11.5px] text-fg-muted select-none">
          <Checkbox
            checked={remember}
            onCheckedChange={setRemember}
            ariaLabel={t("approval.remember")}
          />
          {t("approval.remember")}
        </label>
      </div>
      {/* Scope picker — only meaningful once "don't ask again" is on. Session
          keys the rule to this session, project to this folder, global everywhere. */}
      {remember && (
        <div className="mt-2 flex justify-end">
          <Segmented
            value={rememberScope}
            options={[
              { value: "session", label: t("approvals.scope.session") },
              { value: "project", label: t("approvals.scope.project") },
              { value: "global", label: t("approvals.scope.global") },
            ]}
            onChange={setRememberScope}
            ariaLabel={t("approval.remember.scope")}
          />
        </div>
      )}
    </HitlCardShell>
  );
}
