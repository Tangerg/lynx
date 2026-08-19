import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { CurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import type { AgentRunView } from "@/plugins/builtin/agent/public/viewState";
import { RootRunOutcome } from "./RootRunOutcome";

function completedRun(): AgentRunView {
  return {
    id: "run-completed",
    sessionId: "session-a",
    parentRunId: null,
    rootRunId: "run-completed",
    spawnedByItemId: null,
    status: "finished",
    activeSegmentId: null,
    outcome: { type: "completed" },
    metrics: {
      steps: 12,
      activeDurationMillis: 246_000,
      usage: { inputTokens: 82_400, outputTokens: 1_200, cacheReadTokens: 0, costUsd: 0.14 },
    },
    progress: null,
    createdAt: "2026-08-19T00:00:00Z",
    finishedAt: "2026-08-19T00:04:06Z",
  };
}

describe("RootRunOutcome", () => {
  afterEach(cleanup);

  it("does not append a completion-and-accounting footer after an ordinary turn", () => {
    const { container } = render(
      <RootRunOutcome material={CurrentRootMaterial.from(completedRun())} />,
    );

    expect(container.firstChild).toBeNull();
    expect(screen.queryByText("Completed")).toBeNull();
    expect(screen.queryByText(/82\.4k/)).toBeNull();
  });
});
