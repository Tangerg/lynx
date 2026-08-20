// Unit tests for the runDigest derivation. These exist because the
// derivation logic used to live inline in run-summary.tsx where it
// couldn't be tested without rendering React; extracting it to a pure
// module lets us pin the bucketing rules directly.

import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { buildPlaintext, deriveLatestRun } from "./runDigest";

// Spread-helpers so tests stay terse — each entry only needs to set
// the fields it actually cares about.
let seq = 0;
const entry = (
  fields: Partial<Parameters<typeof deriveLatestRun>[0]["timeline"][number]>,
): Parameters<typeof deriveLatestRun>[0]["timeline"][number] => ({
  id: `tl:${++seq}`,
  ts: 1_000 + seq,
  kind: "run-start",
  runId: "r1",
  ...fields,
});

const view = (
  patch: Partial<Parameters<typeof deriveLatestRun>[0]>,
): Parameters<typeof deriveLatestRun>[0] => ({
  timeline: [],
  toolCalls: {},
  attention: { status: "finished", runId: "r1" },
  outcome: null,
  ...patch,
});

describe("deriveLatestRun", () => {
  it("returns null when no run has started", () => {
    expect(deriveLatestRun(view({ timeline: [] }))).toBeNull();
  });

  it("uses the selected root boundary and ignores another Run's entries", () => {
    const v = view({
      attention: { status: "finished", runId: "r2" },
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "run-end", runId: "r1", status: "ok" }),
        entry({ kind: "run-start", runId: "r2" }),
        entry({ kind: "tool-start", runId: "r2", refId: "t1", summary: "shell" }),
      ],
    });
    const d = deriveLatestRun(v);
    expect(d?.runId).toBe("r2");
    expect(d?.status).toBe("unknown"); // finished attention without a terminal is incomplete material
  });

  it("keeps the whole Run across HITL continuation Segment starts", () => {
    const firstStart = entry({ kind: "run-start", runId: "r1" });
    const v = view({
      timeline: [
        firstStart,
        entry({ kind: "tool-start", runId: "r1", refId: "before-hitl" }),
        entry({ kind: "approval-request", runId: "r1", refId: "approval-1" }),
        entry({ kind: "approval-result", runId: "r1", refId: "approval-1", status: "approved" }),
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "tool-start", runId: "r1", refId: "after-hitl" }),
        entry({ kind: "run-end", runId: "r1", status: "ok" }),
      ],
      toolCalls: {
        "before-hitl": {
          id: "before-hitl",
          runId: "r1",
          name: "shell",
          fn: "npm test",
          args: "",
          status: "ok",
        },
        "after-hitl": {
          id: "after-hitl",
          runId: "r1",
          name: "shell",
          fn: "npm run build",
          args: "",
          status: "ok",
        },
      },
      outcome: { type: "completed" },
    });

    expect(deriveLatestRun(v)).toMatchObject({
      startedAt: firstStart.ts,
      status: "ok",
      commands: [
        { cmd: "npm test", status: "ok" },
        { cmd: "npm run build", status: "ok" },
      ],
      approvals: [{ command: "", decision: "approved" }],
    });
  });

  it("rebuilds Tool material from a durable snapshot that only has tool-end", () => {
    const v = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        // sessions.snapshot replays a completed Item through completion
        // semantics. It owns the durable Tool fact and its terminal timeline
        // entry, but cannot invent a live item.started observation.
        entry({ kind: "tool-end", runId: "r1", refId: "cold-command", status: "ok" }),
        entry({ kind: "tool-end", runId: "r1", refId: "cold-edit", status: "ok" }),
        entry({ kind: "tool-end", runId: "r1", refId: "cold-read", status: "ok" }),
        entry({ kind: "run-end", runId: "r1", status: "ok" }),
      ],
      toolCalls: {
        "cold-command": {
          id: "cold-command",
          runId: "r1",
          name: "shell",
          fn: "Run tests",
          command: "npm test",
          args: "",
          status: "ok",
        },
        "cold-edit": {
          id: "cold-edit",
          runId: "r1",
          name: "apply_patch",
          fn: "src/runtime.ts",
          args: "",
          result: '{"changes":[{"path":"src/runtime.ts","status":"modified"}]}',
          status: "ok",
        },
        "cold-read": {
          id: "cold-read",
          runId: "r1",
          name: "read",
          fn: "read",
          args: '{"path":"src/model.ts"}',
          status: "ok",
        },
      },
      outcome: { type: "completed" },
    });

    expect(deriveLatestRun(v)).toMatchObject({
      commands: [{ cmd: "npm test", status: "ok" }],
      changedFiles: [{ path: "src/runtime.ts" }],
      readFiles: ["src/model.ts"],
    });
  });

  it("rebuilds exact approvals from cold Tool facts without duplicating live history", () => {
    const cold = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "tool-end", runId: "r1", refId: "approved-tool", status: "ok" }),
        entry({ kind: "tool-end", runId: "r1", refId: "declined-tool", status: "err" }),
        entry({ kind: "run-end", runId: "r1", status: "ok" }),
      ],
      toolCalls: {
        "approved-tool": {
          id: "approved-tool",
          runId: "r1",
          name: "shell",
          fn: "Run tests",
          command: "go test ./...",
          args: "",
          status: "ok",
          approvalDecision: "approved",
        },
        "declined-tool": {
          id: "declined-tool",
          runId: "r1",
          name: "shell",
          fn: "Remove build output",
          command: "rm -rf dist",
          args: "",
          status: "denied",
          approvalDecision: "declined",
        },
      },
      outcome: { type: "completed" },
    });
    expect(deriveLatestRun(cold)?.approvals).toEqual([
      { command: "go test ./...", decision: "approved" },
      { command: "rm -rf dist", decision: "declined" },
    ]);

    const live = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({
          kind: "approval-request",
          runId: "r1",
          refId: "approved-tool",
          summary: "go test ./...",
        }),
        entry({ kind: "approval-result", runId: "r1", refId: "approved-tool", status: "approved" }),
        entry({ kind: "tool-end", runId: "r1", refId: "approved-tool", status: "ok" }),
        entry({ kind: "run-end", runId: "r1", status: "ok" }),
      ],
      toolCalls: { "approved-tool": cold.toolCalls["approved-tool"]! },
      outcome: { type: "completed" },
    });
    expect(deriveLatestRun(live)?.approvals).toEqual([
      { command: "go test ./...", decision: "approved" },
    ]);

    const answeredElsewhere = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({
          kind: "approval-request",
          runId: "r1",
          refId: "approved-tool",
          summary: "go test ./...",
        }),
        entry({ kind: "tool-end", runId: "r1", refId: "approved-tool", status: "ok" }),
        entry({ kind: "run-end", runId: "r1", status: "ok" }),
      ],
      toolCalls: { "approved-tool": cold.toolCalls["approved-tool"]! },
      outcome: { type: "completed" },
    });
    expect(deriveLatestRun(answeredElsewhere)?.approvals).toEqual([
      { command: "go test ./...", decision: "approved" },
    ]);
  });

  it("flags status ok / err / running based on terminal entry", () => {
    const ok = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "run-end", runId: "r1", status: "ok" }),
      ],
      outcome: { type: "completed" },
    });
    expect(deriveLatestRun(ok)?.status).toBe("ok");

    const err = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "run-error", runId: "r1", summary: "boom" }),
      ],
    });
    const d = deriveLatestRun(err);
    expect(d?.status).toBe("err");
    expect(d?.errors).toEqual(["boom"]);

    const running = view({
      timeline: [entry({ kind: "run-start", runId: "r1" })],
      attention: { status: "running", runId: "r1" },
    });
    expect(deriveLatestRun(running)?.status).toBe("running");
  });

  it("does not collapse an HITL-waiting Run into unknown", () => {
    const waiting = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({
          kind: "approval-request",
          runId: "r1",
          refId: "approval-1",
          summary: "go test ./...",
        }),
      ],
      attention: { status: "waiting", runId: "r1" },
    });

    expect(deriveLatestRun(waiting)?.status).toBe("waiting");
  });

  it("does not report canceled and limit outcomes as successful", () => {
    const canceled = {
      ...view({
        timeline: [
          entry({ kind: "run-start", runId: "r1" }),
          entry({ kind: "run-end", runId: "r1", summary: "canceled" }),
        ],
      }),
      outcome: { type: "canceled" as const },
    };
    const limited = {
      ...view({
        timeline: [
          entry({ kind: "run-start", runId: "r1" }),
          entry({ kind: "run-end", runId: "r1", summary: "maxSteps" }),
        ],
      }),
      outcome: { type: "maxSteps" as const },
    };
    const unproven = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "run-end", runId: "r1" }),
      ],
    });

    expect(deriveLatestRun(canceled)?.status).toBe("canceled");
    expect(deriveLatestRun(limited)?.status).toBe("limit");
    expect(deriveLatestRun(unproven)?.status).toBe("unknown");
  });

  it("buckets file writes, file reads, and shell runs via toolCalls", () => {
    const v = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "tool-start", refId: "t-write", summary: "write_file" }),
        entry({ kind: "tool-start", refId: "t-read", summary: "read_file" }),
        entry({ kind: "tool-start", refId: "t-shell", summary: "shell" }),
        entry({ kind: "run-end", runId: "r1" }),
      ],
      toolCalls: {
        "t-write": {
          id: "t-write",
          runId: "r1",
          name: "apply_patch", // fileEdit category (§4.4.2)
          fn: "src/auth.ts", // toolLabel(apply_patch) = the changed path
          args: "",
          result: '{"changes":[{"path":"src/auth.ts","status":"modified"}]}',
          status: "ok",
        },
        "t-read": {
          id: "t-read",
          runId: "r1",
          name: "read", // read category (§4.4.2)
          fn: "read",
          args: "src/types.ts",
          status: "ok",
        },
        "t-shell": {
          id: "t-shell",
          runId: "r1",
          name: "shell", // command category (§4.4.2)
          fn: "pnpm test", // toolLabel(shell) = arguments.command
          args: "",
          status: "err",
        },
      },
    });
    const d = deriveLatestRun(v);
    expect(d?.changedFiles).toEqual([{ path: "src/auth.ts" }]);
    expect(d?.readFiles).toEqual(["src/types.ts"]);
    expect(d?.commands).toEqual([{ cmd: "pnpm test", status: "err" }]);
  });

  it("pairs approval-request with its approval-result by refId", () => {
    const v = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "approval-request", refId: "a1", summary: "psql migrate" }),
        entry({ kind: "approval-result", refId: "a1", status: "approved" }),
        entry({ kind: "approval-request", refId: "a2", summary: "rm -rf /tmp" }),
        entry({ kind: "run-end", runId: "r1" }),
      ],
    });
    const d = deriveLatestRun(v);
    expect(d?.approvals).toEqual([
      { command: "psql migrate", decision: "approved" },
      { command: "rm -rf /tmp", decision: undefined },
    ]);
  });
});

describe("buildPlaintext", () => {
  it("renders only the buckets that have entries", () => {
    const out = buildPlaintext(t, {
      runId: "r1",
      startedAt: 0,
      endedAt: 1000,
      status: "ok",
      changedFiles: [{ path: "src/a.ts", added: 3, removed: 1 }],
      readFiles: [],
      commands: [{ cmd: "pnpm test", status: "ok" }],
      approvals: [],
      errors: [],
    });
    expect(out).toContain("Run r1 — ok");
    expect(out).toContain("Changed files:");
    expect(out).toContain("src/a.ts (+3 -1)");
    expect(out).toContain("Commands:");
    expect(out).not.toContain("Read files");
    expect(out).not.toContain("Approvals");
    expect(out).not.toContain("Errors");
  });
});
