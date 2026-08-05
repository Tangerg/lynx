import type { CommandSpec, LayoutSlotSpec, ShortcutSpec } from "@/plugins/sdk";

export type CommandRun = CommandSpec["run"];

export function commandPaletteOverlaySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "command-palette",
    order: 10,
    component,
  };
}

export function commandPaletteShortcut(togglePalette: () => void): ShortcutSpec {
  return {
    key: "Mod+K",
    description: "shortcut.commandPalette",
    // Cmd+K is the escape hatch users expect while typing in the composer.
    allowInInputs: true,
    handler: (event) => {
      event.preventDefault();
      togglePalette();
    },
  };
}

export function commandPaletteCommand(openPalette: CommandRun): CommandSpec {
  return {
    id: "command.open",
    label: "command.openPalette",
    icon: "command",
    group: "command.group.general",
    keywords: ["palette", "search", "command"],
    // Not a row inside the palette: "Open command palette" offered to open the
    // thing already open. The command still exists because it owns the ⌘K
    // binding — the shortcut is what it is for, the row was never useful.
    when: "!paletteOpen",
    combo: "Mod+K",
    run: openPalette,
  };
}
