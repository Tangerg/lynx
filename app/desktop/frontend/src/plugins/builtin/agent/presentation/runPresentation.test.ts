import { describe, expect, it } from "vitest";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { agentRunDetail, agentRunPresentationState, agentRunStepCount } from "./runPresentation";

function run(overrides: Partial<AgentRunView> = {}): AgentRunView {
  return {
    id: "run-1",
    sessionId: "session-1",
    parentRunId: null,
    rootRunId: "run-1",
    spawnedByItemId: null,
    status: "running",
    activeSegmentId: "segment-1",
    outcome: null,
    metrics: {
      steps: 3,
      activeDurationMillis: 100,
      usage: { inputTokens: 10, outputTokens: 2, cacheReadTokens: 0 },
    },
    progress: { step: 4, activity: "Inspecting tests" },
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: null,
    ...overrides,
  };
}

describe("Run presentation facts", () => {
  it("keeps running and waiting distinct and reports live activity", () => {
    expect(agentRunPresentationState(run())).toBe("running");
    expect(agentRunPresentationState(run({ status: "waiting", activeSegmentId: null }))).toBe(
      "waiting",
    );
    expect(agentRunDetail(run())).toBe("Inspecting tests");
    expect(agentRunStepCount(run())).toBe(4);
  });

  it.each([
    [{ type: "completed" } as const, "finished"],
    [{ type: "failed", error: { message: "Provider failed" } } as const, "error"],
    [{ type: "timedOut", error: { message: "Provider timed out" } } as const, "error"],
    [{ type: "lost", error: { message: "Runtime restarted" } } as const, "error"],
    [{ type: "canceled", detail: "Stopped by user" } as const, "canceled"],
    [{ type: "maxSteps", detail: "Step limit" } as const, "limit"],
    [{ type: "maxBudget", detail: "Budget limit" } as const, "limit"],
  ])("projects terminal outcome %o as %s", (outcome, expected) => {
    const finished = run({
      status: "finished",
      activeSegmentId: null,
      progress: null,
      outcome,
      finishedAt: "2026-01-01T00:00:01.000Z",
    });
    expect(agentRunPresentationState(finished)).toBe(expected);
    expect(agentRunDetail(finished)).toBe(
      outcome.type === "completed"
        ? null
        : outcome.type === "failed" || outcome.type === "timedOut" || outcome.type === "lost"
          ? outcome.error.message
          : outcome.detail,
    );
    expect(agentRunStepCount(finished)).toBe(3);
  });
});
