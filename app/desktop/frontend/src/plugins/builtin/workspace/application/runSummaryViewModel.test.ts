import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { RunDigest } from "@/plugins/builtin/agent/public/runDigest";
import {
  runSummaryApprovalBadge,
  runSummaryCommandTone,
  runSummarySubtext,
  runSummaryViewModel,
} from "./runSummaryViewModel";

const digest = (over: Partial<RunDigest> = {}): RunDigest => ({
  runId: "run-1",
  startedAt: 1_000,
  endedAt: 6_200,
  status: "ok",
  changedFiles: [{ path: "src/app.ts", added: 2, removed: 1 }],
  readFiles: ["README.md"],
  commands: [{ cmd: "npm test", status: "ok" }],
  approvals: [{ command: "rm tmp", decision: "declined" }],
  errors: ["failed"],
  ...over,
});

describe("runSummaryViewModel", () => {
  it("projects header status and section counts from a run digest", () => {
    expect(runSummaryViewModel(t, digest())).toEqual({
      subtext: "run run-1 · 5s",
      statusBadge: {
        labelKey: "runSummary.status.done",
        tone: "success",
      },
      changedFiles: {
        count: 1,
        items: [{ path: "src/app.ts", added: 2, removed: 1 }],
      },
      readFiles: {
        count: 1,
        items: ["README.md"],
      },
      commands: {
        count: 1,
        items: [{ cmd: "npm test", status: "ok" }],
      },
      approvals: {
        count: 1,
        items: [{ command: "rm tmp", decision: "declined" }],
      },
      errors: {
        count: 1,
        items: ["failed"],
      },
    });
  });

  it("projects running, waiting, and unknown status badges", () => {
    expect(
      runSummaryViewModel(t, digest({ status: "running", endedAt: null }), { now: 3_400 }),
    ).toMatchObject({
      subtext: "run run-1 · 2s elapsed",
      statusBadge: {
        labelKey: "runSummary.status.running",
        tone: "accent",
      },
    });

    expect(
      runSummaryViewModel(t, digest({ status: "waiting", endedAt: null })).statusBadge,
    ).toEqual({
      labelKey: "runSummary.status.waiting",
      tone: "warning",
    });

    expect(runSummaryViewModel(t, digest({ status: "unknown" })).statusBadge).toEqual({
      labelKey: "runSummary.status.unknown",
      tone: "neutral",
    });
  });

  it("uses non-success badges for canceled and limit outcomes", () => {
    expect(runSummaryViewModel(t, digest({ status: "canceled" })).statusBadge).toEqual({
      labelKey: "agent.runTree.status.canceled",
      tone: "neutral",
    });
    expect(runSummaryViewModel(t, digest({ status: "limit" })).statusBadge).toEqual({
      labelKey: "agent.runTree.status.limit",
      tone: "warning",
    });
  });
});

describe("runSummarySubtext", () => {
  it("omits the duration separator when the digest has no start time", () => {
    expect(runSummarySubtext(t, { runId: null, startedAt: null, endedAt: null })).toBe("run —");
  });

  it("uses the current time for an in-flight run", () => {
    expect(
      runSummarySubtext(t, { runId: "run-2", startedAt: 10_000, endedAt: null }, { now: 73_000 }),
    ).toBe("run run-2 · 1m 3s elapsed");
  });
});

describe("run summary row helpers", () => {
  it("maps command statuses to text color classes", () => {
    expect(runSummaryCommandTone("ok")).toBe("neutral");
    expect(runSummaryCommandTone("running")).toBe("neutral");
    expect(runSummaryCommandTone("err")).toBe("negative");
  });

  it("maps approval decisions to badge labels and tones", () => {
    expect(runSummaryApprovalBadge("approved")).toEqual({
      labelKey: "runSummary.approval.approved",
      tone: "success",
    });
    expect(runSummaryApprovalBadge("declined")).toEqual({
      labelKey: "runSummary.approval.declined",
      tone: "warning",
    });
    expect(runSummaryApprovalBadge(undefined)).toEqual({
      labelKey: "runSummary.approval.pending",
      tone: "neutral",
    });
  });
});
