import type { AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";
import { describe, expect, it } from "vitest";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { appendBlockToLatestAssistant, appendBlockToMessage, compose } from "./state";

// Helpers to construct messages without typing the whole shape every time.
const msg = (id: string, role: Message["role"] = "assistant"): Message => ({
  id,
  role,
  createdAt: "2026-01-01T00:00:00.000Z",
  runId: null,
  blocks: [],
});

const stateWith = (messages: Message[]): AgentSessionView => ({
  ...EMPTY_AGENT_SESSION_VIEW,
  messages,
});

const IMAGE_BLOCK = { kind: "image", mime: "image/png", data: "iVBOR" } as const;

describe("appendBlockToMessage", () => {
  it("appends to the matching message id", () => {
    const update = appendBlockToMessage("m1", { kind: "text", text: "hi", status: "complete" });
    const next = update(stateWith([msg("m1"), msg("m2")]));

    expect(next.messages[0]!.blocks).toHaveLength(1);
    expect(next.messages[1]!.blocks).toHaveLength(0);
  });

  it("is a no-op when the id is missing", () => {
    const initial = stateWith([msg("m1")]);
    const update = appendBlockToMessage("nope", { kind: "text", text: "x", status: "complete" });
    const next = update(initial);
    expect(next.messages[0]!.blocks).toHaveLength(0);
  });
});

describe("appendBlockToLatestAssistant", () => {
  it("targets the most-recent assistant message", () => {
    const update = appendBlockToLatestAssistant(IMAGE_BLOCK);
    const next = update(
      stateWith([
        msg("u1", "user"),
        msg("a1"), // assistant — not latest
        msg("u2", "user"),
        msg("a2"), // latest assistant
      ]),
    );

    expect(next.messages[1]!.blocks).toHaveLength(0);
    expect(next.messages[3]!.blocks).toHaveLength(1);
    expect(next.messages[3]!.blocks[0]).toEqual(IMAGE_BLOCK);
  });

  it("is a no-op when no assistant messages exist", () => {
    const update = appendBlockToLatestAssistant(IMAGE_BLOCK);
    const initial = stateWith([msg("u1", "user")]);
    expect(update(initial)).toBe(initial);
  });
});

describe("compose", () => {
  it("applies updates left-to-right", () => {
    const update = compose(
      appendBlockToLatestAssistant(IMAGE_BLOCK),
      appendBlockToLatestAssistant({
        kind: "text",
        text: "building",
        status: "running",
      }),
    );
    const next = update(stateWith([msg("assistant")]));
    expect(next.messages[0]?.blocks.map((block) => block.kind)).toEqual(["image", "text"]);
  });

  it("returns the original state when called with zero updates", () => {
    const update = compose();
    expect(update(EMPTY_AGENT_SESSION_VIEW)).toBe(EMPTY_AGENT_SESSION_VIEW);
  });
});
