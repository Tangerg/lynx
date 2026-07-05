import { describe, expect, it } from "vitest";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import {
  canCopyMessage,
  canEditMessage,
  canRateMessage,
  canRegenerateMessage,
  canUseMessageRunCheckpoint,
} from "./messageActionAvailability";

const message = (overrides: Partial<Message>): Message => ({
  blocks: [],
  id: "m",
  role: "assistant",
  time: "",
  who: "",
  ...overrides,
});

describe("message action availability", () => {
  it("uses copy payload availability for copy actions", () => {
    expect(canCopyMessage({ canCopy: true })).toBe(true);
    expect(canCopyMessage({ canCopy: false })).toBe(false);
  });

  it("allows editing draftable user messages, including image-only content", () => {
    expect(
      canEditMessage(
        message({
          role: "user",
          blocks: [{ kind: "image", mime: "image/png", data: "abc" }],
        }),
      ),
    ).toBe(true);
    expect(canEditMessage(message({ role: "assistant", blocks: [] }))).toBe(false);
  });

  it("uses user run ids for checkpoint actions", () => {
    expect(canUseMessageRunCheckpoint(message({ role: "user", runId: "run_1" }))).toBe(true);
    expect(canUseMessageRunCheckpoint(message({ role: "assistant", runId: "run_1" }))).toBe(false);
  });

  it("keeps regenerate and feedback assistant-only", () => {
    expect(canRegenerateMessage(message({ role: "assistant" }))).toBe(true);
    expect(canRegenerateMessage(message({ role: "user" }))).toBe(false);
    expect(canRateMessage(message({ role: "assistant" }))).toBe(true);
    expect(canRateMessage(message({ role: "user" }))).toBe(false);
  });
});
