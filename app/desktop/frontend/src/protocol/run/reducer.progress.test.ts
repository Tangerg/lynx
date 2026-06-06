// run.progress is the ephemeral mid-run readout (step / usage / cost / activity);
// run.finished.result is the authoritative landing (API.md §5.2). The reducer
// must surface progress live AND let the finished totals win.
import { beforeEach, describe, expect, it } from "vitest";
import type { RunOutcome, StreamEvent } from "@/rpc";
import type { AgentViewState } from "./viewState";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "./viewState";

const runStarted = (id: string): StreamEvent => ({
  type: "run.started",
  run: { id, sessionId: "ses_1" } as never,
});
const progress = (p: Record<string, unknown>): StreamEvent =>
  ({ type: "run.progress", progress: p }) as StreamEvent;
const runFinished = (outcome: RunOutcome): StreamEvent => ({ type: "run.finished", outcome });

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/core-reducer");
  await loadPlugin(spec);
});

describe("reducer — run.progress (mid-run live readout)", () => {
  it("surfaces step / maxSteps / activity / tokens / cost while the run streams", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, runStarted("run_1"));
    s = reduce(
      s,
      progress({
        step: 2,
        maxSteps: 8,
        activity: "calling tool: ls -la",
        usage: { inputTokens: 1200, outputTokens: 80, costUsd: 0.0123 },
      }),
    );
    expect(s.run).toMatchObject({
      running: true,
      step: 2,
      totalSteps: 8,
      activity: "calling tool: ls -la",
      cost: "0.01",
    });
    expect(s.run.tokens.used).toBe("1280");
  });

  it("run.finished totals are authoritative over the last progress preview", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ step: 1, usage: { inputTokens: 10, outputTokens: 5 } }));
    s = reduce(
      s,
      runFinished({
        type: "completed",
        result: { steps: 3, usage: { inputTokens: 1200, outputTokens: 80, costUsd: 0.5 } },
      }),
    );
    expect(s.run.running).toBe(false);
    expect(s.run.step).toBe(3);
    expect(s.run.tokens.used).toBe("1280");
    expect(s.run.cost).toBe("0.50");
  });

  it("a progress event carrying only `activity` patches just that field", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ step: 4 }));
    s = reduce(s, progress({ activity: "thinking" }));
    expect(s.run.step).toBe(4); // unchanged by the activity-only event
    expect(s.run.activity).toBe("thinking");
  });
});
