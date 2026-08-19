import { describe, expect, it } from "vitest";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import type { TranscriptRow } from "../conversation/transcriptRows";
import { CurrentRootMaterial } from "./runReadModel";

function run(id: string, status: AgentRunView["status"]): AgentRunView {
  return {
    id,
    sessionId: "session-root-material",
    parentRunId: null,
    rootRunId: id,
    spawnedByItemId: null,
    status,
    activeSegmentId: status === "finished" ? null : `segment-${id}`,
    outcome: status === "finished" ? { type: "completed" } : null,
    metrics: {
      steps: 0,
      activeDurationMillis: 0,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
    progress: null,
    createdAt: "2026-08-18T00:00:00.000Z",
    finishedAt: status === "finished" ? "2026-08-18T00:00:01.000Z" : null,
  };
}

function row(id: string, owner: TranscriptRow["runOwner"]): TranscriptRow {
  return {
    message: {
      id,
      role: id.startsWith("local-") ? "user" : "assistant",
      runId: owner.kind === "owned" ? owner.runId : null,
      blocks: [],
    },
    runOwner: owner,
    facts: { toolCalls: {}, delegatedRuns: {} },
  };
}

describe("CurrentRootMaterial", () => {
  it("publishes the latest prompt footprint independently from cumulative metrics", () => {
    const active = {
      ...run("run-context", "running"),
      progress: { contextTokens: 198_000 },
      metrics: {
        steps: 4,
        activeDurationMillis: 100,
        usage: { inputTokens: 900_000, outputTokens: 10_000, cacheReadTokens: 500_000 },
      },
    };

    expect(CurrentRootMaterial.from(active).contextTokens).toBe(198_000);
  });

  it("assigns terminal material to the finished Run's last exact narrative row", () => {
    const material = CurrentRootMaterial.from(run("run-a", "finished"));
    const rows = [
      row("assistant-a-1", { kind: "owned", runId: "run-a", status: "finished" }),
      row("assistant-a-2", { kind: "owned", runId: "run-a", status: "finished" }),
      row("local-successor", { kind: "unassigned" }),
    ];

    expect(material.terminalTurnIndex(rows)).toBe(1);
    expect(material.attention).toEqual({ status: "finished", runId: "run-a" });
    expect(material.outcome).toEqual({ type: "completed" });
  });

  it("gives no terminal ownership to an unfinished or absent root", () => {
    const rows = [row("assistant-a", { kind: "owned", runId: "run-a", status: "running" })];

    expect(CurrentRootMaterial.from(run("run-a", "running")).terminalTurnIndex(rows)).toBe(-1);
    expect(CurrentRootMaterial.idle.terminalTurnIndex(rows)).toBe(-1);
    expect(CurrentRootMaterial.idle.running).toBe(false);
  });

  it("does not reserve terminal footer layout for a failure owned by the recovery banner", () => {
    const failed: AgentRunView = {
      ...run("run-failed", "finished"),
      outcome: { type: "failed", error: { message: "Provider failed" } },
    };
    const rows = [
      row("assistant-failed", { kind: "owned", runId: "run-failed", status: "finished" }),
    ];

    expect(CurrentRootMaterial.from(failed).terminalTurnIndex(rows)).toBe(-1);
  });
});
