// segment.progress is the ephemeral mid-run readout (step / usage / cost / activity);
// segment.finished.result is the authoritative landing (API.md §5.2). The reducer
// must surface progress live AND let the finished totals win.
import { beforeEach, describe, expect, it } from "vitest";
import type { AgentStreamEvent as StreamEvent } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { foldTestEvent as reduce, runFinished } from "./reducer.fixtures";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { selectCurrentRootRun } from "../view/runTree";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const runStarted = (id: string): StreamEvent => ({
  type: "segment.started",
  run: { id, sessionId: "ses_1" } as never,
});
const progress = (p: Record<string, unknown>): StreamEvent =>
  ({ type: "segment.progress", progress: p }) as StreamEvent;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
});

describe("reducer — segment.progress (mid-run live readout)", () => {
  it("surfaces step / activity / tokens / cost while the run streams", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, runStarted("run_1"));
    s = reduce(
      s,
      progress({
        step: 2,
        activity: "calling tool: ls -la",
        usage: { inputTokens: 1200, outputTokens: 80, cacheReadTokens: 0, costUsd: 0.0123 },
      }),
    );
    expect(selectCurrentRootRun(s)).toMatchObject({
      status: "running",
      progress: {
        step: 2,
        activity: "calling tool: ls -la",
        usage: {
          inputTokens: 1200,
          outputTokens: 80,
          cacheReadTokens: 0,
          costUsd: 0.0123,
        },
      },
    });
  });

  it("segment.finished totals are authoritative over the last progress preview", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ step: 1, usage: { inputTokens: 10, outputTokens: 5 } }));
    s = reduce(
      s,
      runFinished(
        { type: "completed" },
        {
          steps: 3,
          activeDurationMillis: 0,
          usage: { inputTokens: 1200, outputTokens: 80, cacheReadTokens: 0, costUsd: 0.5 },
        },
      ),
    );
    const run = selectCurrentRootRun(s)!;
    expect(run.status).toBe("finished");
    expect(run.metrics.steps).toBe(3);
    expect(run.metrics.usage).toEqual({
      inputTokens: 1200,
      outputTokens: 80,
      cacheReadTokens: 0,
      costUsd: 0.5,
    });
  });

  it("a progress event carrying only `activity` patches just that field", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ step: 4 }));
    s = reduce(s, progress({ activity: "thinking" }));
    expect(selectCurrentRootRun(s)?.progress).toMatchObject({
      step: 4,
      activity: "thinking",
    });
  });

  it("surfaces contextTokens (the live context-window footprint driving compaction)", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ contextTokens: 45_000 }));
    expect(selectCurrentRootRun(s)?.progress?.contextTokens).toBe(45_000);
  });

  it("retains the last context footprint when terminal metrics replace live progress", () => {
    let s: AgentSessionView = EMPTY_AGENT_SESSION_VIEW;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ contextTokens: 45_000, activity: "thinking", step: 2 }));
    s = reduce(
      s,
      runFinished(
        { type: "completed" },
        {
          steps: 3,
          activeDurationMillis: 10,
          usage: { inputTokens: 80_000, outputTokens: 1_000, cacheReadTokens: 40_000 },
        },
      ),
    );

    expect(selectCurrentRootRun(s)?.progress).toEqual({ contextTokens: 45_000 });
  });
});
