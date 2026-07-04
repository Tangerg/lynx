// Built-in plugin: "Run Summary" workspace view — task-end review
// surface for the most recent run (UX review §2.4 P0.3).
//
// Pure renderer. The "what did the agent just do" derivation lives in
// agent run-digest public surface (testable in isolation); this file only
// owns the React surface that displays the buckets + the "Copy
// summary" affordance.

import type { ReactNode } from "react";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import { useEffect, useRef, useState } from "react";
import { EmptyState, Icon, IconButton } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/utils";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { buildPlaintext, durationText } from "@/plugins/builtin/agent/public/runDigest";
import { useLatestRunDigest } from "@/plugins/builtin/workspace/presentation/runSummaryView";

function useStatusLabel(): Record<RunDigest["status"], { label: string; cls: string }> {
  const t = useT();
  return {
    ok: {
      label: t("runSummary.status.done"),
      cls: "bg-success/12 text-success",
    },
    err: {
      label: t("runSummary.status.errored"),
      cls: "bg-negative/12 text-negative",
    },
    running: {
      label: t("runSummary.status.running"),
      cls: "bg-accent/12 text-accent",
    },
    unknown: {
      label: t("runSummary.status.unknown"),
      cls: "bg-surface-2 text-fg-muted",
    },
  };
}

import { useT } from "@/lib/i18n";

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
  const statusLabel = useStatusLabel();
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

  const status = statusLabel[digest.status];
  const dur = digest.startedAt
    ? digest.endedAt
      ? durationText(digest.startedAt, digest.endedAt)
      : durationText(digest.startedAt, Date.now()) + t("runSummary.elapsed")
    : "";

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
      sub={`run ${digest.runId ?? "—"} · ${dur}`}
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
            status.cls,
          )}
        >
          {status.label}
        </span>
      </div>

      <Section title="runSummary.section.changedFiles" count={digest.changedFiles.length}>
        {digest.changedFiles.map((f) => (
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

      <Section title="runSummary.section.readFiles" count={digest.readFiles.length}>
        {digest.readFiles.map((p) => (
          <div key={p} className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted">
            <Icon name="filetext" size={11} className="text-fg-faint" />
            <span className="truncate">{p}</span>
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.commands" count={digest.commands.length}>
        {digest.commands.map((c, i) => (
          <div key={`${c.cmd}:${i}`} className="flex items-baseline gap-2 font-mono text-[12px]">
            <Icon name="terminal" size={11} className="text-fg-faint" />
            <span
              className={cn("truncate", c.status === "err" ? "text-negative" : "text-fg-muted")}
            >
              {c.cmd}
            </span>
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.approvals" count={digest.approvals.length}>
        {digest.approvals.map((a, i) => (
          <div
            key={`${a.command}:${i}`}
            className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted"
          >
            <Icon name="shield" size={11} className="text-fg-faint" />
            <span className="truncate">{a.command || t("runSummary.approval.noCommand")}</span>
            <span
              className={cn(
                "ml-auto rounded-sm px-1 text-[10px] font-semibold",
                a.decision === "approved"
                  ? "text-success"
                  : a.decision === "declined"
                    ? "text-warning"
                    : "text-fg-faint",
              )}
            >
              {a.decision
                ? t(`runSummary.approval.${a.decision}`)
                : t("runSummary.approval.pending")}
            </span>
          </div>
        ))}
      </Section>

      <Section title="runSummary.section.errors" count={digest.errors.length}>
        {digest.errors.map((e, i) => (
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
