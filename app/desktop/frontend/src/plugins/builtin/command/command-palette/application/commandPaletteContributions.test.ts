import { describe, expect, it, vi } from "vitest";
import { commandPaletteCommand, commandPaletteShortcut } from "./commandPaletteContributions";

describe("commandPaletteShortcut", () => {
  it("binds Mod+K as an input-safe palette toggle", () => {
    const shortcut = commandPaletteShortcut(vi.fn());

    expect(shortcut).toMatchObject({
      key: "Mod+K",
      description: "Open the command palette",
      allowInInputs: true,
    });
  });

  it("prevents the browser default before toggling the palette", () => {
    const togglePalette = vi.fn();
    const shortcut = commandPaletteShortcut(togglePalette);
    const event = { preventDefault: vi.fn() } as unknown as KeyboardEvent;

    shortcut.handler(event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(togglePalette).toHaveBeenCalledOnce();
  });
});

describe("commandPaletteCommand", () => {
  it("projects the open action into a stable command spec", () => {
    const openPalette = vi.fn();

    expect(commandPaletteCommand((key) => `t:${key}`, openPalette)).toEqual({
      id: "command.open",
      label: "t:command.openPalette",
      icon: "command",
      group: "General",
      keywords: ["palette", "search", "command"],
      combo: "Mod+K",
      run: openPalette,
    });
  });
});
