import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import { useState } from "react";
import type { Tone } from "@/lib/tone";
import { Badge, Button, Checkbox, Divider, Icon, Segmented } from "@/ui";
import { HitlCardShell, HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import { approvalHeadline } from "./approvalHeadline";
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
import { useApprovalArgsEditor } from "../../application/approvalArgsEditor";
import { useApprovalCardActions } from "../../application/approvalCardActions";
import { ApprovalArgsEditor } from "./ApprovalArgsEditor";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the action card with the
   *  Approve / Decline buttons; `"complete"` collapses to a settled
   *  checkpoint row driven by `decision`. */
  status: BlockStatus;
  /** The tool awaiting a decision. The headline is derived here, at render, so it
   *  follows the language the user is reading in. */
  toolName?: string;
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
  toolName,
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
  const runtimeAvailable = useRuntimeCommandsAvailable();

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
    runtimeAvailable,
  });

  const finalised = approvalSettledDecision(status, decision, pending);
  if (finalised === "approved") {
    return <HitlSettledRow label={t("approval.settled.approved")} />;
  }
  if (finalised === "declined") {
    return <Divider icon={<Icon name="x" size="xs" />}>{t("approval.settled.declined")}</Divider>;
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
      variant="warning"
      icon="shield"
      iconClassName="text-warning"
      label={t("approval.required")}
      trailing={<Badge tone={approvalBadgeTone(riskView.tone)}>{t(riskView.labelKey)}</Badge>}
    >
      <div className="mb-1 text-ui-md font-semibold leading-body text-fg">
        {approvalHeadline(t, toolName)}
      </div>
      {/* Shell-prompt command line — only for command-style approvals. Other
          tools have no `cmd` (their payload is just args), so skip the box
          instead of rendering a lonely "$".

          The recessed well, not an inverting ink slab: `bg-fg` flips with the
          scheme, so on a dark palette the single most consequential thing on the
          card — the command you are about to authorise — rendered as a white
          block, brighter than anything around it and in the opposite polarity
          from every other code surface in the app. */}
      {cmd.trim() && (
        <code className="my-1.5 block whitespace-pre-wrap break-all rounded-sm bg-sunken px-2.5 py-2 font-mono text-ui-sm text-fg">
          <span className="text-success">$</span> {cmd}
        </code>
      )}
      {dangers.length > 0 && (
        <div className="my-1.5 flex items-start gap-2 rounded-sm bg-negative-wash px-3 py-2 text-ui-md leading-body text-negative">
          <Icon name="alert" size="sm" className="mt-px shrink-0" />
          <span>
            <span className="font-semibold">{t("approval.danger")}</span>{" "}
            {dangers.map((key) => t(key)).join(" · ")}
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
          <span className="mr-0.5 text-ui-sm font-medium text-fg-faint">
            {t("approval.grants")}
          </span>
          {scopeViews.map((view) => (
            <Badge
              key={view.scope}
              tone={approvalBadgeTone(view.tone)}
              className="font-mono font-semibold"
            >
              {view.scope}
            </Badge>
          ))}
          {target && (
            <span className="inline-flex items-center gap-1 rounded-sm bg-surface-2 px-1.5 py-px font-mono text-ui-sm text-fg-muted">
              <Icon name="folder" size="xs" className="text-fg-faint" />
              {target}
            </span>
          )}
          {reversibilityView && (
            <Badge
              tone={approvalBadgeTone(reversibilityView.tone)}
              className="font-mono font-semibold"
            >
              {t(reversibilityView.labelKey)}
            </Badge>
          )}
        </div>
      )}
      <div className="mb-2 text-ui-md leading-body text-fg-muted">{reason}</div>
      <div className="flex items-center gap-2">
        {/* The label alone. These carried their combos as a raw <kbd> — a glyph
            strip inside a button, hand-spelled past the Kbd atom, telling you how to
            press the thing your pointer is already on. The bindings still work; the
            place to read them is the shortcuts pane. */}
        <Button variant="primary" size="sm" disabled={disabled} onClick={approve}>
          {t("approval.action.approve")}
        </Button>
        <Button variant="outline" size="sm" disabled={disabled} onClick={decline}>
          {t("approval.action.decline")}
        </Button>
        {rememberable && (
          <Checkbox
            disabled={!runtimeAvailable}
            checked={remember}
            onCheckedChange={setRemember}
            label={t("approval.remember")}
            className="ml-auto gap-1.5 text-ui-sm"
          />
        )}
      </div>
      {/* Scope picker — only meaningful once "don't ask again" is on. Session
          keys the rule to this session, project to this folder, global everywhere. */}
      {rememberable && remember && (
        <fieldset disabled={!runtimeAvailable} className="mt-2 flex justify-end">
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
        </fieldset>
      )}
    </HitlCardShell>
  );
}

// Tinted pill fills — no inset ring borders (§ light Geist recipe). Tone rides
// the semantic bg/text token alone.
//
// Risk, scope, reversibility — three words carrying a state, which is `Badge`. The
// atom exists because fourteen callsites paired a fill with an ink by hand, and three
// of the fourteen were here: `approvalRiskToneClass`, `approvalScopeToneClass` and
// `approvalReversibilityToneClass` were byte-identical but for one branch, which is
// what a duplicated table looks like just before the copies drift apart.
function approvalBadgeTone(tone: ApprovalTone): Tone | undefined {
  if (tone === "danger") return "negative";
  if (tone === "warning") return "warning";
  return undefined;
}
