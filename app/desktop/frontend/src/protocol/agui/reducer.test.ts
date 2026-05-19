import { beforeEach, describe, expect, it } from "vitest";
import { EventType, type BaseEvent } from "@ag-ui/core";
import { createHost } from "@/plugins/sdk/host";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { CUSTOM } from "./customEvents";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE, type AgentViewState } from "./viewState";
import {
  appendBlockToLatestAssistant,
  appendBlockToMessage,
} from "@/plugins/sdk/state";

// Cast helper — every event we craft is a single discriminated variant; the
// reducer is happy with `BaseEvent` typing.
const ev = <T extends BaseEvent>(e: T): BaseEvent => e;

// Phase 6 moved AG-UI protocol semantics out of the reducer into the
// `lyra.builtin.core-reducer` plugin. Every test in this file that fires
// a built-in event type (RUN_*, TEXT_MESSAGE_*, TOOL_CALL_*) — including
// the CUSTOM-fallback tests that seed state with TEXT_MESSAGE_START — has
// to load core-reducer first. Hoisting to the file's top level applies to
// every describe block below.
beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/core-reducer");
  await loadPlugin(spec);
});

describe("reducer — built-in events", () => {
  it("RUN_STARTED flips running + records ids", () => {
    const next = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.RUN_STARTED,
      threadId: "t",
      runId: "r",
    }));
    expect(next.run.running).toBe(true);
    expect(next.run.threadId).toBe("t");
    expect(next.run.runId).toBe("r");
  });

  it("RUN_FINISHED flips running off", () => {
    let s = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.RUN_STARTED, threadId: "t", runId: "r",
    }));
    s = reduce(s, ev({
      type: EventType.RUN_FINISHED, threadId: "t", runId: "r",
    }));
    expect(s.run.running).toBe(false);
  });

  it("TEXT_MESSAGE_* builds an assistant message with one streaming text block", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant" }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_CONTENT, messageId: "m1", delta: "hi " }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_CONTENT, messageId: "m1", delta: "there" }));
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_END, messageId: "m1" }));

    expect(s.messages).toHaveLength(1);
    expect(s.messages[0].role).toBe("assistant");
    expect(s.messages[0].blocks).toEqual([
      { kind: "text", text: "hi there", streaming: false },
    ]);
  });

  it("TOOL_CALL_* attaches a tool block to the parent message", () => {
    let s: AgentViewState = INITIAL_VIEW_STATE;
    s = reduce(s, ev({ type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant" }));
    s = reduce(s, ev({
      type: EventType.TOOL_CALL_START,
      toolCallId: "t1", toolCallName: "bash", parentMessageId: "m1",
    }));
    s = reduce(s, ev({ type: EventType.TOOL_CALL_ARGS, toolCallId: "t1", delta: "pnpm test" }));
    s = reduce(s, ev({ type: EventType.TOOL_CALL_END, toolCallId: "t1" }));

    expect(s.toolCalls["t1"]).toMatchObject({ fn: "bash", args: "pnpm test", status: "ok" });
    expect(s.messages[0].blocks).toEqual([{ kind: "tool", toolCallId: "t1" }]);
  });
});

describe("reducer — built-in CUSTOM events (via builtin plugin handlers)", () => {
  // Phase 5 moved `lyra.plan` / `lyra.telemetry` handling out of the reducer
  // and into individual plugins. The reducer alone no longer reacts to those
  // names — we load the builtin handler before each test.
  it("lyra.plan installs the plan once plan-handler is loaded", async () => {
    const { default: spec } = await import("@/plugins/builtin/plan-handler");
    await loadPlugin(spec);

    const next = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.CUSTOM,
      name: CUSTOM.PLAN,
      value: { items: [{ id: 1, pid: "T-1", status: "todo", text: "do x" }] },
    }));
    expect(next.plan).toHaveLength(1);
    expect(next.plan[0].text).toBe("do x");
  });

  it("lyra.telemetry patches the run state once telemetry-handler is loaded", async () => {
    const { default: spec } = await import("@/plugins/builtin/telemetry-handler");
    await loadPlugin(spec);

    const next = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.CUSTOM,
      name: CUSTOM.TELEMETRY,
      value: {
        step: 3, totalSteps: 7, activity: "scan",
        tokens: { used: "1k", total: "200k" },
        ctxPct: 12, cost: "0.10",
      },
    }));
    expect(next.run.step).toBe(3);
    expect(next.run.activity).toBe("scan");
    expect(next.run.tokens).toEqual({ used: "1k", total: "200k" });
  });
});

describe("reducer — plugin CUSTOM fallback", () => {
  it("unrecognized name with no registered handler is a no-op", () => {
    const next = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.CUSTOM,
      name: "unregistered.xyz",
      value: { whatever: true },
    }));
    expect(next).toEqual(INITIAL_VIEW_STATE);
  });

  it("routes to a plugin-registered handler", () => {
    // Seed: one assistant message so appendBlockToLatestAssistant has a target.
    const seeded = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant",
    }));

    const host = createHost("plug", []);
    host.agui.on<{ text: string }>("custom.banner", (value) =>
      appendBlockToLatestAssistant({ kind: "text", text: `banner: ${value.text}`, streaming: false }),
    );

    const next = reduce(seeded, ev({
      type: EventType.CUSTOM,
      name: "custom.banner",
      value: { text: "hi" },
    }));

    expect(next.messages[0].blocks).toEqual([
      { kind: "text", text: "banner: hi", streaming: false },
    ]);
  });

  it("a handler that throws gets isolated + logged to error store", () => {
    const host = createHost("plug", []);
    host.agui.on("custom.boom", () => { throw new Error("nope"); });

    const next = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.CUSTOM,
      name: "custom.boom",
      value: undefined,
    }));

    expect(next).toEqual(INITIAL_VIEW_STATE);
    const log = usePluginErrorStore.getState().log;
    expect(log).toHaveLength(1);
    expect(log[0]).toMatchObject({ plugin: "plug", source: "agui" });
  });

  it("a handler that returns void leaves state untouched", () => {
    const host = createHost("plug", []);
    host.agui.on("custom.metrics", () => { /* fire-and-forget side effect */ });

    const next = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.CUSTOM,
      name: "custom.metrics",
      value: { count: 1 },
    }));
    expect(next).toBe(INITIAL_VIEW_STATE);
  });

  it("handler can use appendBlockToMessage for explicit targeting", () => {
    const seeded = reduce(INITIAL_VIEW_STATE, ev({
      type: EventType.TEXT_MESSAGE_START, messageId: "m1", role: "assistant",
    }));

    const host = createHost("plug", []);
    host.agui.on<{ id: string }>("custom.tag", (v) =>
      appendBlockToMessage(v.id, { kind: "plan" }),
    );

    const next = reduce(seeded, ev({
      type: EventType.CUSTOM,
      name: "custom.tag",
      value: { id: "m1" },
    }));

    expect(next.messages[0].blocks).toEqual([{ kind: "plan" }]);
  });
});
