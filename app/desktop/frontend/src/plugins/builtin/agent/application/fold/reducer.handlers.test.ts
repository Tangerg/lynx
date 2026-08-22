// Per-handler contract tests — the ISOLATED state delta each built-in
// StreamEvent handler (handlers.ts: segment.* / item.* / plan.*) produces from a
// SINGLE event, plus what it deliberately leaves untouched (isolation).
//
// reducer.events.test.ts covers multi-event fold scenarios (how a stream
// builds bubbles/turns); this file pins each handler's minimal per-type effect
// and the branches those scenarios don't reach: deltas that target nothing, and
// segment.started's usage reset. Kept deliberately narrow — one event, one
// contract — so a regression names the exact handler.

import { beforeEach, describe, expect, it } from "vitest";
import type { AgentItem as Item, AgentStreamEvent as StreamEvent } from "@/plugins/sdk";
import { foldTestEvent as reduce, runFinished } from "./reducer.fixtures";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { selectCurrentRootRun, selectVisibleProblem } from "../view/runTree";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

// Terse builders (mirror reducer.events.test.ts). Items are partial — only the
// fields the fold reads matter; the cast keeps the wire shape from bloating.
function item(partial: Record<string, unknown>): Item {
  return {
    runId: "run_1",
    status: "running",
    createdAt: "2026-06-03T00:00:00Z",
    ...partial,
  } as Item;
}
const started = (i: Item): StreamEvent => ({ type: "item.started", item: i });
const delta = (itemId: string, d: Record<string, unknown>): StreamEvent =>
  ({ type: "item.delta", itemId, delta: d }) as StreamEvent;
const runStarted = (id: string, sessionId: string): StreamEvent => ({
  type: "segment.started",
  run: { id, sessionId } as never,
});
const runProgress = (progress: Record<string, unknown>): StreamEvent =>
  ({ type: "segment.progress", progress }) as StreamEvent;
const snapshot = (revision: number): StreamEvent => ({
  type: "plan.updated",
  plan: { revision, steps: [] },
});

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
});

describe("handler contract — run.*", () => {
  it("segment.started resets usage to zero + clears a prior error, without touching the stream", () => {
    // Seed a dirty state: accumulated usage, a stored error, and one open block.
    let s = reduce(EMPTY_AGENT_SESSION_VIEW, runStarted("r0", "s0"));
    s = reduce(
      s,
      runProgress({ usage: { inputTokens: 500, outputTokens: 200, cacheReadTokens: 40 } }),
    );
    s = reduce(
      s,
      runFinished(
        { type: "failed", error: { code: "provider_error", message: "boom" } },
        {
          steps: 3,
          activeDurationMillis: 20,
          usage: { inputTokens: 500, outputTokens: 200, cacheReadTokens: 40 },
        },
      ),
    );
    s = reduce(s, started(item({ id: "a", type: "agentMessage", content: [] })));
    expect(s.runsById.r0?.metrics.usage.inputTokens).toBe(500);
    expect(selectVisibleProblem(s)).not.toBeNull();

    const out = reduce(s, runStarted("r1", "s1"));
    expect(selectCurrentRootRun(out)).toMatchObject({
      status: "running",
      id: "r1",
      sessionId: "s1",
      metrics: {
        usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
      },
    });
    expect(selectVisibleProblem(out)).toBeNull();
    // Isolation: a run boundary is not a turn boundary — the open bubble is kept
    // by reference (onRunStarted never maps the message list).
    expect(out.messages).toBe(s.messages);
    expect(out.timeline.at(-1)).toMatchObject({ kind: "run-start", runId: "r1" });
  });

  it("segment.progress patches only the fields present, leaving sibling readout untouched", () => {
    let s = reduce(EMPTY_AGENT_SESSION_VIEW, runStarted("r1", "s1"));
    s = reduce(
      s,
      runProgress({
        step: 3,
        activity: "reading",
        contextTokens: 4200,
        usage: { inputTokens: 100, outputTokens: 5, cacheReadTokens: 0 },
      }),
    );
    const out = reduce(s, runProgress({ step: 4 })); // step only
    expect(selectCurrentRootRun(out)?.progress).toEqual({
      step: 4,
      activity: "reading",
      contextTokens: 4200,
      usage: { inputTokens: 100, outputTokens: 5, cacheReadTokens: 0 },
    });
  });

  it("segment.progress carrying a subagent envelope runId is an identity no-op", () => {
    const s = reduce(EMPTY_AGENT_SESSION_VIEW, runStarted("root", "s1"));
    const out = reduce(s, runProgress({ step: 9, activity: "child" }), "sub_run");
    expect(out).toBe(s);
  });

  it("segment.finished{completed} settles running without disturbing messages / shared", () => {
    let s = reduce(EMPTY_AGENT_SESSION_VIEW, runStarted("r1", "s1"));
    s = reduce(s, started(item({ id: "a", type: "agentMessage", content: [] })));
    s = reduce(s, snapshot(1));
    const out = reduce(
      s,
      runFinished({ type: "completed" }, { steps: 2, activeDurationMillis: 0 }),
    );
    expect(out.runsById.r1?.status).toBe("finished");
    expect(out.messages).toEqual(s.messages);
    expect(out.shared).toEqual(s.shared);
  });
});

describe("handler contract — item.delta targeting", () => {
  it("a content delta for an unknown itemId touches nothing (no ghost block)", () => {
    const s = reduce(
      EMPTY_AGENT_SESSION_VIEW,
      started(item({ id: "real", type: "agentMessage", content: [] })),
    );
    const out = reduce(s, delta("ghost", { type: "content", text: "leak?" }));
    expect(out.messages).toEqual(s.messages);
  });

  it("a toolOutput delta for an unknown itemId is a no-op on tools + stream", () => {
    const s = reduce(
      EMPTY_AGENT_SESSION_VIEW,
      started(item({ id: "real", type: "agentMessage", content: [] })),
    );
    const out = reduce(s, delta("ghost", { type: "toolOutput", text: "x" }));
    expect(out.toolCalls).toEqual(s.toolCalls);
    expect(out.messages).toEqual(s.messages);
  });
});

describe("handler contract — plan.*", () => {
  it("plan.updated replaces the Plan wholesale, isolating run + stream", () => {
    let s = reduce(EMPTY_AGENT_SESSION_VIEW, runStarted("r1", "s1"));
    s = reduce(s, snapshot(1));
    const out = reduce(s, snapshot(2));
    expect(out.plan).toMatchObject({ revision: 2 });
    expect(out.runsById).toBe(s.runsById);
    expect(out.messages).toBe(s.messages);
  });
});
