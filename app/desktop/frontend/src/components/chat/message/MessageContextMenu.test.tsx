// MessageContextMenu tests — locks in two things that have proved
// easy to break:
//
//   1. **Role-conditional items.** Edit is user-only, Regenerate is
//      assistant-only; mis-targeted refactors keep wanting to either
//      expose them on both sides or drop one accidentally.
//   2. **Edit in composer** populates `composerStore` rather than
//      mutating the message in place. This is the user's expected
//      affordance — message immutability + composer rehydrate.
//
// We don't test Regenerate behaviour here because it would require
// stubbing an entire agentStore session + send() pipeline; the
// helper's `send` lookup is covered indirectly by the agentStore
// suite.

import type { Message } from "@/protocol/run/viewState";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  getComposerText,
  replaceComposerDraft,
} from "@/plugins/builtin/chat/composer/public/draft";
import { MessageContextMenu } from "./MessageContextMenu";

function buildMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: "m1",
    role: "user",
    who: "you",
    time: "12:00",
    blocks: [{ kind: "text", text: "hello world", status: "complete" }],
    ...overrides,
  };
}

// Base UI ContextMenu.Trigger opens on the native `contextmenu` event
// (right-click). fireEvent.contextMenu mimics that. The menu mounts
// into a Portal — getByText still finds it because Testing Library
// queries against `document.body`.
function openMenu(triggerLabel: string): void {
  fireEvent.contextMenu(screen.getByText(triggerLabel));
}

describe("messageContextMenu", () => {
  it("shows Edit in composer + copy items for a user message", () => {
    render(
      <MessageContextMenu msg={buildMessage({ role: "user" })}>
        <div>user message</div>
      </MessageContextMenu>,
    );
    openMenu("user message");
    expect(screen.getByText("Edit in composer")).toBeTruthy();
    expect(screen.getByText("Copy plain text")).toBeTruthy();
    expect(screen.queryByText("Regenerate response")).toBeNull();
  });

  it("shows Regenerate + copy items for an assistant message", () => {
    render(
      <MessageContextMenu msg={buildMessage({ role: "assistant" })}>
        <div>assistant message</div>
      </MessageContextMenu>,
    );
    openMenu("assistant message");
    expect(screen.getByText("Regenerate response")).toBeTruthy();
    expect(screen.getByText("Copy plain text")).toBeTruthy();
    expect(screen.queryByText("Edit in composer")).toBeNull();
  });

  it("does not show copy items when the message body is empty", () => {
    render(
      <MessageContextMenu
        msg={buildMessage({
          role: "user",
          blocks: [{ kind: "text", text: "", status: "complete" }],
        })}
      >
        <div>empty</div>
      </MessageContextMenu>,
    );
    openMenu("empty");
    // No copy / edit items — only assistant-side Regenerate would
    // remain, but this is a user message, so the menu has no
    // actionable items.
    expect(screen.queryByText("Copy markdown")).toBeNull();
    expect(screen.queryByText("Copy plain text")).toBeNull();
    expect(screen.queryByText("Edit in composer")).toBeNull();
  });

  it("Edit in composer loads the message text into composerStore", () => {
    replaceComposerDraft({ text: "", images: [] });
    render(
      <MessageContextMenu
        msg={buildMessage({
          role: "user",
          blocks: [{ kind: "text", text: "draft me again", status: "complete" }],
        })}
      >
        <div>user msg</div>
      </MessageContextMenu>,
    );
    openMenu("user msg");
    fireEvent.click(screen.getByText("Edit in composer"));
    expect(getComposerText()).toBe("draft me again");
  });
});
