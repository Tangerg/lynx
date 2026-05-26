// Reasoning-style event semantics for the core-reducer plugin —
// TOOL_CALL_RESULT + REASONING_MESSAGE_* + THINKING_TEXT_MESSAGE_*.
// All three feed text/result data into the most recent assistant
// message, so they share the `withAssistantMessage` fixture.

import type { BaseEvent } from "@ag-ui/core";
import type { AgentViewState, Message } from "@/protocol/agui/viewState";
import { EventType } from "@ag-ui/core";
import { beforeEach, describe, expect, it } from "vitest";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "@/protocol/agui/reducer";
import { INITIAL_VIEW_STATE } from "@/protocol/agui/viewState";

const ev = <T extends BaseEvent>(e: T): BaseEvent => e;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/core-reducer");
  await loadPlugin(spec);
});

function withAssistantMessage(id = "m1"): AgentViewState {
  return reduce(
    INITIAL_VIEW_STATE,
    ev({
      type: EventType.TEXT_MESSAGE_START,
      messageId: id,
      role: "assistant",
    }),
  );
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
    s = reduce(
      s,
      ev({
        type: EventType.TOOL_CALL_START,
        toolCallId: "t1",
        toolCallName: "read_file",
        parentMessageId: "m1",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.TOOL_CALL_RESULT,
        messageId: "m1",
        toolCallId: "t1",
        content: "file contents here",
      }),
    );
    expect(s.toolCalls.t1.result).toBe("file contents here");
  });

  it("no-op when toolCallId is unknown (defensive)", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.TOOL_CALL_RESULT,
        messageId: "m1",
        toolCallId: "ghost",
        content: "x",
      }),
    );
    expect(s.toolCalls).toEqual({});
  });
});

// ---------------------------------------------------------------------------
// REASONING_MESSAGE_*
// ---------------------------------------------------------------------------

describe("core-reducer — REASONING_MESSAGE_*", () => {
  it("sTART appends a streaming reasoning block to the latest assistant message", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.REASONING_MESSAGE_START,
        messageId: "r1",
      }),
    );
    expect(lastBlocks(s, "m1")).toEqual([
      { kind: "reasoning", reasoningId: "r1", text: "", status: "running" },
    ]);
  });

  it("cONTENT appends delta text to the matching reasoning block", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.REASONING_MESSAGE_START,
        messageId: "r1",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.REASONING_MESSAGE_CONTENT,
        messageId: "r1",
        delta: "first ",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.REASONING_MESSAGE_CONTENT,
        messageId: "r1",
        delta: "thought",
      }),
    );
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", text: "first thought", status: "running" });
  });

  it("eND flips streaming off on the matching block", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.REASONING_MESSAGE_START, messageId: "r1" }));
    s = reduce(s, ev({ type: EventType.REASONING_MESSAGE_END, messageId: "r1" }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", status: "complete" });
  });

  it("sTART is a no-op when there's no assistant message to attach to", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.REASONING_MESSAGE_START,
        messageId: "r1",
      }),
    );
    expect(s.messages).toEqual([]);
  });

  it("cHUNK materializes the reasoning block on first chunk and appends deltas", () => {
    let s = withAssistantMessage();
    s = reduce(
      s,
      ev({
        type: EventType.REASONING_MESSAGE_CHUNK,
        messageId: "r1",
        delta: "alpha ",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.REASONING_MESSAGE_CHUNK,
        messageId: "r1",
        delta: "beta",
      }),
    );
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({
      kind: "reasoning",
      reasoningId: "r1",
      text: "alpha beta",
      status: "running",
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
  it("sTART opens a new reasoning block with a synthetic id", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_START }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", text: "", status: "running" });
    expect((block as { reasoningId: string }).reasoningId).toMatch(/^thinking:/);
  });

  it("cONTENT appends delta to the currently-open thinking block", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_START }));
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_CONTENT, delta: "hmm " }));
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_CONTENT, delta: "yes" }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", text: "hmm yes", status: "running" });
  });

  it("eND flips streaming off on the active thinking block", () => {
    let s = withAssistantMessage();
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_START }));
    s = reduce(s, ev({ type: EventType.THINKING_TEXT_MESSAGE_END }));
    const block = lastBlocks(s, "m1")[0];
    expect(block).toMatchObject({ kind: "reasoning", status: "complete" });
  });

  it("cONTENT/END with no open thinking block is a no-op", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.THINKING_TEXT_MESSAGE_CONTENT,
        delta: "abandoned",
      }),
    );
    expect(s).toBe(INITIAL_VIEW_STATE);
  });

  it("sTART is a no-op without an assistant message to attach to", () => {
    const s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.THINKING_TEXT_MESSAGE_START,
      }),
    );
    expect(s.messages).toEqual([]);
  });
});
