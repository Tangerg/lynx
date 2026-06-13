// Built-in plugin: "Run Summary" workspace view — task-end review
// surface for the most recent run (UX review §2.4 P0.3).
//
// Pure renderer. The "what did the agent just do" derivation lives in
// `protocol/run/runDigest.ts` (testable in isolation); this file only
// owns the React surface that displays the buckets + the "Copy
// summary" affordance.

import type { ReactNode } from "react";
import type { RunDigest } from "@/protocol/run/runDigest";
import { useEffect, useMemo, useRef, useState } from "react";
import { EmptyState, Icon, IconButton } from "@/components/common";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/utils";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { buildPlaintext, deriveLatestRun, durationText } from "@/protocol/run/runDigest";
import { INITIAL_VIEW_STATE } from "@/protocol/run/viewState";
import { useAgentSlice } from "@/state/agentStore";

const STATUS_LABEL: Record<RunDigest["status"], { label: string; cls: string }> = {
  ok: { label: "Done", cls: "border-positive/40 bg-positive/12 text-positive" },
  err: { label: "Errored", cls: "border-negative/40 bg-negative/15 text-negative" },
  running: { label: "Running", cls: "border-accent/40 bg-accent/12 text-accent" },
  unknown: { label: "Unknown", cls: "border-line bg-surface-2 text-fg-muted" },
};

function Section({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: ReactNode;
}) {
  if (count === 0) return null;
  return (
    <div className="px-4 py-3">
      <div className="mb-1.5 flex items-baseline gap-2">
        <span className="text-[12px] font-semibold text-fg">{title}</span>
        <span className="font-mono text-[11px] text-fg-faint">{count}</span>
      </div>
      <div className="grid gap-1">{children}</div>
    </div>
  );
}

function RunSummaryTab() {
  // Subscribe only to the three slices the digest actually depends on.
  // Going through `useAgentSlice((v) => v)` would re-render this tab on
  // every TEXT_MESSAGE_CONTENT during streaming, even though messages
  // don't affect the summary. Timeline is the dominant change driver.
  const timeline = useAgentSlice((v) => v.timeline);
  const toolCalls = useAgentSlice((v) => v.toolCalls);
  const running = useAgentSlice((v) => v.run.running);

  const digest = useMemo(
    () =>
      deriveLatestRun({
        ...INITIAL_VIEW_STATE,
        timeline,
        toolCalls,
        run: { ...INITIAL_VIEW_STATE.run, running },
      }),
    [timeline, toolCalls, running],
  );

  const [copied, setCopied] = useState(false);
  // Track + clear the "copied" reset timer so it can't fire setState after the
  // Run-Summary tab unmounts (same guard ShikiCodeBlock uses for its copy flag).
  const copyTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(copyTimer.current), []);

  if (!digest) {
    return (
      <WorkspaceViewLayout icon="check" titleStrong title="Run summary" sub="No runs yet">
        <EmptyState
          icon="check"
          title="Nothing to summarise yet"
          sub="When a run finishes, its changes, commands, approvals, and errors collect here."
        />
      </WorkspaceViewLayout>
    );
  }

  const status = STATUS_LABEL[digest.status];
  const dur = digest.startedAt
    ? digest.endedAt
      ? durationText(digest.startedAt, digest.endedAt)
      : durationText(digest.startedAt, Date.now()) + " elapsed"
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
      title="Run summary"
      sub={`run ${digest.runId ?? "—"} · ${dur}`}
      actions={
        <IconButton title={copied ? "Copied" : "Copy summary"} onClick={onCopy}>
          <Icon name={copied ? "check" : "copy"} size={14} />
        </IconButton>
      }
    >
      <div className="px-4 pb-2 pt-1">
        <span
          className={cn(
            "inline-flex items-center rounded-sm border px-1.5 py-px font-mono text-[10.5px] font-semibold",
            status.cls,
          )}
        >
          {status.label}
        </span>
      </div>

      <Section title="Changed files" count={digest.changedFiles.length}>
        {digest.changedFiles.map((f) => (
          <div
            key={f.path}
            className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted"
          >
            <Icon name="filetext" size={11} className="text-fg-faint" />
            <span className="truncate text-fg">{f.path}</span>
            {(f.added != null || f.removed != null) && (
              <span className="ml-auto text-[11px]">
                <span className="text-positive">+{f.added ?? 0}</span>
                {" / "}
                <span className="text-negative">-{f.removed ?? 0}</span>
              </span>
            )}
          </div>
        ))}
      </Section>

      <Section title="Read files" count={digest.readFiles.length}>
        {digest.readFiles.map((p) => (
          <div key={p} className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted">
            <Icon name="filetext" size={11} className="text-fg-faint" />
            <span className="truncate">{p}</span>
          </div>
        ))}
      </Section>

      <Section title="Commands" count={digest.commands.length}>
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

      <Section title="Approvals" count={digest.approvals.length}>
        {digest.approvals.map((a, i) => (
          <div
            key={`${a.command}:${i}`}
            className="flex items-baseline gap-2 font-mono text-[12px] text-fg-muted"
          >
            <Icon name="shield" size={11} className="text-fg-faint" />
            <span className="truncate">{a.command || "(no command)"}</span>
            <span
              className={cn(
                "ml-auto rounded-xs px-1 text-[10px] font-semibold",
                a.decision === "approved"
                  ? "text-positive"
                  : a.decision === "declined"
                    ? "text-warning"
                    : "text-fg-faint",
              )}
            >
              {a.decision ?? "pending"}
            </span>
          </div>
        ))}
      </Section>

      <Section title="Errors" count={digest.errors.length}>
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
  title: "Run summary",
  icon: "check",
  openByDefault: false,
  // Sits next to Timeline (35) — both are about "what happened".
  order: 36,
  component: RunSummaryTab,
});
