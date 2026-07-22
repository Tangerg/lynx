// Per-handler contract tests — the ISOLATED state delta each built-in
// StreamEvent handler (handlers.ts: run.* / item.* / state.*) produces from a
// SINGLE event, plus what it deliberately leaves untouched (isolation).
//
// reducer.events.test.ts covers multi-event fold scenarios (how a stream
// builds bubbles/turns); this file pins each handler's minimal per-type effect
// and the branches those scenarios don't reach: the `plan` item on all three
// phases (started / delta / completed), the item.delta `plan` branch, deltas
// that target nothing, and segment.started's usage reset. Kept deliberately narrow
// — one event, one contract — so a regression names the exact handler.

import { beforeEach, describe, expect, it } from "vitest";
import type { Item, RunOutcome, StreamEvent } from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "@/plugins/sdk/types/agentView";

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
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const delta = (itemId: string, d: Record<string, unknown>): StreamEvent =>
  ({ type: "item.delta", itemId, delta: d }) as StreamEvent;
const runStarted = (id: string, sessionId: string): StreamEvent => ({
  type: "segment.started",
  run: { id, sessionId } as never,
});
const runFinished = (outcome: RunOutcome): StreamEvent => ({ type: "segment.finished", outcome });
const runProgress = (progress: Record<string, unknown>): StreamEvent =>
  ({ type: "segment.progress", progress }) as StreamEvent;
const snapshot = (state: Record<string, unknown>): StreamEvent =>
  ({ type: "state.snapshot", state }) as StreamEvent;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
});

describe("handler contract — run.*", () => {
  it("segment.started resets usage to zero + clears a prior error, without touching the stream", () => {
    // Seed a dirty state: accumulated usage, a stored error, and one open block.
    let s = reduce(INITIAL_VIEW_STATE, runStarted("r0", "s0"));
    s = reduce(
      s,
      runProgress({ usage: { inputTokens: 500, outputTokens: 200, cacheReadTokens: 40 } }),
    );
    s = reduce(
      s,
      runFinished({ type: "error", result: { error: { type: "provider_error", detail: "boom" } } }),
    );
    s = reduce(s, started(item({ id: "a", type: "agentMessage", content: [] })));
    expect(s.run.usage.inputTokens).toBe(500);
    expect(s.error).not.toBeNull();

    const out = reduce(s, runStarted("r1", "s1"));
    expect(out.run).toMatchObject({ running: true, runId: "r1", sessionId: "s1" });
    expect(out.run.usage).toEqual({ inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 });
    expect(out.error).toBeNull();
    // Isolation: a run boundary is not a turn boundary — the open bubble is kept
    // by reference (onRunStarted never maps the message list).
    expect(out.messages).toBe(s.messages);
    expect(out.timeline.at(-1)).toMatchObject({ kind: "run-start", runId: "r1" });
  });

  it("segment.progress patches only the fields present, leaving sibling readout untouched", () => {
    let s = reduce(INITIAL_VIEW_STATE, runStarted("r1", "s1"));
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
    expect(out.run.step).toBe(4);
    expect(out.run).toMatchObject({ activity: "reading", contextTokens: 4200 });
    expect(out.run.usage).toEqual({ inputTokens: 100, outputTokens: 5, cacheReadTokens: 0 });
  });

  it("segment.progress carrying a subagent envelope runId is an identity no-op", () => {
    const s = reduce(INITIAL_VIEW_STATE, runStarted("root", "s1"));
    const out = reduce(s, runProgress({ step: 9, activity: "child" }), "sub_run");
    expect(out).toBe(s);
  });

  it("segment.finished{completed} settles running without disturbing messages / plan / shared", () => {
    let s = reduce(INITIAL_VIEW_STATE, runStarted("r1", "s1"));
    s = reduce(s, started(item({ id: "a", type: "agentMessage", content: [] })));
    s = reduce(s, snapshot({ k: 1 }));
    s = reduce(
      s,
      started(
        item({ id: "p", type: "plan", steps: [{ id: "s1", title: "X", status: "running" }] }),
      ),
    );
    const out = reduce(s, runFinished({ type: "completed", result: { steps: 2 } }));
    expect(out.run.running).toBe(false);
    expect(out.messages).toEqual(s.messages);
    expect(out.plan).toEqual(s.plan);
    expect(out.shared).toEqual(s.shared);
  });
});

describe("handler contract — item.* plan branch (started / delta / completed)", () => {
  const planSteps = [
    { id: "s1", title: "Read the code", status: "running" },
    { id: "s2", title: "Write the fix", status: "pending" },
    { id: "s3", title: "Run tests", status: "completed" },
  ];
  // mapPlan: PlanStep{id,title,status} → PlanItem{id:i+1, pid, text:title, status}
  // with pending/failed→todo, running→doing, completed→done.
  const expectedPlan = [
    { id: 1, pid: "s1", status: "doing", text: "Read the code" },
    { id: 2, pid: "s2", status: "todo", text: "Write the fix" },
    { id: 3, pid: "s3", status: "done", text: "Run tests" },
  ];

  it("item.started{plan} maps wire steps into state.plan (not the message stream)", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      started(item({ id: "p1", type: "plan", steps: planSteps })),
    );
    expect(s.plan).toEqual(expectedPlan);
    expect(s.messages).toHaveLength(0);
  });

  it("item.delta{type:'plan'} replaces the plan wholesale (the mid-run update branch)", () => {
    let s = reduce(INITIAL_VIEW_STATE, started(item({ id: "p1", type: "plan", steps: planSteps })));
    s = reduce(
      s,
      delta("p1", {
        type: "plan",
        steps: [{ id: "s1", title: "Read the code", status: "completed" }],
      }),
    );
    expect(s.plan).toEqual([{ id: 1, pid: "s1", status: "done", text: "Read the code" }]);
  });

  it("item.completed{plan} settles the final plan", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      completed(item({ id: "p1", type: "plan", status: "complete", steps: planSteps })),
    );
    expect(s.plan).toEqual(expectedPlan);
  });
});

describe("handler contract — item.delta targeting", () => {
  it("a content delta for an unknown itemId touches nothing (no ghost block)", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      started(item({ id: "real", type: "agentMessage", content: [] })),
    );
    const out = reduce(s, delta("ghost", { type: "content", text: "leak?" }));
    expect(out.messages).toEqual(s.messages);
  });

  it("a toolOutput delta for an unknown itemId is a no-op on tools + stream", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      started(item({ id: "real", type: "agentMessage", content: [] })),
    );
    const out = reduce(s, delta("ghost", { type: "toolOutput", text: "x" }));
    expect(out.toolCalls).toEqual(s.toolCalls);
    expect(out.messages).toEqual(s.messages);
  });
});

describe("handler contract — state.*", () => {
  it("state.snapshot replaces shared wholesale, isolating run + stream", () => {
    let s = reduce(INITIAL_VIEW_STATE, runStarted("r1", "s1"));
    s = reduce(s, snapshot({ a: 1 }));
    const out = reduce(s, snapshot({ b: 2 }));
    expect(out.shared).toEqual({ b: 2 });
    expect(out.run).toBe(s.run);
    expect(out.messages).toBe(s.messages);
  });
});
