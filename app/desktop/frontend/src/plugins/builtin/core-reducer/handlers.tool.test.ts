// Tool-call stream semantics for the core-reducer plugin. The headline
// case is defensive dedup: a repeated TOOL_CALL_START for the same
// toolCallId must NOT push a second `tool` block / timeline entry — the
// same bug class onTextStart guards against (duplicate render + doubled
// timeline). onToolChunk already skips its block append via `!existing`;
// this locks the same guarantee for the explicit START path.

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
    ev({ type: EventType.TEXT_MESSAGE_START, messageId: id, role: "assistant" }),
  );
}

function blocksOf(state: AgentViewState, messageId: string): Message["blocks"] {
  const m = state.messages.find((x) => x.id === messageId);
  if (!m) throw new Error(`no message with id ${messageId}`);
  return m.blocks;
}

const startTool = (state: AgentViewState) =>
  reduce(
    state,
    ev({
      type: EventType.TOOL_CALL_START,
      toolCallId: "t1",
      toolCallName: "read_file",
      parentMessageId: "m1",
    }),
  );

describe("core-reducer — TOOL_CALL_START", () => {
  it("appends one tool block + one timeline entry on first start", () => {
    const s = startTool(withAssistantMessage());
    const toolBlocks = blocksOf(s, "m1").filter((b) => b.kind === "tool");
    expect(toolBlocks).toHaveLength(1);
    expect(s.timeline.filter((t) => t.kind === "tool-start" && t.refId === "t1")).toHaveLength(1);
    expect(s.toolCalls.t1?.fn).toBe("read_file");
  });

  it("is idempotent: a repeated start for the same toolCallId is a no-op", () => {
    let s = startTool(withAssistantMessage());
    const before = s;
    s = startTool(s);
    // No second block, no second timeline entry, identical reference back
    // (the guard returns state untouched — same as onTextStart).
    expect(s).toBe(before);
    expect(blocksOf(s, "m1").filter((b) => b.kind === "tool")).toHaveLength(1);
    expect(s.timeline.filter((t) => t.kind === "tool-start" && t.refId === "t1")).toHaveLength(1);
  });
});
