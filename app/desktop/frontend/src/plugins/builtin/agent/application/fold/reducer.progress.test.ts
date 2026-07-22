// segment.progress is the ephemeral mid-run readout (step / usage / cost / activity);
// segment.finished.result is the authoritative landing (API.md §5.2). The reducer
// must surface progress live AND let the finished totals win.
import { beforeEach, describe, expect, it } from "vitest";
import type { RunOutcome, StreamEvent } from "@/rpc";
import type { AgentViewState } from "@/plugins/sdk/types/agentView";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "@/plugins/sdk/types/agentView";

const runStarted = (id: string): StreamEvent => ({
  type: "segment.started",
  run: { id, sessionId: "ses_1" } as never,
});
const progress = (p: Record<string, unknown>): StreamEvent =>
  ({ type: "segment.progress", progress: p }) as StreamEvent;
const runFinished = (outcome: RunOutcome): StreamEvent => ({ type: "segment.finished", outcome });

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
});

describe("reducer — segment.progress (mid-run live readout)", () => {
  it("surfaces step / activity / tokens / cost while the run streams", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, runStarted("run_1"));
    s = reduce(
      s,
      progress({
        step: 2,
        activity: "calling tool: ls -la",
        usage: { inputTokens: 1200, outputTokens: 80, costUsd: 0.0123 },
      }),
    );
    expect(s.run).toMatchObject({
      running: true,
      step: 2,
      activity: "calling tool: ls -la",
      usage: { inputTokens: 1200, outputTokens: 80, cacheReadTokens: 0, costUsd: 0.0123 },
    });
  });

  it("segment.finished totals are authoritative over the last progress preview", () => {
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
    expect(s.run.usage).toEqual({
      inputTokens: 1200,
      outputTokens: 80,
      cacheReadTokens: 0,
      costUsd: 0.5,
    });
  });

  it("a progress event carrying only `activity` patches just that field", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ step: 4 }));
    s = reduce(s, progress({ activity: "thinking" }));
    expect(s.run.step).toBe(4); // unchanged by the activity-only event
    expect(s.run.activity).toBe("thinking");
  });

  it("surfaces contextTokens (the live context-window footprint driving compaction)", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, runStarted("run_1"));
    s = reduce(s, progress({ contextTokens: 45_000 }));
    expect(s.run.contextTokens).toBe(45_000);
  });
});
