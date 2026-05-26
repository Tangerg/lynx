// Reducer — CUSTOM event dispatch. The dispatcher itself routes by
// `event.name`; built-in handlers (plan / telemetry / approval / …)
// register from their own plugins; plugin-defined handlers register
// via `host.agui.on(name, handler)`. Tests cover both flows plus the
// "unrecognised name" + "throwing handler isolation" edges.

import type { BaseEvent } from "@ag-ui/core";
import { EventType } from "@ag-ui/core";
import { beforeEach, describe, expect, it } from "vitest";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { createHost } from "@/plugins/sdk/host";
import { appendBlockToLatestAssistant, appendBlockToMessage } from "@/plugins/sdk/state";
import { CUSTOM } from "./customEvents";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "./viewState";

const ev = <T extends BaseEvent>(e: T): BaseEvent => e;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/core-reducer");
  await loadPlugin(spec);
});

describe("reducer — built-in CUSTOM events (via builtin plugin handlers)", () => {
  // `lyra.plan` / `lyra.telemetry` handling lives in individual plugins.
  // The reducer alone no longer reacts to those names — we load the
  // builtin handler before each test.
  it("lyra.plan installs the plan once plan-handler is loaded", async () => {
    const { planHandler: spec } = await import("@/plugins/builtin/agui-handlers");
    await loadPlugin(spec);

    const next = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.CUSTOM,
        name: CUSTOM.PLAN,
        value: { items: [{ id: 1, pid: "T-1", status: "todo", text: "do x" }] },
      }),
    );
    expect(next.plan).toHaveLength(1);
    expect(next.plan[0].text).toBe("do x");
  });

  it("lyra.telemetry patches the run state once telemetry-handler is loaded", async () => {
    const { telemetryHandler: spec } = await import("@/plugins/builtin/agui-handlers");
    await loadPlugin(spec);

    const next = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.CUSTOM,
        name: CUSTOM.TELEMETRY,
        value: {
          step: 3,
          totalSteps: 7,
          activity: "scan",
          tokens: { used: "1k", total: "200k" },
          ctxPct: 12,
          cost: "0.10",
        },
      }),
    );
    expect(next.run.step).toBe(3);
    expect(next.run.activity).toBe("scan");
    expect(next.run.tokens).toEqual({ used: "1k", total: "200k" });
  });
});

describe("reducer — plugin CUSTOM fallback", () => {
  it("unrecognized name with no registered handler is a no-op", () => {
    const next = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.CUSTOM,
        name: "unregistered.xyz",
        value: { whatever: true },
      }),
    );
    expect(next).toEqual(INITIAL_VIEW_STATE);
  });

  it("routes to a plugin-registered handler", () => {
    // Seed: one assistant message so appendBlockToLatestAssistant has a target.
    const seeded = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.TEXT_MESSAGE_START,
        messageId: "m1",
        role: "assistant",
      }),
    );

    const host = createHost("plug", []);
    host.agui.on<{ text: string }>("custom.banner", (value) =>
      appendBlockToLatestAssistant({
        kind: "text",
        text: `banner: ${value.text}`,
        status: "complete",
      }),
    );

    const next = reduce(
      seeded,
      ev({
        type: EventType.CUSTOM,
        name: "custom.banner",
        value: { text: "hi" },
      }),
    );

    expect(next.messages[0].blocks).toEqual([
      { kind: "text", text: "banner: hi", status: "complete" },
    ]);
  });

  it("a handler that throws gets isolated + logged to error store", () => {
    const host = createHost("plug", []);
    host.agui.on("custom.boom", () => {
      throw new Error("nope");
    });

    const next = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.CUSTOM,
        name: "custom.boom",
        value: undefined,
      }),
    );

    expect(next).toEqual(INITIAL_VIEW_STATE);
    const log = usePluginErrorStore.getState().log;
    expect(log).toHaveLength(1);
    expect(log[0]).toMatchObject({ plugin: "plug", source: "agui" });
  });

  it("a handler that returns void leaves state untouched", () => {
    const host = createHost("plug", []);
    host.agui.on("custom.metrics", () => {
      /* fire-and-forget side effect */
    });

    const next = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.CUSTOM,
        name: "custom.metrics",
        value: { count: 1 },
      }),
    );
    expect(next).toBe(INITIAL_VIEW_STATE);
  });

  it("handler can use appendBlockToMessage for explicit targeting", () => {
    const seeded = reduce(
      INITIAL_VIEW_STATE,
      ev({
        type: EventType.TEXT_MESSAGE_START,
        messageId: "m1",
        role: "assistant",
      }),
    );

    const host = createHost("plug", []);
    host.agui.on<{ id: string }>("custom.tag", (v) => appendBlockToMessage(v.id, { kind: "plan" }));

    const next = reduce(
      seeded,
      ev({
        type: EventType.CUSTOM,
        name: "custom.tag",
        value: { id: "m1" },
      }),
    );

    expect(next.messages[0].blocks).toEqual([{ kind: "plan" }]);
  });
});
