// Run digest — pure derivation from AgentViewState.timeline + toolCalls.
//
// Picks the most recent RUN_STARTED boundary and walks forward, bucketing
// the entries into a structured summary: changed/read files, commands,
// approvals, errors. The Run Summary workspace view consumes this; the
// derivation lives here so it can be unit-tested in isolation (and so
// future surfaces — telemetry export, end-of-run toasts — can reuse it).

import type { AgentViewState, TimelineEntry } from "./viewState";

export interface ApprovalDigest {
  command: string;
  decision?: "approved" | "declined";
}

export interface ChangedFile {
  path: string;
  added?: number;
  removed?: number;
}

export interface CommandDigest {
  cmd: string;
  status: "ok" | "err";
}

export interface RunDigest {
  runId: string | null;
  startedAt: number | null;
  endedAt: number | null;
  status: "running" | "ok" | "err" | "unknown";
  changedFiles: ChangedFile[];
  readFiles: string[];
  commands: CommandDigest[];
  approvals: ApprovalDigest[];
  errors: string[];
}

// First non-whitespace token of a tool args string — used as the path
// for file-touching tools whose first arg is the path.
function firstToken(args: string): string {
  const m = args.match(/^([^\s(,]+)/);
  return m ? (m[1] ?? "") : "";
}

// Tool categorisation. Lookup sets (not switch) so adding a new tool
// kind is one row. Unknown tool fns are ignored from the bucket-y
// digests but still counted in the timeline view itself.
const FILE_WRITE = new Set(["write_file", "edit_file", "create_file"]);
const FILE_READ = new Set(["read_file", "cat"]);
const SHELL_RUN = new Set(["bash", "shell", "run", "sh"]);

export function deriveLatestRun(view: AgentViewState): RunDigest | null {
  // Walk timeline backwards for the last run-start. If none, no digest.
  const startIdx = view.timeline.findLastIndex((e) => e.kind === "run-start");
  if (startIdx < 0) return null;

  const slice = view.timeline.slice(startIdx);
  // startIdx came from a successful in-bounds find above, so slice[0] exists.
  const startEntry = slice[0]!;
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
        // Only a successfully-run command is "ok"; err / denied did not run
        // to a clean result.
        status: tool.status === "ok" ? "ok" : "err",
      });
    }
  }

  return digest;
}

export function durationText(start: number, end: number | null): string {
  if (!end) return "—";
  const sec = Math.round((end - start) / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  return `${min}m ${sec % 60}s`;
}

/** Plaintext rendering — for "Copy summary" / paste-into-PR workflows. */
export function buildPlaintext(d: RunDigest): string {
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
