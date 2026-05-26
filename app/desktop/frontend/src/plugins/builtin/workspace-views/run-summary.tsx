// Built-in plugin: "Run Summary" workspace view — task-end review
// surface for the most recent run (UX review §2.4 P0.3).
//
// Derives entirely from `view.timeline` (C2) + `view.toolCalls` — no
// new state. The job is: pick the last run boundary, walk the entries
// it contains, and categorise them into the four buckets a user wants
// after the agent stops:
//
//   - Changed files     (write_file / edit_file tool calls)
//   - Read files        (read_file)
//   - Commands run      (bash / shell tools)
//   - Approvals         (request → decision)
//   - Errors            (run-error entries)
//
// "Copy summary" produces a plaintext digest the user can paste into a
// PR description or chat — useful for the common "what did the agent
// do?" handoff.

import type { ReactNode } from "react";
import type { AgentViewState, TimelineEntry } from "@/protocol/agui/viewState";
import { useState } from "react";
import { EmptyState, Icon, IconButton, ScrollArea } from "@/components/common";
import { ViewHeader } from "@/components/views/ViewHeader";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useAgentSlice } from "@/state/agentStore";

interface ApprovalDigest {
  command: string;
  decision?: "approved" | "declined";
}

interface RunDigest {
  runId: string | null;
  startedAt: number | null;
  endedAt: number | null;
  status: "running" | "ok" | "err" | "unknown";
  changedFiles: { path: string; added?: number; removed?: number }[];
  readFiles: string[];
  commands: { cmd: string; status: "ok" | "err" }[];
  approvals: ApprovalDigest[];
  errors: string[];
}

// First non-whitespace token of a tool args string — used as the path
// for file-touching tools whose first arg is the path.
function firstToken(args: string): string {
  const m = args.match(/^([^\s(,]+)/);
  return m ? m[1] : "";
}

// Tools categorisation. Lookup map (not switch) so adding a new tool
// kind is one row. Unknown tool fns are ignored from the bucket-y
// digests but still counted in the timeline view itself.
const FILE_WRITE = new Set(["write_file", "edit_file", "create_file"]);
const FILE_READ = new Set(["read_file", "cat"]);
const SHELL_RUN = new Set(["bash", "shell", "run", "sh"]);

function deriveLatestRun(view: AgentViewState): RunDigest | null {
  // Walk timeline backwards for the last run-start. If none, no digest.
  let startIdx = -1;
  for (let i = view.timeline.length - 1; i >= 0; i--) {
    if (view.timeline[i].kind === "run-start") {
      startIdx = i;
      break;
    }
  }
  if (startIdx < 0) return null;

  const slice = view.timeline.slice(startIdx);
  const startEntry = slice[0];
  const terminal = slice.find(
    (e): e is TimelineEntry => e.kind === "run-end" || e.kind === "run-error",
  );

  const digest: RunDigest = {
    runId: startEntry.runId,
    startedAt: startEntry.ts,
    endedAt: terminal?.ts ?? null,
    status: terminal
      ? terminal.kind === "run-error"
        ? "err"
        : "ok"
      : view.run.running
        ? "running"
        : "unknown",
    changedFiles: [],
    readFiles: [],
    commands: [],
    approvals: [],
    errors: [],
  };

  // Track tool-start refs so we can pair them with their tool-end and
  // know which tools were *attempted* even if not yet ended.
  const startedTools = new Set<string>();
  for (const e of slice) {
    if (e.kind === "tool-start" && e.refId) {
      startedTools.add(e.refId);
    }
    if (e.kind === "run-error" && e.summary) {
      digest.errors.push(e.summary);
    }
    if (e.kind === "approval-request" && e.refId) {
      const result = slice.find((x) => x.kind === "approval-result" && x.refId === e.refId);
      digest.approvals.push({
        command: e.summary ?? "",
        decision:
          result?.status === "approved" || result?.status === "declined"
            ? result.status
            : undefined,
      });
    }
  }

  // Pull the categorised tool details from view.toolCalls — that's
  // where the args, status, added/removed counts already live.
  for (const id of startedTools) {
    const tool = view.toolCalls[id];
    if (!tool) continue;
    if (FILE_WRITE.has(tool.fn)) {
      const path = firstToken(tool.args);
      if (path) {
        digest.changedFiles.push({
          path,
          added: tool.added,
          removed: tool.removed,
        });
      }
    } else if (FILE_READ.has(tool.fn)) {
      const path = firstToken(tool.args);
      if (path) digest.readFiles.push(path);
    } else if (SHELL_RUN.has(tool.fn)) {
      digest.commands.push({
        cmd: tool.args || tool.fn,
        status: tool.status === "err" ? "err" : "ok",
      });
    }
  }

  return digest;
}

function durationText(start: number, end: number | null): string {
  if (!end) return "—";
  const sec = Math.round((end - start) / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  return `${min}m ${sec % 60}s`;
}

function buildPlaintext(d: RunDigest): string {
  const lines: string[] = [];
  lines.push(`Run ${d.runId ?? "(unknown)"} — ${d.status}`);
  if (d.changedFiles.length > 0) {
    lines.push("", "Changed files:");
    for (const f of d.changedFiles) {
      const diff =
        f.added != null || f.removed != null ? ` (+${f.added ?? 0} -${f.removed ?? 0})` : "";
      lines.push(`  ${f.path}${diff}`);
    }
  }
  if (d.commands.length > 0) {
    lines.push("", "Commands:");
    for (const c of d.commands) lines.push(`  [${c.status}] ${c.cmd}`);
  }
  if (d.approvals.length > 0) {
    lines.push("", "Approvals:");
    for (const a of d.approvals) lines.push(`  [${a.decision ?? "pending"}] ${a.command}`);
  }
  if (d.errors.length > 0) {
    lines.push("", "Errors:");
    for (const e of d.errors) lines.push(`  ${e}`);
  }
  return lines.join("\n");
}

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
  const view = useAgentSlice((v) => v);
  const digest = deriveLatestRun(view);
  const [copied, setCopied] = useState(false);

  if (!digest) {
    return (
      <>
        <ViewHeader icon="check" titleStrong title="Run summary" sub="No runs yet" />
        <ScrollArea>
          <EmptyState
            icon="check"
            title="Nothing to summarise yet"
            sub="When a run finishes, its changes, commands, approvals, and errors collect here."
          />
        </ScrollArea>
      </>
    );
  }

  const status = STATUS_LABEL[digest.status];
  const dur = digest.startedAt
    ? digest.endedAt
      ? durationText(digest.startedAt, digest.endedAt)
      : durationText(digest.startedAt, Date.now()) + " elapsed"
    : "";

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(buildPlaintext(digest));
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard can fail in non-secure contexts; silently ignore.
    }
  };

  return (
    <>
      <ViewHeader
        icon="check"
        titleStrong
        title="Run summary"
        sub={`run ${digest.runId ?? "—"} · ${dur}`}
        actions={
          <IconButton title={copied ? "Copied" : "Copy summary"} onClick={onCopy}>
            <Icon name={copied ? "check" : "copy"} size={14} />
          </IconButton>
        }
      />
      <ScrollArea>
        <div className="px-4 pb-2 pt-1">
          <span
            className={cn(
              "inline-flex items-center rounded-sm border px-1.5 py-px font-mono text-[10.5px] font-semibold uppercase tracking-wider",
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
            <div
              key={`${c.cmd}:${i}`}
              className="flex items-baseline gap-2 font-mono text-[12px]"
            >
              <Icon name="terminal" size={11} className="text-fg-faint" />
              <span
                className={cn(
                  "truncate",
                  c.status === "err" ? "text-negative" : "text-fg-muted",
                )}
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
                  "ml-auto rounded-xs px-1 text-[10px] font-semibold uppercase tracking-wider",
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
            <div
              key={`${e}:${i}`}
              className="flex items-baseline gap-2 text-[12px] text-negative"
            >
              <Icon name="bug" size={11} />
              <span className="whitespace-pre-wrap break-words">{e}</span>
            </div>
          ))}
        </Section>
      </ScrollArea>
    </>
  );
}

export const runSummaryView = definePlugin({
  name: "lyra.builtin.view-run-summary",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "run-summary",
      title: "Run summary",
      icon: "check",
      openByDefault: false,
      // Sits next to Timeline (35) — both are about "what happened".
      order: 36,
      component: RunSummaryTab,
    });
  },
});
