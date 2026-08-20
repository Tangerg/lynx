// Run digest — pure derivation from AgentSessionView.timeline + toolCalls.
//
// Picks the selected root Run's first available start and walks that exact Run
// identity across every continuation Segment, bucketing changed/read files,
// commands, approvals, and errors. The Run Summary workspace view consumes
// this; the derivation lives here so it can be unit-tested in isolation (and so
// future surfaces — telemetry export, end-of-run toasts — can reuse it).

import type { Translate } from "@/lib/i18n";
import type { ApprovalDecision } from "../domain/hitl";
import type {
  AgentRunOutcome,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { toolCategory } from "../domain/toolCategory";
import type { AgentRootAttention } from "../application/view/runTree";
import { projectPatchChanges } from "../public/patchResult";

export interface ApprovalDigest {
  command: string;
  decision?: ApprovalDecision;
}

export interface ChangedFile {
  path: string;
  added?: number;
  removed?: number;
}

export interface CommandDigest {
  cmd: string;
  status: "running" | "ok" | "err";
}

export interface RunDigest {
  runId: string | null;
  startedAt: number | null;
  endedAt: number | null;
  status: "running" | "waiting" | "ok" | "err" | "canceled" | "limit" | "unknown";
  changedFiles: ChangedFile[];
  readFiles: string[];
  commands: CommandDigest[];
  approvals: ApprovalDigest[];
  errors: string[];
}

export interface RunDigestSource {
  timeline: TimelineEntry[];
  toolCalls: Record<string, ToolCall>;
  attention: AgentRootAttention;
  outcome: AgentRunOutcome | null;
}

// First non-whitespace token of a tool args string — used as the path
// for file-touching tools whose first arg is the path.
function firstToken(args: string): string {
  const m = args.match(/^([^\s(,]+)/);
  return m ? (m[1] ?? "") : "";
}

// A read tool's path lives in its args object (JSON) — pull `path`/`file`,
// else fall back to the first token (a bare path string).
function argPath(args: string): string {
  const t = args.trim();
  if (t[0] === "{") {
    try {
      const o = JSON.parse(t) as Record<string, unknown>;
      const p = o.path;
      if (typeof p === "string") return p;
    } catch {
      /* not JSON — fall through */
    }
  }
  return firstToken(args);
}

export function deriveLatestRun(source: RunDigestSource): RunDigest | null {
  if (!source.attention.runId) return null;
  const runId = source.attention.runId;
  // A session timeline interleaves root and descendant Runs. The summary owns
  // one selected root, so child boundaries and tools cannot displace it.
  const startIdx = source.timeline.findIndex(
    (entry) => entry.kind === "run-start" && entry.runId === runId,
  );
  if (startIdx < 0) return null;

  const slice = source.timeline.slice(startIdx).filter((entry) => entry.runId === runId);
  // startIdx came from a successful in-bounds find above, so slice[0] exists.
  const startEntry = slice[0]!;
  const terminal = slice.find(
    (e): e is TimelineEntry => e.kind === "run-end" || e.kind === "run-error",
  );

  const digest: RunDigest = {
    runId: startEntry.runId,
    startedAt: startEntry.ts,
    endedAt: terminal?.ts ?? null,
    status: runDigestStatus(terminal, source.attention, source.outcome),
    changedFiles: [],
    readFiles: [],
    commands: [],
    approvals: [],
    errors: [],
  };

  // Keep the first causal occurrence of every Tool. Live folding contributes a
  // start and (usually) an end; a cold durable snapshot of a completed Item can
  // only contribute the end. Both paths own the same Tool read-model fact, and
  // neither should have to fabricate the other's observation to build a digest.
  const materializedTools = new Set<string>();
  const timelineApprovalRefs = new Set<string>();
  for (const e of slice) {
    if ((e.kind === "tool-start" || e.kind === "tool-end") && e.refId) {
      materializedTools.add(e.refId);
    }
    if (e.kind === "run-error" && e.summary) {
      digest.errors.push(e.summary);
    }
    if (e.kind === "approval-request" && e.refId) {
      timelineApprovalRefs.add(e.refId);
      const result = slice.find((x) => x.kind === "approval-result" && x.refId === e.refId);
      const durableDecision = source.toolCalls[e.refId]?.approvalDecision;
      digest.approvals.push({
        command: e.summary ?? "",
        decision:
          result?.status === "approved" || result?.status === "declined"
            ? result.status
            : durableDecision,
      });
    }
  }

  // Pull the categorised tool details from view.toolCalls — that's
  // where the args, status, added/removed counts already live.
  for (const id of materializedTools) {
    const tool = source.toolCalls[id];
    if (!tool || tool.runId !== runId) continue;
    if (tool.approvalDecision && !timelineApprovalRefs.has(id)) {
      digest.approvals.push({
        command: tool.command ?? tool.fn,
        decision: tool.approvalDecision,
      });
    }
    // Bucket by the §4.4.2 display category (derived from tool.name), the same
    // table the fold + icon routing use.
    const category = toolCategory(tool.name);
    if (category === "command") {
      digest.commands.push({
        // The command, not the row's human label: a digest pasted into a PR has to
        // say what actually ran.
        cmd: tool.command ?? tool.fn,
        // ok = clean exit; running = still in flight (must not flag it red);
        // err / denied did not run to a clean result.
        status: tool.status === "ok" ? "ok" : tool.status === "running" ? "running" : "err",
      });
    } else if (category === "fileEdit") {
      // One persisted PatchResult is the scope of one call. In particular, do
      // not expand this from the current worktree: that can include later calls
      // and user edits, while a restored Run Summary must remain historical.
      for (const change of projectPatchChanges(tool.result)) {
        digest.changedFiles.push({ path: change.path });
      }
    } else if (category === "read") {
      const path = argPath(tool.args) || tool.fn;
      if (path) digest.readFiles.push(path);
    }
  }

  return digest;
}

function runDigestStatus(
  terminal: TimelineEntry | undefined,
  attention: AgentRootAttention,
  outcome: AgentRunOutcome | null,
): RunDigest["status"] {
  if (!terminal) {
    if (attention.status === "running") return "running";
    if (attention.status === "waiting") return "waiting";
    return "unknown";
  }
  if (terminal.kind === "run-error") return "err";
  switch (outcome?.type) {
    case "completed":
      return "ok";
    case "canceled":
      return "canceled";
    case "maxSteps":
    case "maxBudget":
      return "limit";
    case "timedOut":
    case "failed":
    case "lost":
      return "err";
    default:
      return terminal.status === "ok" ? "ok" : "unknown";
  }
}

export function durationText(t: Translate, start: number, end: number | null): string {
  if (!end) return "—";
  const sec = Math.round((end - start) / 1000);
  if (sec < 60) return t("duration.seconds", { sec });
  const min = Math.floor(sec / 60);
  return t("duration.minutes", { min, sec: sec % 60 });
}

/** Plaintext rendering — for "Copy summary" / paste-into-PR workflows.
 *
 *  The section captions come from the catalogs like any other copy: this is what
 *  the user copies out of the app, and there is no reason their notes should be in
 *  a language the rest of the UI isn't. What stays verbatim is the runtime's own
 *  vocabulary — a run status, a command status, an approval decision — because
 *  those are the wire's words, and the bracketed slots would otherwise mix
 *  languages. A missing decision reads as "—" rather than a translated
 *  "pending" for the same reason. */
export function buildPlaintext(t: Translate, d: RunDigest): string {
  const lines: string[] = [];
  lines.push(t("runDigest.plaintext.run", { id: d.runId ?? "—", status: d.status }));
  if (d.changedFiles.length > 0) {
    lines.push("", t("runDigest.plaintext.changedFiles"));
    for (const f of d.changedFiles) {
      const diff =
        f.added != null || f.removed != null ? ` (+${f.added ?? 0} -${f.removed ?? 0})` : "";
      lines.push(`  ${f.path}${diff}`);
    }
  }
  if (d.commands.length > 0) {
    lines.push("", t("runDigest.plaintext.commands"));
    for (const c of d.commands) lines.push(`  [${c.status}] ${c.cmd}`);
  }
  if (d.approvals.length > 0) {
    lines.push("", t("runDigest.plaintext.approvals"));
    for (const a of d.approvals) lines.push(`  [${a.decision ?? "—"}] ${a.command}`);
  }
  if (d.errors.length > 0) {
    lines.push("", t("runDigest.plaintext.errors"));
    for (const e of d.errors) lines.push(`  ${e}`);
  }
  return lines.join("\n");
}
