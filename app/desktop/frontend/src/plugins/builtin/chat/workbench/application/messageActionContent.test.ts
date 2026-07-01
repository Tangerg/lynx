import { describe, expect, it } from "vitest";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import {
  messageDraftContent,
  messageHasDraftContent,
  regenerationPromptBefore,
} from "./messageActionContent";

const message = (overrides: Partial<Message>): Message => ({
  blocks: [],
  id: "m",
  role: "assistant",
  time: "",
  who: "",
  ...overrides,
});

describe("messageActionContent", () => {
  it("extracts text and inline images for composer drafts", () => {
    expect(
      messageDraftContent(
        message({
          blocks: [
            { kind: "text", status: "complete", text: "hello" },
            { kind: "image", mime: "image/png", data: "abc" },
          ],
        }),
      ),
    ).toEqual({ text: "hello", images: [{ mime: "image/png", data: "abc" }] });
  });

  it("treats image-only messages as draftable content", () => {
    expect(
      messageHasDraftContent(
        message({ blocks: [{ kind: "image", mime: "image/png", data: "x" }] }),
      ),
    ).toBe(true);
    expect(messageHasDraftContent(message({ blocks: [] }))).toBe(false);
  });

  it("finds the user prompt before an assistant message for regeneration", () => {
    expect(
      regenerationPromptBefore(
        [
          message({
            blocks: [{ kind: "text", status: "complete", text: "  prompt  " }],
            id: "u1",
            role: "user",
            runId: "run_1",
          }),
          message({ id: "a1", role: "assistant" }),
        ],
        "a1",
      ),
    ).toEqual({ text: "prompt", images: [], runId: "run_1" });
  });

  it("does not skip past an empty user prompt", () => {
    expect(
      regenerationPromptBefore(
        [
          message({
            blocks: [{ kind: "text", status: "complete", text: "older" }],
            id: "u1",
            role: "user",
            runId: "run_1",
          }),
          message({
            blocks: [{ kind: "text", status: "complete", text: "   " }],
            id: "u2",
            role: "user",
            runId: "run_2",
          }),
          message({ id: "a1", role: "assistant" }),
        ],
        "a1",
      ),
    ).toBeNull();
  });
});
