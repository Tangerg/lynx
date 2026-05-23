// Per-event semantics tests for the core-reducer plugin.
//
// `protocol/agui/reducer.test.ts` covers the dispatcher + the 17 most
// common event types (RUN_* / TEXT_MESSAGE_* / TOOL_CALL_* / STATE_* /
// STEP_* / MESSAGES_SNAPSHOT). This file fills the gap with the 9
// remaining event types — reasoning / thinking / activity / tool
// result — plus a handful of edge cases worth pinning.
//
// Pattern matches the existing reducer.test.ts: load the core-reducer
// plugin once per spec, feed events through `reduce()`, assert state.

import { beforeEach, describe, expect, it } from "vitest";
import { EventType, type BaseEvent } from "@ag-ui/core";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "@/protocol/agui/reducer";
import {
  INITIAL_VIEW_STATE,
  type AgentViewState,
  type Message,
} from "@/protocol/agui/viewState";

const ev = <T extends BaseEvent>(e: T): BaseEvent => e;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/core-reducer");
  await loadPlugin(spec);
});

// Test-fixture helper — seed an assistant message that downstream events
// (reasoning / thinking / activity) can attach blocks to.
function withAssistantMessage(id = "m1"): AgentViewState {
  return reduce(INITIAL_VIEW_STATE, ev({
    type: EventType.TEXT_MESSAGE_START,
    messageId: id,
    role: "assistant",
  }));
}

function lastBlocks(state: AgentViewState, messageId: string): Message["blocks"] {
  const m = state.messages.find((x) => x.id === messageId);
  if (!m) throw new Error(`no message with id ${messageId}`);
  return m.blocks;
}

// ---------------------------------------------------------------------------
// TOOL_CALL_RESULT
// ---------------------------------------------------------------------------

describe("core-reducer — TOOL_CALL_RESULT", () => {
  it("stores ev.content on the matching tool call", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.TOOL_CALL_START,
      toolCallId: "t1", toolCallName: "read_file", parentMessageId: "m1",
    }));
    s = reduce(s, ev({
      type: EventType.TOOL_CALL_RESULT,
      messageId: "m1",
      toolCallId: "t1",
      content: "file contents here",
    }));
    expect(s.toolCalls.t1.result).toBe("file contents here");
  });

  it("no-op when toolCallId is unknown (defensive)", () => {
    const s = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.TOOL_CALL_RESULT,
      messageId: "m1",
      toolCallId: "ghost",
      content: "x",
    }));
    expect(s.toolCalls).toEqual({});
  });
});

// ---------------------------------------------------------------------------
// REASONING_MESSAGE_*
// ---------------------------------------------------------------------------

describe("core-reducer — REASONING_MESSAGE_*", () => {
  it("START appends a streaming reasoning block to the latest assistant message", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.REASONING_MESSAGE_START,
      messageId: "r1",
    }));
    expect(lastBlocks(s, "m1")).toEqual([
      { kind: "reasoning", reasoningId: "r1", text: "", streaming: true },
    ]);
  });

  it("CONTENT appends delta text to the matching reasoning block", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.REASONING_MESSAGE_START,
      messageId: "r1",
    }));
    s = reduce(s, ev({
      type: EventType.REASONING_MESSAGE_CONTENT,
      messageId: "r1",
      delta: "first ",
    }));
    s = reduce(s, ev({
      type: EventType.REASONING_MESSAGE_CONTENT,
      messageId: "r1",
      delta: "thought",
    }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", text: "first thought", streaming: true });
  });

  it("END flips streaming off on the matching block", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.REASONING_MESSAGE_START, messageId: "r1" }));
    s = reduce(s, ev({ type: EventType.REASONING_MESSAGE_END,   messageId: "r1" }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", streaming: false });
  });

  it("START is a no-op when there's no assistant message to attach to", () => {
    const s = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.REASONING_MESSAGE_START,
      messageId: "r1",
    }));
    expect(s.messages).toEqual([]);
  });

  it("CHUNK materializes the reasoning block on first chunk and appends deltas", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.REASONING_MESSAGE_CHUNK,
      messageId: "r1",
      delta: "alpha ",
    }));
    s = reduce(s, ev({
      type: EventType.REASONING_MESSAGE_CHUNK,
      messageId: "r1",
      delta: "beta",
    }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({
      kind: "reasoning", reasoningId: "r1", text: "alpha beta", streaming: true,
    });
  });
});

// ---------------------------------------------------------------------------
// THINKING_TEXT_MESSAGE_*
//
// Translate Claude-3.7 style extended-thinking events into our reasoning
// block model. Events don't carry messageId — they're implicitly scoped
// to the currently-open thinking block.
// ---------------------------------------------------------------------------

describe("core-reducer — THINKING_TEXT_MESSAGE_*", () => {
  it("START opens a new reasoning block with a synthetic id", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_START }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", text: "", streaming: true });
    expect((block as { reasoningId: string }).reasoningId).toMatch(/^thinking:/);
  });

  it("CONTENT appends delta to the currently-open thinking block", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_START }));
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_CONTENT, delta: "hmm " }));
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_CONTENT, delta: "yes" }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", text: "hmm yes", streaming: true });
  });

  it("END flips streaming off on the active thinking block", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_START }));
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_END }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", streaming: false });
  });

  it("CONTENT/END with no open thinking block is a no-op", () => {
    const s = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.THINKING_TEXT_MESSAGE_CONTENT,
      delta: "abandoned",
    }));
    expect(s).toBe(INITIAL_VIEW_STATE);
  });

  it("START is a no-op without an assistant message to attach to", () => {
    const s = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.THINKING_TEXT_MESSAGE_START,
    }));
    expect(s.messages).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// ACTIVITY_SNAPSHOT / ACTIVITY_DELTA
// ---------------------------------------------------------------------------

describe("core-reducer — ACTIVITY_SNAPSHOT", () => {
  it("writes content onto message.activities under the activityType key", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1",
      activityType: "websearch",
      content: { query: "react query", hits: 12 },
    }));
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.websearch).toEqual({ query: "react query", hits: 12 });
  });

  it("merges with prior content by default (replace=false)", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1",
      activityType: "websearch",
      content: { query: "q1", hits: 5 },
    }));
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1",
      activityType: "websearch",
      content: { hits: 10, latencyMs: 200 },
    }));
    const m = s.messages.find((x) => x.id === "m1")!;
    // hits gets overwritten (10), query preserved (q1), latencyMs added.
    expect(m.activities?.websearch).toEqual({ query: "q1", hits: 10, latencyMs: 200 });
  });

  it("replaces wholesale when replace=true", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1",
      activityType: "websearch",
      content: { query: "q1", hits: 5 },
    }));
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1",
      activityType: "websearch",
      content: { totalMs: 400 },
      replace: true,
    } as BaseEvent));
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.websearch).toEqual({ totalMs: 400 });
  });

  it("scopes by (messageId, activityType) — different keys coexist", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1", activityType: "websearch", content: { hits: 1 },
    }));
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1", activityType: "exec", content: { exitCode: 0 },
    }));
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities).toEqual({
      websearch: { hits: 1 },
      exec: { exitCode: 0 },
    });
  });
});

describe("core-reducer — ACTIVITY_DELTA", () => {
  it("applies a JSON Patch to the existing activity content", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1", activityType: "exec",
      content: { stdout: "", stderr: "" },
    }));
    s = reduce(s, ev({
      type: EventType.ACTIVITY_DELTA,
      messageId: "m1", activityType: "exec",
      patch: [
        { op: "replace", path: "/stdout", value: "line one\n" },
        { op: "add", path: "/exitCode", value: 0 },
      ],
    } as BaseEvent));
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.exec).toEqual({
      stdout: "line one\n", stderr: "", exitCode: 0,
    });
  });

  it("starts from {} when no prior content exists for the activity", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.ACTIVITY_DELTA,
      messageId: "m1", activityType: "exec",
      patch: [{ op: "add", path: "/started", value: true }],
    } as BaseEvent));
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.exec).toEqual({ started: true });
  });

  it("with a broken patch leaves prior content unchanged", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({
      type: EventType.ACTIVITY_SNAPSHOT,
      messageId: "m1", activityType: "exec",
      content: { stdout: "kept" },
    }));
    s = reduce(s, ev({
      type: EventType.ACTIVITY_DELTA,
      messageId: "m1", activityType: "exec",
      patch: [{ op: "remove", path: "/does/not/exist" }],
    } as BaseEvent));
    const m = s.messages.find((x) => x.id === "m1")!;
    expect(m.activities?.exec).toEqual({ stdout: "kept" });
  });
});
