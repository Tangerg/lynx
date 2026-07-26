import { describe, expect, it, vi } from "vitest";
import { chatSearchCommand, chatSearchOverlaySlot } from "./chatSearchContributions";

function Component() {
  return null;
}

describe("chatSearchOverlaySlot", () => {
  it("projects the chat search component into the overlay slot spec", () => {
    expect(chatSearchOverlaySlot(Component)).toEqual({
      id: "chat-search",
      order: 50,
      component: Component,
    });
  });
});

describe("chatSearchCommand", () => {
  it("carries Mod+F so the global keymap can bind it", () => {
    const command = chatSearchCommand(vi.fn());

    expect(command).toMatchObject({
      id: "chat.search",
      label: "command.chatSearch",
      combo: "Mod+F",
    });
  });

  it("opens chat search when run", () => {
    const openSearch = vi.fn();

    void chatSearchCommand(openSearch).run();

    expect(openSearch).toHaveBeenCalledOnce();
  });
});
