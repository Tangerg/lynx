import type { IconName } from "@/components/common";
import type { ApprovalModeValue } from "@/lib/data/queries";
import type { ReactNode } from "react";
import { DropdownMenu, Icon } from "@/components/common";
import { APPROVAL_MODES, setApprovalMode } from "@/plugins/builtin/agent/public/approvalPolicy";
import { rpcErrorText } from "@/lib/rpcErrors";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { useSelectedModel } from "./public/selectedModel";
import { useApprovalMode, useProjects } from "@/lib/data/queries";
import { fmtTokens } from "@/lib/format";
import { t, useT } from "@/lib/i18n";
import { notifyError } from "@/lib/notify";
import { basename } from "@/lib/path";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_STATUS } from "@/plugins/sdk/kernelPoints";
import { useAgentRunContextTokens, useAgentRunUsage } from "@/state/agentStore";

function Chip({ icon, title, children }: { icon: IconName; title: string; children: ReactNode }) {
  return (
    <span
      title={title}
      className="inline-flex items-center gap-1.5 text-[11.5px] text-fg-faint whitespace-nowrap transition-colors hover:text-fg-muted"
    >
      <Icon name={icon} size={11} className="shrink-0" />
      <span>{children}</span>
    </span>
  );
}

function CwdChip() {
  const cwd = useActiveSessionCwd();
  if (!cwd) return null;
  return (
    <Chip icon="folder" title={cwd}>
      {basename(cwd)}
    </Chip>
  );
}

function GitBranchChip() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const { data: projects } = useProjects();
  const branch = cwd ? projects?.find((p) => p.id === cwd)?.branch : undefined;
  if (!branch) return null;
  return (
    <Chip icon="branch" title={t("composer.gitBranch")}>
      {branch}
    </Chip>
  );
}

function ApprovalModeChip() {
  const t = useT();
  const { data: mode, isError } = useApprovalMode();
  if (isError || mode === undefined) return null;
  const current = APPROVAL_MODES.find((m) => m.value === mode) ?? APPROVAL_MODES[2]!;
  const onSelect = async (next: ApprovalModeValue) => {
    if (next === mode) return;
    try {
      await setApprovalMode(next);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    }
  };
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <button
            type="button"
            aria-label={t("approvals.mode.aria")}
            className="inline-flex items-center gap-1.5 text-[11.5px] text-fg-faint whitespace-nowrap transition-colors hover:text-fg-muted data-[popup-open]:text-fg"
          >
            <Icon
              name="shield"
              size={11}
              className={cn("shrink-0", mode === "yolo" ? "text-warning" : "")}
            />
            <span className={cn(mode === "yolo" && "text-warning")}>{t(current.labelKey)}</span>
            <Icon name="chevron-down" size={9} className="opacity-70" />
          </button>
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6} className="min-w-[248px]">
        {APPROVAL_MODES.map((m) => (
          <DropdownMenu.Item
            key={m.value}
            onClick={() => void onSelect(m.value)}
            className="grid grid-cols-[minmax(0,1fr)_14px] items-start gap-2 rounded-sm px-2 py-1.5 outline-none data-[highlighted]:bg-surface-2"
          >
            <span className="min-w-0">
              <span className="block text-[12.5px] font-semibold text-fg">{t(m.labelKey)}</span>
              <span className="block text-[11.5px] leading-snug text-fg-muted">{t(m.descKey)}</span>
            </span>
            {m.value === mode && <Icon name="check" size={12} className="mt-0.5 text-accent" />}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

function ctxTone(pct: number): string {
  if (pct >= 90) return "text-negative";
  if (pct >= 70) return "text-warning";
  return "text-fg-faint";
}

function UsageChip() {
  const usage = useAgentRunUsage();
  const contextTokens = useAgentRunContextTokens();
  const model = useSelectedModel();
  if (usage.inputTokens + usage.outputTokens === 0) return null;
  const window = model?.contextWindow;
  const pct =
    contextTokens !== undefined && window
      ? Math.min(100, Math.round((contextTokens / window) * 100))
      : undefined;
  return (
    <span
      title={t("composer.usage.hint")}
      className="inline-flex items-center gap-1.5 text-[11.5px] text-fg-faint whitespace-nowrap tabular-nums"
    >
      <span>↑{fmtTokens(usage.inputTokens)}</span>
      <span>↓{fmtTokens(usage.outputTokens)}</span>
      {usage.costUsd !== undefined && <span>·&nbsp;${usage.costUsd.toFixed(2)}</span>}
      {pct !== undefined && (
        <span className={ctxTone(pct)} title={t("composer.usage.context")}>
          ·&nbsp;{pct}%
        </span>
      )}
    </span>
  );
}

const chips = [
  { id: "cwd", order: 0, component: CwdChip },
  { id: "approval-mode", order: 1, component: ApprovalModeChip },
  { id: "git-branch", order: 2, component: GitBranchChip },
  { id: "usage", order: 3, component: UsageChip },
];

export const composerChips = definePlugin({
  name: "lyra.builtin.composer-chips",
  version: "1.0.0",
  setup({ host }) {
    for (const chip of chips) {
      host.extensions.contribute(COMPOSER_STATUS, chip);
    }
  },
});
