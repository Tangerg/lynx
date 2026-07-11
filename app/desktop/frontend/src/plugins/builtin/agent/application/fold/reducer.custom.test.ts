// Reducer — `custom` StreamEvent dispatch. The dispatcher routes by
// `event.name`; built-in semantics are all first-class events now, so
// `custom` is purely the third-party extension channel. Tests cover
// routing, the unrecognised-name no-op, and throwing-handler isolation.

import { beforeEach, describe, expect, it } from "vitest";
import type { Item, StreamEvent } from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { createHost } from "@/plugins/sdk/host";
import { appendBlockToLatestAssistant, appendBlockToMessage } from "@/plugins/sdk/state";
import { reduce } from "./reducer";
import { INITIAL_VIEW_STATE } from "@/plugins/sdk/types/agentView";

const custom = (name: string, payload: unknown): StreamEvent => ({ type: "custom", name, payload });

// Seed one assistant turn so latest-assistant helpers have a target.
function seedAssistant() {
  const item = {
    id: "item_1",
    runId: "run_1",
    status: "running",
    createdAt: "2026-06-03T00:00:00Z",
    type: "agentMessage",
    content: [],
  } as unknown as Item;
  return reduce(INITIAL_VIEW_STATE, { type: "item.started", item });
}

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
});

describe("reducer — custom StreamEvent fallback", () => {
  it("unrecognized name with no registered handler is a no-op", () => {
    const next = reduce(INITIAL_VIEW_STATE, custom("unregistered.xyz", { whatever: true }));
    expect(next).toEqual(INITIAL_VIEW_STATE);
  });

  it("routes to a plugin-registered handler", () => {
    const seeded = seedAssistant();
    const host = createHost("plug", []);
    host.events.onCustom<{ text: string }>("custom.banner", (value) =>
      appendBlockToLatestAssistant({
        kind: "text",
        text: `banner: ${value.text}`,
        status: "complete",
      }),
    );

    const next = reduce(seeded, custom("custom.banner", { text: "hi" }));
    const last = next.messages[0]!.blocks.at(-1);
    expect(last).toEqual({ kind: "text", text: "banner: hi", status: "complete" });
  });

  it("a handler that throws is isolated + logged to the error store", () => {
    const host = createHost("plug", []);
    host.events.onCustom("custom.boom", () => {
      throw new Error("nope");
    });

    const next = reduce(INITIAL_VIEW_STATE, custom("custom.boom", undefined));
    expect(next).toEqual(INITIAL_VIEW_STATE);
    const log = usePluginErrorStore.getState().log;
    expect(log.at(-1)).toMatchObject({ plugin: "plug", source: "events" });
  });

  it("a void-returning handler leaves state untouched", () => {
    const host = createHost("plug", []);
    host.events.onCustom("custom.metrics", () => {
      /* fire-and-forget side effect */
    });
    const next = reduce(INITIAL_VIEW_STATE, custom("custom.metrics", { count: 1 }));
    expect(next).toBe(INITIAL_VIEW_STATE);
  });

  it("handler can use appendBlockToMessage for explicit targeting", () => {
    const seeded = seedAssistant();
    const targetId = seeded.messages[0]!.id;
    const host = createHost("plug", []);
    host.events.onCustom<{ id: string }>("custom.tag", (v) =>
      appendBlockToMessage(v.id, { kind: "plan" }),
    );

    const next = reduce(seeded, custom("custom.tag", { id: targetId }));
    expect(next.messages[0]!.blocks.at(-1)).toEqual({ kind: "plan" });
  });
});
