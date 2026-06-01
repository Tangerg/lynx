// Reducer — built-in AG-UI event behaviour. Covers RUN_* + STEP_* +
// TEXT_MESSAGE_* + TOOL_CALL_* + the fused CHUNK variants. CUSTOM-
// event dispatch lives next door in reducer.custom.test.ts;
// accumulator-shape tests (timeline / shared state / messages
// snapshot) in reducer.aggregates.test.ts.

import type { BaseEvent } from "@ag-ui/core";
import type { AgentViewState } from "./viewState";
import { EventType } from "@ag-ui/core";
import { beforeEach, describe, expect, it } from "vitest";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "./viewState";

// Cast helper — every event we craft is a single discriminated variant; the
// reducer is happy with `BaseEvent` typing.
const ev = <T extends BaseEvent>(e: T): BaseEvent => e;

// AG-UI protocol semantics live in the `lyra.builtin.core-reducer`
// plugin, not the reducer dispatcher.
beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/core-reducer");
  await loadPlugin(spec);
});

describe("reducer — built-in events", () => {
  it("rUN_STARTED flips running + records ids", () => {
    const next = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.RUN_STARTED,
        threadId: "t",
        runId: "r",
      }),
    );
    expect(next.run.running).toBe(true);
    expect(next.run.threadId).toBe("t");
    expect(next.run.runId).toBe("r");
  });

  it("rUN_FINISHED flips running off", () => {
    let s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.RUN_STARTED,
        threadId: "t",
        runId: "r",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.RUN_FINISHED,
        threadId: "t",
        runId: "r",
      }),
    );
    expect(s.run.running).toBe(false);
  });

  it("tEXT_MESSAGE_* builds an assistant message with one streaming text block", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant" }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_CONTENT, messageId: "m1", delta: "hi " }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_CONTENT, messageId: "m1", delta: "there" }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_END, messageId: "m1" }));

    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]!.role).toBe("assistant");
    expect(s.messages[0]!.blocks).toEqual([{ kind: "text", text: "hi there", status: "complete" }]);
  });

  it("tOOL_CALL_* attaches a tool block to the parent message", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant" }));
    s = reduce(
      s,
      ev({
        type: EventType.TOOL_CALL_START,
        toolCallId: "t1",
        toolCallName: "bash",
        parentMessageId: "m1",
      }),
    );
    s = reduce(s, ev({ type: EventType.TOOL_CALL_ARGS, toolCallId: "t1", delta: "pnpm test" }));
    s = reduce(s, ev({ type: EventType.TOOL_CALL_END, toolCallId: "t1" }));

    expect(s.toolCalls.t1).toMatchObject({ fn: "bash", args: "pnpm test", status: "ok" });
    expect(s.messages[0]!.blocks).toEqual([{ kind: "tool", toolCallId: "t1" }]);
  });
});

describe("reducer — run error / step finished", () => {
  it("rUN_ERROR stores message + flips running off; RUN_STARTED clears it", () => {
    let s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.RUN_STARTED,
        threadId: "t",
        runId: "r",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.RUN_ERROR,
        message: "boom",
        code: "E_TIMEOUT",
      }),
    );
    expect(s.error).toEqual({ message: "boom", code: "E_TIMEOUT" });
    expect(s.run.running).toBe(false);

    s = reduce(
      s,
      ev({
        type: EventType.RUN_STARTED,
        threadId: "t",
        runId: "r2",
      }),
    );
    expect(s.error).toBeNull();
  });

  it("sTEP_FINISHED bumps step counter and clears matching activity", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, ev({ type: EventType.STEP_STARTED, stepName: "analyse" }));
    expect(s.run.activity).toBe("analyse");
    s = reduce(s, ev({ type: EventType.STEP_FINISHED, stepName: "analyse" }));
    expect(s.run.step).toBe(1);
    expect(s.run.activity).toBe("");
  });
});

describe("reducer — chunk variants", () => {
  it("tEXT_MESSAGE_CHUNK materializes message on first chunk and appends deltas", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(
      s,
      ev({
        type: EventType.TEXT_MESSAGE_CHUNK,
        messageId: "m1",
        role: "assistant",
        delta: "hi ",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.TEXT_MESSAGE_CHUNK,
        messageId: "m1",
        delta: "world",
      }),
    );
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]!.blocks).toEqual([{ kind: "text", text: "hi world", status: "running" }]);
  });

  it("tOOL_CALL_CHUNK materializes tool on first chunk; later chunks fill the name", () => {
    let s = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.TEXT_MESSAGE_START,
        messageId: "m1",
        role: "assistant",
      }),
    );
    s = reduce(
      s,
      ev({
        type: EventType.TOOL_CALL_CHUNK,
        toolCallId: "t1",
        parentMessageId: "m1",
        delta: '{"path":',
      }),
    );
    expect(s.toolCalls.t1!.fn).toBe("");
    expect(s.toolCalls.t1!.args).toBe('{"path":');
    s = reduce(
      s,
      ev({
        type: EventType.TOOL_CALL_CHUNK,
        toolCallId: "t1",
        toolCallName: "read_file",
        delta: '"x"}',
      }),
    );
    expect(s.toolCalls.t1!.fn).toBe("read_file");
    expect(s.toolCalls.t1!.args).toBe('{"path":"x"}');
  });
});
