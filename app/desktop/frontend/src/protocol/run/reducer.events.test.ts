// Reducer — built-in v2 StreamEvent behaviour. Covers run.started /
// run.finished (completed / error / interrupt) + item.started / item.delta
// / item.completed folding into message bubbles + tool calls. `custom`
// dispatch lives in reducer.custom.test.ts; shared-state / plan accumulator
// tests in reducer.aggregates.test.ts.

import { beforeEach, describe, expect, it } from "vitest";
import type { Item, RunOutcome, StreamEvent } from "@/rpc";
import type { AgentViewState } from "./viewState";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "./viewState";

// Builders. Items are partial — only the fields the fold reads matter; the
// cast keeps the test terse without re-stating the full wire shape.
function item(partial: Record<string, unknown>): Item {
  return {
    runId: "run_1",
    status: "inProgress",
    createdAt: "2026-06-03T00:00:00Z",
    ...partial,
  } as Item;
}
const started = (i: Item): StreamEvent => ({ type: "item.started", item: i });
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const delta = (itemId: string, d: Record<string, unknown>): StreamEvent =>
  ({ type: "item.delta", itemId, delta: d }) as StreamEvent;
const runStarted = (id: string, sessionId: string): StreamEvent => ({
  type: "run.started",
  run: { id, sessionId } as never,
});
// A resume/edit continuation Run — RunRef carries parentRunId (API.md §4.x).
const runResumed = (id: string, sessionId: string, parentRunId: string): StreamEvent => ({
  type: "run.started",
  run: { id, sessionId, parentRunId } as never,
});
const runFinished = (outcome: RunOutcome): StreamEvent => ({ type: "run.finished", outcome });

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/core-reducer");
  await loadPlugin(spec);
});

describe("reducer — run lifecycle", () => {
  it("run.started flips running + records ids; run.finished flips off", () => {
    let s = reduce(INITIAL_VIEW_STATE, runStarted("run_1", "ses_1"));
    expect(s.run).toMatchObject({ running: true, runId: "run_1", sessionId: "ses_1" });
    s = reduce(s, runFinished({ type: "completed", result: { steps: 2 } }));
    expect(s.run.running).toBe(false);
    expect(s.run.step).toBe(2);
  });

  it("run.finished{error} stores the error; a fresh run.started clears it", () => {
    let s = reduce(INITIAL_VIEW_STATE, runStarted("run_1", "ses_1"));
    s = reduce(
      s,
      runFinished({ type: "error", result: { error: { type: "provider_error", detail: "boom" } } }),
    );
    expect(s.error).toEqual({ message: "boom", code: "provider_error" });
    expect(s.run.running).toBe(false);
    s = reduce(s, runStarted("run_2", "ses_1"));
    expect(s.error).toBeNull();
  });
});

describe("reducer — item fold", () => {
  it("agentMessage start + content deltas + completed build one streaming text block", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, started(item({ id: "item_1", type: "agentMessage", content: [] })));
    s = reduce(s, delta("item_1", { type: "content", text: "hi " }));
    s = reduce(s, delta("item_1", { type: "content", text: "there" }));
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]!.blocks).toEqual([
      { kind: "text", itemId: "item_1", text: "hi there", status: "running" },
    ]);
    s = reduce(
      s,
      completed(
        item({
          id: "item_1",
          type: "agentMessage",
          status: "completed",
          content: [{ type: "text", text: "hi there" }],
        }),
      ),
    );
    expect(s.messages[0]!.blocks[0]).toMatchObject({ status: "complete", text: "hi there" });
  });

  it("agentMessage start with no content shell still streams (content arrives via deltas)", () => {
    // The real runtime's item.started shell carries NO `content` field — it
    // streams in via item.delta and only lands whole on item.completed. The
    // fold must fold that to an empty running text block, not crash.
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, started(item({ id: "item_1", type: "agentMessage" }))); // no `content`
    expect(s.messages[0]!.blocks).toEqual([
      { kind: "text", itemId: "item_1", text: "", status: "running" },
    ]);
    s = reduce(s, delta("item_1", { type: "content", text: "streamed" }));
    expect(s.messages[0]!.blocks[0]).toMatchObject({ text: "streamed", status: "running" });
  });

  it("toolCall folds into a tool block + toolCalls entry; args + stdout accumulate", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(
      s,
      started(
        item({
          id: "t1",
          type: "toolCall",
          tool: { kind: "commandExecution", command: ["pnpm", "test"] },
        }),
      ),
    );
    s = reduce(s, delta("t1", { type: "toolArguments", argumentsTextDelta: '{"x":' }));
    s = reduce(s, delta("t1", { type: "toolArguments", argumentsTextDelta: "1}" }));
    // commandExecution stdout streams via toolOutput (no item.output field).
    s = reduce(s, delta("t1", { type: "toolOutput", text: "ok" }));
    expect(s.messages[0]!.blocks).toEqual([{ kind: "tool", toolCallId: "t1" }]);
    expect(s.toolCalls.t1).toMatchObject({ fn: "pnpm test", args: '{"x":1}', status: "running" });
    s = reduce(
      s,
      completed(
        item({
          id: "t1",
          type: "toolCall",
          status: "completed",
          tool: { kind: "commandExecution", command: ["pnpm", "test"], exitCode: 0 },
        }),
      ),
    );
    // The streamed stdout survives completion (writeToolCall keeps prev.result).
    expect(s.toolCalls.t1).toMatchObject({ status: "ok", result: "ok" });
  });

  it("contiguous assistant items fold into one turn bubble; a userMessage opens a new one", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, started(item({ id: "r1", type: "reasoning", text: "think" })));
    s = reduce(s, started(item({ id: "a1", type: "agentMessage", content: [] })));
    expect(s.messages).toHaveLength(1); // reasoning + text share the turn
    expect(s.messages[0]!.blocks.map((b) => b.kind)).toEqual(["reasoning", "text"]);
    s = reduce(
      s,
      started(item({ id: "u1", type: "userMessage", content: [{ type: "text", text: "next" }] })),
    );
    expect(s.messages).toHaveLength(2);
    expect(s.messages[1]!.role).toBe("user");
  });

  it("a streamed userMessage reconciles the optimistic placeholder (no duplicate)", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    // send() renders the user's bubble optimistically with a local-* id.
    s = reduce(
      s,
      completed(
        item({
          id: "local-1",
          type: "userMessage",
          status: "completed",
          content: [{ type: "text", text: "hi" }],
        }),
      ),
    );
    expect(s.messages).toHaveLength(1);
    // The runtime then streams the real userMessage Item with its own server
    // id (started + completed). It must upgrade the placeholder in place, not
    // append a second bubble.
    s = reduce(
      s,
      started(
        item({ id: "item_real", type: "userMessage", content: [{ type: "text", text: "hi" }] }),
      ),
    );
    s = reduce(
      s,
      completed(
        item({
          id: "item_real",
          type: "userMessage",
          status: "completed",
          content: [{ type: "text", text: "hi" }],
        }),
      ),
    );
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]!.id).toBe("item_real");
    expect(s.messages[0]!.role).toBe("user");
  });

  it("body-less item.started shells fold without crashing (tool / question / plan)", () => {
    // The runtime's started shell may carry only ItemBase fields — the body
    // (tool / question / steps) streams in later or lands whole on completed.
    // Each must fold to an empty block, not throw (which the reducer's
    // try/catch would swallow, silently dropping the block forever).
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, started(item({ id: "t1", type: "toolCall" }))); // no `tool`
    expect(s.toolCalls.t1).toMatchObject({ fn: "tool", status: "running" });
    s = reduce(s, started(item({ id: "q1", type: "question" }))); // no `question`
    expect(s.messages.flatMap((m) => m.blocks).find((b) => b.kind === "question")).toMatchObject({
      kind: "question",
      itemId: "q1",
      questions: [],
    });
    s = reduce(s, started(item({ id: "p1", type: "plan" }))); // no `steps`
    expect(s.plan).toEqual([]);
  });

  it("item.completed{status:inProgress} is a lost item — settles incomplete, not a forever spinner", () => {
    // History hydration (items.list) of a run lost to a crash/restart replays
    // its last item as item.completed but with status still inProgress (a known
    // backend reconciliation gap). The fold must coerce that to incomplete — a "running" block here
    // would spin forever (no live stream will ever complete it).
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(
      s,
      completed(
        item({
          id: "a1",
          type: "agentMessage",
          status: "inProgress", // contradictory on a completed event — coerced
          content: [{ type: "text", text: "half a thoug" }],
        }),
      ),
    );
    expect(s.messages[0]!.blocks[0]).toMatchObject({ status: "incomplete", text: "half a thoug" });
  });

  it("item.completed{status:incomplete} settles the block as incomplete, not complete", () => {
    // A canceled/interrupted run settles its agentMessage as `incomplete`
    // (API.md §4.3); the fold must preserve that, not stamp "complete".
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, started(item({ id: "a1", type: "agentMessage", content: [] })));
    s = reduce(s, delta("a1", { type: "content", text: "partial" }));
    s = reduce(
      s,
      completed(
        item({
          id: "a1",
          type: "agentMessage",
          status: "incomplete",
          content: [{ type: "text", text: "partial" }],
        }),
      ),
    );
    expect(s.messages[0]!.blocks[0]).toMatchObject({ status: "incomplete", text: "partial" });
  });

  it("a failed toolCall projects its error detail", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(
      s,
      started(
        item({ id: "t1", type: "toolCall", tool: { kind: "commandExecution", command: ["bad"] } }),
      ),
    );
    s = reduce(
      s,
      completed(
        item({
          id: "t1",
          type: "toolCall",
          status: "incomplete",
          tool: { kind: "commandExecution", command: ["bad"] },
          error: { type: "tool_failed", detail: "boom" },
        }),
      ),
    );
    expect(s.toolCalls.t1).toMatchObject({ status: "err", error: "boom" });
  });

  it("a HITL-denied toolCall projects `denied`, not `err`", () => {
    // Backend settles a declined tool as incomplete + error.type
    // "denied_by_user" (API.md §8.1). That's a user decision — the fold maps
    // it to a neutral `denied` state, distinct from a real failure.
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(
      s,
      started(
        item({ id: "t1", type: "toolCall", tool: { kind: "commandExecution", command: ["bash"] } }),
      ),
    );
    s = reduce(
      s,
      completed(
        item({
          id: "t1",
          type: "toolCall",
          status: "incomplete",
          tool: { kind: "commandExecution", command: ["bash"] },
          error: { type: "denied_by_user", detail: "tool call denied by user" },
        }),
      ),
    );
    expect(s.toolCalls.t1).toMatchObject({ status: "denied" });
    // And the tool-end timeline entry records the decision, not "ok"/"err".
    expect(s.timeline.findLast((e) => e.kind === "tool-end")).toMatchObject({ status: "declined" });
  });
});

describe("reducer — HITL interrupt", () => {
  it("run.finished{interrupt} materializes an approval block + open interrupt", () => {
    let s = reduce(INITIAL_VIEW_STATE, runStarted("run_1", "ses_1"));
    s = reduce(
      s,
      started(
        item({
          id: "tool_1",
          type: "toolCall",
          tool: { kind: "commandExecution", command: ["rm", "-rf", "x"] },
        }),
      ),
    );
    s = reduce(
      s,
      runFinished({
        type: "interrupt",
        interrupts: [
          {
            itemId: "tool_1" as never,
            kind: "approval",
            payload: { tool: { kind: "commandExecution", command: ["rm", "-rf", "x"] } },
          },
        ],
      }),
    );
    const block = s.messages.flatMap((m) => m.blocks).find((b) => b.kind === "approval");
    expect(block).toMatchObject({
      kind: "approval",
      status: "requires-action",
      itemId: "tool_1",
      parentRunId: "run_1",
      command: "rm -rf x", // derived from payload.tool (commandExecution)
    });
    expect(s.openInterrupts).toHaveLength(1);
    expect(s.openInterrupts[0]!.parentRunId).toBe("run_1");
  });

  it("approval payload carries a ToolInvocation: command → cmd line, generic tool → editable args", () => {
    let s = reduce(INITIAL_VIEW_STATE, runStarted("run_1", "ses_1"));
    s = reduce(
      s,
      started(
        item({
          id: "t1",
          type: "toolCall",
          tool: { kind: "tool", name: "fs.write", arguments: {} },
        }),
      ),
    );
    s = reduce(
      s,
      runFinished({
        type: "interrupt",
        interrupts: [
          {
            itemId: "t1" as never,
            kind: "approval",
            payload: {
              tool: { kind: "tool", name: "fs.write", arguments: { path: "/etc/hosts" } },
              risk: "high",
            },
          },
        ],
      }),
    );
    const block = s.messages.flatMap((m) => m.blocks).find((b) => b.kind === "approval");
    // Generic tool: arguments object becomes the editable `args`; no cmd line.
    expect(block).toMatchObject({
      kind: "approval",
      command: "",
      args: { path: "/etc/hosts" },
      risk: "high",
    });
  });

  it("resume run keeps the open turn: post-approval text appends to the same bubble", () => {
    // run_1: tool call → interrupt (approval). Tool block + approval land in
    // one assistant turn.
    let s = reduce(INITIAL_VIEW_STATE, runStarted("run_1", "ses_1"));
    s = reduce(
      s,
      started(
        item({
          id: "tool_1",
          type: "toolCall",
          tool: { kind: "commandExecution", command: ["rm", "x"] },
        }),
      ),
    );
    s = reduce(
      s,
      runFinished({
        type: "interrupt",
        interrupts: [
          {
            itemId: "tool_1" as never,
            kind: "approval",
            payload: { tool: { kind: "commandExecution", command: ["rm", "x"] } },
          },
        ],
      }),
    );
    expect(s.messages).toHaveLength(1);
    const turnId = s.messages[0]!.id;

    // Approve → resume Run (parentRunId set). Its agentMessage must fold into
    // the SAME bubble, not spawn a second avatar/name header.
    s = reduce(s, runResumed("run_2", "ses_1", "run_1"));
    s = reduce(s, started(item({ id: "msg_1", type: "agentMessage", content: [] })));
    s = reduce(s, delta("msg_1", { type: "content", text: "Deleted." }));

    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]!.id).toBe(turnId);
    expect(s.messages[0]!.blocks.map((b) => b.kind)).toEqual(["tool", "approval", "text"]);
  });
});
