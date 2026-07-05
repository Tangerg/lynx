import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { EmptyState, Icon, IconButton } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/utils";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { buildPlaintext } from "@/plugins/builtin/agent/public/runDigest";
import { useT } from "@/lib/i18n";
import { useLatestRunDigest } from "@/plugins/builtin/workspace/presentation/runSummaryView";
import {
  runSummaryApprovalBadge,
  runSummaryCommandClassName,
  runSummaryViewModel,
} from "@/plugins/builtin/workspace/application/runSummaryViewModel";

function Section({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: ReactNode;
}) {
  const t = useT();
  if (count === 0) return null;
  return (
    <div className="px-4 py-3">
      <div className="mb-1.5 flex items-baseline gap-2">
        <span className="text-[12px] font-semibold text-fg">{t(title)}</span>
        <span className="font-mono text-[11px] text-fg-faint">{count}</span>
      </div>
      <div className="grid gap-1">{children}</div>
    </div>
  );
}

function RunSummaryTab() {
  const t = useT();
  const digest = useLatestRunDigest();
  const [copied, setCopied] = useState(false);
  // Track + clear the "copied" reset timer so it can't fire setState after the
  // Run-Summary tab unmounts (same guard ShikiCodeBlock uses for its copy flag).
  const copyTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(copyTimer.current), []);

  if (!digest) {
    return (
      <WorkspaceViewLayout
        icon="check"
        titleStrong
        title="runSummary.title"
        sub={t("runSummary.noRuns")}
      >
        <EmptyState
          icon="check"
          title={t("runSummary.empty.title")}
          sub={t("runSummary.empty.sub")}
        />
      </WorkspaceViewLayout>
    );
  }

  const view = runSummaryViewModel(digest, { elapsedSuffix: t("runSummary.elapsed") });

  const onCopy = async () => {
    if (await copyText(buildPlaintext(digest))) {
      setCopied(true);
      clearTimeout(copyTimer.current);
      copyTimer.current = setTimeout(() => setCopied(false), 1500);
    }
  };

  return (
    <WorkspaceViewLayout
      icon="check"
      titleStrong
      title="runSummary.title"
      sub={view.subtext}
      actions={
        <IconButton title={t(copied ? "runSummary.copied" : "runSummary.copy")} onClick={onCopy}>
          <Icon name={copied ? "check" : "copy"} size={14} />
        </IconButton>
      }
    >
      <div className="px-4 pb-2 pt-1">
        <span
          className={cn(
            "inline-flex items-center rounded-sm px-1.5 py-px font-mono text-[10.5px] font-semibold",
            view.statusBadge.className,
          )}
        >
          {t(view.statusBadge.labelKey)}
        </span>
      </div>

      <Section title="runSummary.section.changedFiles" count={view.changedFiles.count}>
        {view.changedFiles.items.map((f) => (
          <div
            key={f.path}
            className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted"
          >
            <Icon name="filetext" size={11} className="text-fg-faint" />
            <span className="truncate text-fg">{f.path}</span>
            {(f.added != null || f.removed != null) && (
              <span className="ml-auto text-[11px]">
                <span className="text-success">+{f.added ?? 0}</span>
                {" / "}
                <span className="text-negative">-{f.removed ?? 0}</span>
              </span>
            )}
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.readFiles" count={view.readFiles.count}>
        {view.readFiles.items.map((p) => (
          <div key={p} className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted">
            <Icon name="filetext" size={11} className="text-fg-faint" />
            <span className="truncate">{p}</span>
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.commands" count={view.commands.count}>
        {view.commands.items.map((c, i) => (
          <div key={`${c.cmd}:${i}`} className="flex items-baseline gap-2 font-mono text-[12px]">
            <Icon name="terminal" size={11} className="text-fg-faint" />
            <span className={cn("truncate", runSummaryCommandClassName(c.status))}>{c.cmd}</span>
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.approvals" count={view.approvals.count}>
        {view.approvals.items.map((a, i) => {
          const approval = runSummaryApprovalBadge(a.decision);
          return (
            <div
              key={`${a.command}:${i}`}
              className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted"
            >
              <Icon name="shield" size={11} className="text-fg-faint" />
              <span className="truncate">{a.command || t("runSummary.approval.noCommand")}</span>
              <span
                className={cn(
                  "ml-auto rounded-sm px-1 text-[10px] font-semibold",
                  approval.className,
                )}
              >
                {t(approval.labelKey)}
              </span>
            </div>
          );
        })}
      </Section>

      <Section title="runSummary.section.errors" count={view.errors.count}>
        {view.errors.items.map((e, i) => (
          <div key={`${e}:${i}`} className="flex items-baseline gap-2 text-[12px] text-negative">
            <Icon name="bug" size={11} />
            <span className="whitespace-pre-wrap break-words">{e}</span>
          </div>
        ))}
      </Section>
    </WorkspaceViewLayout>
  );
}

export const runSummaryView = defineWorkspaceView({
  id: "run-summary",
  title: "workspace.view.title.runSummary",
  icon: "check",
  // Sits next to Timeline (35) — both are about "what happened".
  order: 36,
  splittable: true,
  component: RunSummaryTab,
});
