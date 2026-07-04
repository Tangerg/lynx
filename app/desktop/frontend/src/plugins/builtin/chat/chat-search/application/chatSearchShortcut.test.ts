import { describe, expect, it, vi } from "vitest";
import { chatSearchShortcut } from "./chatSearchShortcut";

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
