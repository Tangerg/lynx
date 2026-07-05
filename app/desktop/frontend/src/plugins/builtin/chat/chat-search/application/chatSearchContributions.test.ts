import { describe, expect, it, vi } from "vitest";
import { chatSearchOverlaySlot, chatSearchShortcut } from "./chatSearchContributions";

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

describe("chatSearchShortcut", () => {
  it("binds Mod+F as an input-safe chat search shortcut", () => {
    const shortcut = chatSearchShortcut((key) => `t:${key}`, vi.fn());

    expect(shortcut).toMatchObject({
      key: "Mod+F",
      description: "t:chatSearch.shortcutDesc",
      allowInInputs: true,
    });
  });

  it("prevents the browser find dialog before opening chat search", () => {
    const openSearch = vi.fn();
    const shortcut = chatSearchShortcut((key) => key, openSearch);
    const event = { preventDefault: vi.fn() } as unknown as KeyboardEvent;

    shortcut.handler(event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(openSearch).toHaveBeenCalledOnce();
  });
});
