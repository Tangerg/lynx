import { toolCategory, type BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import { type ApprovalDecision, type RememberScope } from "@/plugins/builtin/agent/public/hitl";
import { approvalSettledDecision } from "@/plugins/builtin/agent/public/messagePresentation";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { useT } from "@/lib/i18n";
import { Button, Divider, DropdownMenu, Icon, Surface, type IconName } from "@/ui";
import { useApprovalArgsEditor } from "../../application/approvalArgsEditor";
import { useApprovalCardActions } from "../../application/approvalCardActions";
import { ApprovalArgsEditor } from "./ApprovalArgsEditor";
import { approvalHeadline } from "./approvalHeadline";
import { HitlSettledRow } from "./HitlCard";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the request surface;
   *  `"complete"` collapses to a settled checkpoint driven by `decision`. */
  status: BlockStatus;
  /** Runtime tool awaiting the exact Run/Item decision. */
  toolName?: string;
  cmd: string;
  reason: string;
  /** The Run to resume + the toolCall Item awaiting approval — the HITL
   *  resume target (API.md §6). Without either identity this is a decorative
   *  preview: it renders the material but exposes no live action. */
  runId?: string;
  itemId?: string;
  /** Set once the decision is submitted (optimistic) / the run resolves. */
  decision?: ApprovalDecision;
  /** Tool arguments about to be executed. When present, the user may edit them
   *  before approving (approve-with-modified-args, §4.3). */
  args?: Record<string, unknown>;
  /** Whether this approval may create a standing allow rule. */
  rememberable?: boolean;
}

const REMEMBER_ACTIONS: readonly {
  scope: RememberScope;
  labelKey: string;
}[] = [
  { scope: "session", labelKey: "approval.action.allowSession" },
  { scope: "project", labelKey: "approval.action.allowProject" },
  { scope: "global", labelKey: "approval.action.allowGlobal" },
];

// Approval request — one Codex request surface around Runtime-authored material.
// Submission coordination remains in useApprovalCardActions:
//   - Allow once and the registered keyboard action omit remember scope.
//   - A scope appears only when the user explicitly picks a scoped allow action.
//   - Deny never inherits an allow scope.
export function ApprovalCard({
  status,
  toolName,
  cmd,
  reason,
  runId,
  itemId,
  decision,
  args,
  rememberable = false,
}: Props) {
  const t = useT();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const hasArgs = args !== undefined;
  const argsEditor = useApprovalArgsEditor({
    originalArgs: hasArgs ? JSON.stringify(args, null, 2) : "",
  });
  const { pending, disabled, approve, decline } = useApprovalCardActions({
    runId,
    itemId,
    status,
    argsEditor: hasArgs ? argsEditor : undefined,
    runtimeAvailable,
  });

  const finalised = approvalSettledDecision(status, decision, pending);
  if (finalised === "approved") {
    return <HitlSettledRow label={t("approval.settled.approved")} />;
  }
  if (finalised === "declined") {
    return <Divider icon={<Icon name="x" size="xs" />}>{t("approval.settled.declined")}</Divider>;
  }

  const identity = approvalIdentity(t, toolName);
  const title = reason.trim() || approvalHeadline(t, toolName);

  return (
    <Surface inset="none" data-slot="approval-surface" className="overflow-hidden rounded-3xl">
      <div className="px-4 pt-4 pb-3">
        <div className="flex min-w-0 items-center gap-2 text-ui-sm leading-body text-fg-muted">
          <Icon name={identity.icon} size="sm" className="shrink-0 text-fg-faint" />
          <span className="truncate">{identity.label}</span>
        </div>
        <div className="mt-2 text-pretty text-ui-md font-medium leading-body text-fg">{title}</div>
      </div>

      {(cmd.trim() || hasArgs) && (
        <div className="flex flex-col gap-2 px-4 pb-2">
          {cmd.trim() && (
            <code className="block max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-md bg-sunken px-2.5 py-2 font-mono text-ui-sm font-medium text-fg">
              {cmd}
            </code>
          )}
          {hasArgs && (
            <ApprovalArgsEditor
              editing={argsEditor.editing}
              argsText={argsEditor.argsText}
              invalid={argsEditor.invalid}
              onEditToggle={argsEditor.setEditing}
              onTextChange={argsEditor.setArgsText}
            />
          )}
        </div>
      )}

      <div className="flex flex-wrap items-center justify-end gap-2 px-4 pt-2 pb-4">
        <Button variant="outline" size="sm" disabled={disabled} onClick={decline}>
          {t("approval.action.deny")}
        </Button>
        <div
          role="group"
          aria-label={t("approval.action.allowOptions")}
          className="flex items-center"
        >
          <Button
            variant="primary"
            size="sm"
            disabled={disabled}
            className={rememberable ? "rounded-r-none" : undefined}
            onClick={() => approve()}
          >
            {t("approval.action.allowOnce")}
          </Button>
          {rememberable && (
            <DropdownMenu.Root>
              <DropdownMenu.Trigger
                render={
                  <Button
                    variant="primary"
                    size="icon-sm"
                    disabled={disabled}
                    aria-label={t("approval.action.allowOptions")}
                    className="relative -ml-px rounded-l-none before:pointer-events-none before:absolute before:inset-y-1.5 before:left-0 before:w-px before:bg-cta-text/20"
                  >
                    <Icon name="chevron-down" size="xs" />
                  </Button>
                }
              />
              <DropdownMenu.Content align="end" sideOffset={4} className="min-w-[196px]">
                {REMEMBER_ACTIONS.map((action) => (
                  <DropdownMenu.Item
                    key={action.scope}
                    onClick={() => approve(action.scope)}
                    className="grid-cols-[minmax(0,1fr)]"
                  >
                    <span className="truncate">{t(action.labelKey)}</span>
                  </DropdownMenu.Item>
                ))}
              </DropdownMenu.Content>
            </DropdownMenu.Root>
          )}
        </div>
      </div>
    </Surface>
  );
}

function approvalIdentity(
  t: (key: string, params?: Record<string, string | number>) => string,
  toolName: string | undefined,
): { icon: IconName; label: string } {
  if (!toolName) {
    return { icon: "shield", label: t("approval.identity.permission") };
  }
  switch (toolCategory(toolName)) {
    case "command":
      return { icon: "terminal", label: t("approval.identity.terminal") };
    case "fileEdit":
      return { icon: "edit", label: t("approval.identity.fileEdits") };
    default:
      return { icon: "tool", label: toolName };
  }
}
