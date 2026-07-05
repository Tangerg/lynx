import { describe, expect, it } from "vitest";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import {
  messageContextMenuModel,
  type MessageContextMenuCopyState,
} from "./messageContextMenuModel";

const message = (overrides: Partial<Message>): Message => ({
  blocks: [],
  id: "m",
  role: "user",
  time: "",
  who: "",
  ...overrides,
});

const copy = (
  overrides: Partial<MessageContextMenuCopyState> = {},
): MessageContextMenuCopyState => ({
  plain: "",
  code: "",
  canCopy: false,
  ...overrides,
});

describe("messageContextMenuModel", () => {
  it("projects copy payload availability independently from message actions", () => {
    const model = messageContextMenuModel({
      msg: message({ role: "assistant" }),
      copy: copy({ plain: "hi", code: "const x = 1", canCopy: true }),
      canRestoreFiles: false,
    });

    expect(model.copyMarkdown).toBe(true);
    expect(model.copyPlain).toBe(true);
    expect(model.copyCode).toBe(true);
  });

  it("uses draftable content, not plain copy text, for user edit actions", () => {
    const model = messageContextMenuModel({
      msg: message({
        role: "user",
        blocks: [{ kind: "image", mime: "image/png", data: "abc" }],
      }),
      copy: copy(),
      canRestoreFiles: false,
    });

    expect(model.user.visible).toBe(true);
    expect(model.user.editInComposer).toBe(true);
    expect(model.copyPlain).toBe(false);
  });

  it("enables run-scoped user actions from a user run id", () => {
    const model = messageContextMenuModel({
      msg: message({
        role: "user",
        runId: "run_1",
        blocks: [{ kind: "text", status: "complete", text: "fix this" }],
      }),
      copy: copy({ plain: "fix this", canCopy: true }),
      canRestoreFiles: true,
    });

    expect(model.user).toMatchObject({
      visible: true,
      editRerun: true,
      editRerunRestore: true,
      restore: true,
      restoreFiles: true,
      restoreBoth: true,
      fork: true,
    });
  });

  it("enables assistant regenerate actions by restore capability", () => {
    expect(
      messageContextMenuModel({
        msg: message({ role: "assistant" }),
        copy: copy(),
        canRestoreFiles: true,
      }).assistant,
    ).toEqual({ visible: true, regenerate: true, regenerateRestore: true });
  });
});
