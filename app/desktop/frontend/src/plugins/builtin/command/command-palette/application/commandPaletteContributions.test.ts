import { describe, expect, it, vi } from "vitest";
import {
  commandPaletteCommand,
  commandPaletteOverlaySlot,
  commandPaletteShortcut,
} from "./commandPaletteContributions";

describe("commandPaletteShortcut", () => {
  it("binds Mod+K as an input-safe palette toggle", () => {
    const shortcut = commandPaletteShortcut(vi.fn());

    expect(shortcut).toMatchObject({
      key: "Mod+K",
      description: "shortcut.commandPalette",
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

function Component() {
  return null;
}

describe("commandPaletteOverlaySlot", () => {
  it("projects the palette component into the overlay slot spec", () => {
    expect(commandPaletteOverlaySlot(Component)).toEqual({
      id: "command-palette",
      order: 10,
      component: Component,
    });
  });
});

describe("commandPaletteCommand", () => {
  it("projects the open action into a stable command spec", () => {
    const openPalette = vi.fn();

    expect(commandPaletteCommand(openPalette)).toEqual({
      id: "command.open",
      label: "command.openPalette",
      icon: "command",
      group: "command.group.general",
      keywords: ["palette", "search", "command"],
      // Hidden while the palette is open: the row offered to open the thing
      // already open. The command stays because it owns the ⌘K binding.
      when: "!paletteOpen",
      combo: "Mod+K",
      run: openPalette,
    });
  });
});
