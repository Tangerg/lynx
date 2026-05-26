// Unit tests for the runDigest derivation. These exist because the
// derivation logic used to live inline in run-summary.tsx where it
// couldn't be tested without rendering React; extracting it to a pure
// module lets us pin the bucketing rules directly.

import { describe, expect, it } from "vitest";
import { buildPlaintext, deriveLatestRun } from "./runDigest";
import { INITIAL_VIEW_STATE } from "./viewState";

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
  ...INITIAL_VIEW_STATE,
  ...patch,
});

describe("deriveLatestRun", () => {
  it("returns null when no run has started", () => {
    expect(deriveLatestRun(INITIAL_VIEW_STATE)).toBeNull();
  });

  it("picks the last run-start as the start boundary", () => {
    const v = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "run-end", runId: "r1" }),
        entry({ kind: "run-start", runId: "r2" }),
        entry({ kind: "tool-start", runId: "r2", refId: "t1", summary: "bash" }),
      ],
    });
    const d = deriveLatestRun(v);
    expect(d?.runId).toBe("r2");
    expect(d?.status).toBe("unknown"); // running:false, no terminal in slice
  });

  it("flags status ok / err / running based on terminal entry", () => {
    const ok = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "run-end", runId: "r1" }),
      ],
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
      run: { ...INITIAL_VIEW_STATE.run, running: true, runId: "r1" },
    });
    expect(deriveLatestRun(running)?.status).toBe("running");
  });

  it("buckets file writes, file reads, and shell runs via toolCalls", () => {
    const v = view({
      timeline: [
        entry({ kind: "run-start", runId: "r1" }),
        entry({ kind: "tool-start", refId: "t-write", summary: "write_file" }),
        entry({ kind: "tool-start", refId: "t-read", summary: "read_file" }),
        entry({ kind: "tool-start", refId: "t-bash", summary: "bash" }),
        entry({ kind: "run-end", runId: "r1" }),
      ],
      toolCalls: {
        "t-write": {
          id: "t-write",
          fn: "write_file",
          args: "src/auth.ts",
          status: "ok",
          duration: "12ms",
          added: 5,
          removed: 2,
        },
        "t-read": {
          id: "t-read",
          fn: "read_file",
          args: "src/types.ts",
          status: "ok",
          duration: "3ms",
        },
        "t-bash": {
          id: "t-bash",
          fn: "bash",
          args: "pnpm test",
          status: "err",
          duration: "1.4s",
        },
      },
    });
    const d = deriveLatestRun(v);
    expect(d?.changedFiles).toEqual([{ path: "src/auth.ts", added: 5, removed: 2 }]);
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
    const out = buildPlaintext({
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
