import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import { useState } from "react";
import { Button, Checkbox, Divider, Icon, Segmented } from "@/ui";
import { HitlCardShell, HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import { type ApprovalDecision, type RememberScope } from "@/plugins/builtin/agent/public/hitl";
import {
  approvalReversibilityView,
  approvalRiskView,
  approvalScopeViews,
  approvalSettledDecision,
  dangerHints,
  type ApprovalRisk,
  type ApprovalTone,
} from "@/plugins/builtin/agent/public/messagePresentation";
import { cn } from "@/lib/utils";
import { useApprovalArgsEditor } from "../../application/approvalArgsEditor";
import { useApprovalCardActions } from "../../application/approvalCardActions";
import { ApprovalArgsEditor } from "./ApprovalArgsEditor";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the action card with the
   *  Approve / Decline buttons; `"complete"` collapses to a settled
   *  checkpoint row driven by `decision`. */
  status: BlockStatus;
  what: string;
  cmd: string;
  reason: string;
  /** The Run to resume + the toolCall Item awaiting approval — the HITL
   *  resume target (API.md §6). When either is absent the card is a
   *  decorative pre-HITL preview with no buttons. */
  runId?: string;
  itemId?: string;
  /** Set once the decision is submitted (optimistic) / the run resolves. */
  decision?: ApprovalDecision;
  /** Tool arguments about to be executed. When present, the card lets the
   *  user edit them before approving (approve-with-modified-args, §4.3). */
  args?: Record<string, unknown>;
  /** Risk level — drives the badge colour + dot. Defaults to "medium"
   *  when omitted (older backends): "we don't know, be cautious". */
  risk?: ApprovalRisk;
  /** Whether this particular approval may create a standing approval rule. */
  rememberable?: boolean;
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

// Approval card — presentation shell. Submission coordination lives in
// useApprovalCardActions; this component renders against `status`:
//   - "complete"         → settled checkpoint row (decision is authoritative)
//   - "requires-action"  → action card with Approve / Decline buttons,
//                           or optimistic checkpoint while a submit is in
//                           flight (pending mirrors the user's last click)
//
// HITL flow (R-model, API.md §6):
//   1. Run ends with outcome.type="interrupt" carrying an approval Interrupt
//   2. Reducer materialises an approval block (status="requires-action")
//      bound to { runId, itemId }
//   3. User clicks → useApprovalSubmit resumes the run (new segment) via
//      runs.resume + optimistically settles the card (resolveInterrupt)
export function ApprovalCard({
  status,
  what,
  cmd,
  reason,
  runId,
  itemId,
  decision,
  args,
  risk,
  rememberable = false,
  scope,
  target,
  reversible,
}: Props) {
  const t = useT();

  const hasArgs = args !== undefined;
  const originalArgs = hasArgs ? JSON.stringify(args, null, 2) : "";
  const argsEditor = useApprovalArgsEditor({ originalArgs });

  const [remember, setRemember] = useState(false);
  const [rememberScope, setRememberScope] = useState<RememberScope>("session");
  const { pending, disabled, approve, decline } = useApprovalCardActions({
    runId,
    itemId,
    status,
    argsEditor: hasArgs ? argsEditor : undefined,
    rememberScope: rememberable && remember ? rememberScope : undefined,
  });

  const finalised = approvalSettledDecision(status, decision, pending);
  if (finalised === "approved") {
    return <HitlSettledRow label={t("approval.settled.approved")} />;
  }
  if (finalised === "declined") {
    return <Divider icon={<Icon name="x" size={11} />}>{t("approval.settled.declined")}</Divider>;
  }

  // Pre-decision card. Buttons disabled when not resumable (decorative preview),
  // while a request is in flight, OR once the interrupt is no longer pending:
  // settlePendingInterrupts downgrades an unacted interrupt to `incomplete` on
  // run-end precisely so its buttons can't resume a dead run.
  const riskView = approvalRiskView(risk);
  const scopeViews = approvalScopeViews(scope);
  const reversibilityView = approvalReversibilityView(reversible);
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
            "rounded-sm px-1.5 py-px text-[10px] font-medium",
            approvalRiskToneClass(riskView.tone),
          )}
        >
          {t(riskView.labelKey)}
        </span>
      }
    >
      <div className="mb-1.5 text-[16px] font-semibold leading-[1.4] text-fg">{what}</div>
      {/* Shell-prompt command line — only for command-style approvals. Other
          tools have no `cmd` (their payload is just args), so skip the box
          instead of rendering a lonely "$". Dark code chip on the light card. */}
      {cmd.trim() && (
        <code className="my-1.5 block whitespace-pre-wrap break-all rounded-[8px] bg-fg p-3 font-mono text-[12px] text-on-fg">
          $ {cmd}
        </code>
      )}
      {dangers.length > 0 && (
        <div className="my-1.5 flex items-start gap-2 rounded-[8px] bg-negative/10 px-3 py-2 text-[12px] leading-[1.5] text-negative">
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
      {/* Grants summary — spells out what approving actually permits (side-effect
          categories + target + reversibility) so the decision is informed, not a
          blind "OK". Presentation-only; the underlying scope/target/reversible
          fields are the protocol's, untouched. */}
      {(scopeViews.length > 0 || target || reversibilityView) && (
        <div className="mb-2 flex flex-wrap items-center gap-1.5">
          <span className="mr-0.5 text-[11px] font-medium text-fg-faint">
            {t("approval.grants")}
          </span>
          {scopeViews.map((view) => (
            <span
              key={view.scope}
              className={cn(
                "inline-flex items-center rounded-sm px-1.5 py-px font-mono text-[10.5px] font-semibold",
                approvalScopeToneClass(view.tone),
              )}
            >
              {view.scope}
            </span>
          ))}
          {target && (
            <span className="inline-flex items-center gap-1 rounded-sm bg-surface-2 px-1.5 py-px font-mono text-[11px] text-fg-muted">
              <Icon name="folder" size={10} className="text-fg-faint" />
              {target}
            </span>
          )}
          {reversibilityView && (
            <span
              className={cn(
                "inline-flex items-center gap-1 rounded-sm px-1.5 py-px font-mono text-[10.5px] font-semibold",
                approvalReversibilityToneClass(reversibilityView.tone),
              )}
            >
              {t(reversibilityView.labelKey)}
            </span>
          )}
        </div>
      )}
      <div className="mb-2 text-[13px] leading-[1.55] text-fg-muted">{reason}</div>
      <div className="flex items-center gap-2">
        <Button
          variant="primary"
          size="sm"
          data-slot="approval-approve"
          disabled={disabled}
          onClick={approve}
        >
          {t("approval.action.approve")}
          {!disabled && <kbd className="ml-1.5 font-mono text-[10px] opacity-60">⌘↵</kbd>}
        </Button>
        <Button
          variant="outline"
          size="sm"
          data-slot="approval-decline"
          disabled={disabled}
          onClick={decline}
        >
          {t("approval.action.decline")}
          {!disabled && <kbd className="ml-1.5 font-mono text-[10px] opacity-60">⇧⌘⌫</kbd>}
        </Button>
        {rememberable && (
          <label className="ml-auto flex items-center gap-1.5 text-[11.5px] text-fg-muted select-none">
            <Checkbox
              checked={remember}
              onCheckedChange={setRemember}
              ariaLabel={t("approval.remember")}
            />
            {t("approval.remember")}
          </label>
        )}
      </div>
      {/* Scope picker — only meaningful once "don't ask again" is on. Session
          keys the rule to this session, project to this folder, global everywhere. */}
      {rememberable && remember && (
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

// Tinted pill fills — no inset ring borders (§ light Geist recipe). Tone rides
// the semantic bg/text token alone.
function approvalRiskToneClass(tone: ApprovalTone): string {
  if (tone === "danger") return "bg-negative/10 text-negative";
  if (tone === "warning") return "bg-warning/10 text-warning";
  return "bg-fg/[0.06] text-fg-muted";
}

function approvalScopeToneClass(tone: ApprovalTone): string {
  if (tone === "danger") return "bg-negative/10 text-negative";
  if (tone === "warning") return "bg-warning/10 text-warning";
  return "bg-surface-2 text-fg-muted";
}

function approvalReversibilityToneClass(tone: ApprovalTone): string {
  if (tone === "danger") return "bg-negative/10 text-negative";
  return "bg-fg/[0.06] text-fg-muted";
}
