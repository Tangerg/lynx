// Reducer — `custom` StreamEvent dispatch. The dispatcher routes by
// `event.name`; built-in semantics are all first-class events now, so
// `custom` is purely the third-party extension channel. Tests cover
// routing, the unrecognised-name no-op, and throwing-handler isolation.

import { beforeEach, describe, expect, it } from "vitest";
import type { AgentItem as Item, AgentStreamEvent as StreamEvent } from "@/plugins/sdk";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { appendBlockToLatestAssistant, appendBlockToMessage } from "@/plugins/sdk/state";
import { foldTestEvent as reduce } from "./reducer.fixtures";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { CUSTOM_EVENT_HANDLER, definePlugin } from "@/plugins/sdk";
import type { CustomEventHandler } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

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
  return reduce(EMPTY_AGENT_SESSION_VIEW, { type: "item.started", item });
}

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
});

// The fold plugin has to stay installed alongside the handler under test, so each
// case reboots the kernel with both rather than adding to a running one.
async function withHandler(name: string, handler: (payload: never) => unknown) {
  const { default: fold } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(
    fold,
    definePlugin({
      name: "plug",
      setup: (ctx) => {
        ctx.contribute(CUSTOM_EVENT_HANDLER, {
          name,
          handler: handler as CustomEventHandler<unknown>,
        });
      },
    }),
  );
}

describe("reducer — custom StreamEvent fallback", () => {
  it("unrecognized name with no registered handler is a no-op", () => {
    const next = reduce(EMPTY_AGENT_SESSION_VIEW, custom("unregistered.xyz", { whatever: true }));
    expect(next).toEqual(EMPTY_AGENT_SESSION_VIEW);
  });

  it("routes to a plugin-registered handler", async () => {
    const seeded = seedAssistant();
    await withHandler("custom.banner", (value: { text: string }) =>
      appendBlockToLatestAssistant({
        kind: "text",
        text: `banner: ${value.text}`,
        status: "complete",
      }),
    );

    const next = reduce(seeded, custom("custom.banner", { text: "hi" }));
    expect(next.messages[0]!.blocks.at(-1)).toEqual({
      kind: "text",
      text: "banner: hi",
      status: "complete",
    });
  });

  it("a handler that throws is isolated + logged to the error store", async () => {
    await withHandler("custom.boom", () => {
      throw new Error("nope");
    });

    const next = reduce(EMPTY_AGENT_SESSION_VIEW, custom("custom.boom", undefined));
    expect(next).toEqual(EMPTY_AGENT_SESSION_VIEW);
    expect(usePluginErrorStore.getState().log.at(-1)).toMatchObject({
      plugin: "plug",
      source: "events",
    });
  });

  it("a void-returning handler leaves state untouched", async () => {
    await withHandler("custom.metrics", () => {
      /* fire-and-forget side effect */
    });

    const next = reduce(EMPTY_AGENT_SESSION_VIEW, custom("custom.metrics", { count: 1 }));
    expect(next).toBe(EMPTY_AGENT_SESSION_VIEW);
  });

  it("handler can use appendBlockToMessage for explicit targeting", async () => {
    const seeded = seedAssistant();
    const targetId = seeded.messages[0]!.id;
    await withHandler("custom.tag", (v: { id: string }) =>
      appendBlockToMessage(v.id, { kind: "image", mime: "image/png", data: "iVBOR" }),
    );

    const next = reduce(seeded, custom("custom.tag", { id: targetId }));
    expect(next.messages[0]!.blocks.at(-1)).toEqual({
      kind: "image",
      mime: "image/png",
      data: "iVBOR",
    });
  });
});
